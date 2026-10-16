//go:build eval

// REQ-345/M145③：ontoeval CQ 信号接通——对基准集（seeds/evaldata/ontology-cq/）逐条 CQ：
// LLM 翻译为 SPARQL SELECT → 只读白名单校验 → gold 对齐（查询中引用 gold 概念/关系的 URN IRI）。
// 解除 REQ-255「ontoeval CQ 信号 N/A（种子 spec 无 CQ）」遗留；信号=翻译成功率/白名单通过率/gold 对齐率。
// build tag eval 手动跑（LLM 依赖：真机 GLM 经主平台代理）。运行：
//
//	go test -tags eval ./internal/evaldata/ -run TestCQSignal -v
package evaldata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

// cqSignal 单条 CQ 的信号（REQ-255 CQ 信号语义的 eval 落地）。
type cqSignal struct {
	Dataset     string `json:"dataset"`
	ID          string `json:"id"`
	CQ          string `json:"cq"`
	Translated  bool   `json:"translated"`   // LLM 产出非空查询
	WhitelistOK bool   `json:"whitelist_ok"` // 只读白名单通过
	GoldAligned bool   `json:"gold_aligned"` // 查询引用 gold 概念/关系的 URN IRI
	Query       string `json:"query,omitempty"`
	Error       string `json:"error,omitempty"`
}

func TestCQSignal(t *testing.T) {
	platformURL := os.Getenv("PLATFORM_URL")
	if platformURL == "" {
		platformURL = "http://127.0.0.1:8080"
	}
	if resp, err := http.Get(platformURL + "/healthz"); err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf("主平台不可达（%v）——CQ 信号依赖 LLM 翻译链，跳过", err)
	} else {
		resp.Body.Close()
	}
	llm := llmcreate.New(platformURL)
	llm.HTTP.Timeout = 300 * time.Second

	ents, err := os.ReadDir(datasetDir)
	if err != nil {
		t.Skipf("数据集目录不可达: %v", err)
	}
	var signals []cqSignal
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, rerr := os.ReadFile(datasetDir + "/" + e.Name())
		if rerr != nil {
			continue
		}
		var ds cqDataset
		if json.Unmarshal(raw, &ds) != nil {
			continue
		}
		seedRaw, serr := os.ReadFile("../../internal/seed/examples/" + ds.Ontology + ".json")
		if serr != nil {
			continue
		}
		var sp pkgspec.Spec
		if json.Unmarshal(seedRaw, &sp) != nil {
			continue
		}
		for _, dc := range ds.CQs {
			sig := cqSignal{Dataset: ds.Ontology, ID: dc.ID, CQ: dc.CQ}
			// 翻译 prompt：与 REST nl2sparql/cqSparql 同一 IRI 模板口径
			var b strings.Builder
			b.WriteString("你是本体查询专家。把能力问题翻译为一条只读 SPARQL 1.1 SELECT 查询。\n")
			fmt.Fprintf(&b, "本体 IRI 模板：概念=<urn:o:%s:concept:名称>，关系=<urn:o:%s:relation:名称>，实例=<urn:o:%s:instance:名称>。\n", ds.Ontology, ds.Ontology, ds.Ontology)
			conc := make([]string, 0, len(sp.Concepts))
			for _, c := range sp.Concepts {
				conc = append(conc, c.Name)
			}
			rel := make([]string, 0, len(sp.Relations))
			for _, r := range sp.Relations {
				rel = append(rel, r.Name)
			}
			b.WriteString("可用概念：" + strings.Join(conc, "、") + "\n")
			b.WriteString("可用关系：" + strings.Join(rel, "、") + "\n")
			b.WriteString("模式：?x a <概念IRI> 取实例；<实例IRI> <关系IRI> <实例IRI> 为对象属性断言；加 LIMIT ≤50。只输出查询全文，不要 markdown。\n\n")
			b.WriteString("能力问题：" + dc.CQ)
			reply, _, err := llm.RawChat(context.Background(), b.String())
			if err != nil {
				time.Sleep(time.Second) // 代理瞬时故障重试一次（沿回喂重试先例；两败如实记 ✗）
				reply, _, err = llm.RawChat(context.Background(), b.String())
			}
			if err != nil {
				sig.Error = err.Error()
				signals = append(signals, sig)
				continue
			}
			query := strings.TrimSpace(reply)
			query = strings.TrimPrefix(strings.TrimPrefix(query, "```sparql"), "```")
			query = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), "```"))
			sig.Query = query
			sig.Translated = query != ""
			if sig.Translated {
				if verr := vetReadonlySelectEval(query); verr != nil {
					sig.Error = verr.Error()
				} else {
					sig.WhitelistOK = true
					// gold 对齐：查询文本含 gold 概念/关系的 sanitize 后 URN
					aligned := 0
					for _, g := range dc.Gold {
						if strings.Contains(query, sanitizeEval(g)) {
							aligned++
						}
					}
					sig.GoldAligned = aligned > 0
				}
			}
			signals = append(signals, sig)
		}
	}
	if len(signals) == 0 {
		t.Skip("无可评测 CQ（数据集空或种子缺失）")
	}
	tr, wl, al := 0, 0, 0
	for _, s := range signals {
		if s.Translated {
			tr++
		}
		if s.WhitelistOK {
			wl++
		}
		if s.GoldAligned {
			al++
		}
		status := "✗"
		if s.Translated && s.WhitelistOK && s.GoldAligned {
			status = "✓"
		} else if s.Error != "" {
			status = "✗ " + s.Error
		}
		t.Logf("%-14s %-40s 翻译=%-3v 白名单=%-3v gold对齐=%-3v %s", s.Dataset, trunc(s.CQ, 38), s.Translated, s.WhitelistOK, s.GoldAligned, status)
	}
	n := float64(len(signals))
	t.Logf("")
	t.Logf("== CQ 信号（REQ-345③：解除 REQ-255「ontoeval CQ 信号 N/A」遗留）==")
	t.Logf("翻译成功率 %.0f%%（%d/%d）| 白名单通过率 %.0f%% | gold 对齐率 %.0f%%",
		100*float64(tr)/n, tr, len(signals), 100*float64(wl)/n, 100*float64(al)/n)
	// 信号只报告不做硬门禁（沿 M43「质量分只报告」先例——LLM 瞬时故障不应让基准集评测红）；
	// 全过为预期态，系统性低于 80% 时人工排查生成链。
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func sanitizeEval(s string) string {
	// 与 pkg/ontology/spec.Sanitize 同规则（URN IRI 生成口径）——避免跨包引入的轻量副本
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' || r > 127 {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func vetReadonlySelectEval(query string) error {
	// 与 rest.vetReadonlySelect 同口径的 eval 侧轻量实现（首词 SELECT+变更关键字）
	toks := sparqlTokensEval(query)
	if len(toks) == 0 {
		return fmt.Errorf("空查询")
	}
	i := 0
	for i < len(toks) {
		u := strings.ToUpper(toks[i])
		if u == "PREFIX" {
			i += 2
		} else if u == "BASE" {
			i++
		} else {
			break
		}
	}
	if i >= len(toks) || !strings.EqualFold(toks[i], "select") {
		return fmt.Errorf("仅允许 SELECT，得到 %q", toks[min(i, len(toks)-1)])
	}
	for _, tk := range toks {
		switch strings.ToUpper(tk) {
		case "LOAD", "CLEAR", "CREATE", "DROP", "MOVE", "COPY", "ADD", "INSERT", "DELETE", "UPDATE":
			return fmt.Errorf("含变更关键字 %q", tk)
		}
	}
	return nil
}

func sparqlTokensEval(q string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(q)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '#':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case r == '"' || r == '\'':
			qc := r
			i++
			for i < len(runes) {
				if runes[i] == '\\' {
					i += 2
					continue
				}
				if runes[i] == qc {
					break
				}
				i++
			}
		case r == '<':
			for i < len(runes) && runes[i] != '>' {
				i++
			}
		case r == ' ' || r == '\n' || r == '\t' || r == '\r' || r == '(' || r == ')' || r == '{' || r == '}' || r == '.' || r == ',' || r == ';':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}
