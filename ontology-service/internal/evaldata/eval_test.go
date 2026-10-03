//go:build eval

// REQ-207/M43（52 号 E5 并入）：本体域 eval 基准跑分器——双信号：
//   ①结构校验（spec 完整性：概念数>0/孤立概念/关系端点存在等轻量断言）
//   ②质量门禁评分（qualitygate.Check 三维加权 Overall）
// 对种子 5 份（internal/seed/examples）+ 运行平面真实本体 3 份（onto_k8s_ops/med_common/gene_core，
// 经构建平面 /api/ontologies/{id}/spec 拉取）出表。build tag `eval` 手动跑分，沿 agenteval/evaldata 先例。
//
// 运行（仓库 ontology-service/ 目录下）：
//	go test -tags eval ./internal/evaldata/ -run TestOntoEval -v
// 进化诊断步直接消费同一套双信号（internal/evolution 包）。
package evaldata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/qualitygate"
)

type evalResult struct {
	ID       string  `json:"id"`
	Source   string  `json:"source"` // seed | real
	Channel  string  `json:"channel"` // REQ-257：构建渠道（seed|fork|import|custom）
	Overall  float64 `json:"overall"`
	Concepts int     `json:"concepts"`
	Relations int    `json:"relations"`
	StructOK bool    `json:"struct_ok"`
	Issues   string  `json:"issues,omitempty"`
}

// structCheck 结构轻量断言（进化「诊断」信号①）。
func structCheck(sp *pkgspec.Spec) (bool, []string) {
	var issues []string
	if len(sp.Concepts) == 0 {
		issues = append(issues, "无概念")
	}
	idx := map[string]bool{}
	for _, c := range sp.Concepts {
		if strings.TrimSpace(c.Name) == "" {
			issues = append(issues, "存在空名概念")
		}
		idx[c.Name] = true
	}
	for _, r := range sp.Relations {
		if !idx[r.From] {
			issues = append(issues, fmt.Sprintf("关系 %s 的 from %s 不存在", r.Name, r.From))
		}
		if !idx[r.To] {
			issues = append(issues, fmt.Sprintf("关系 %s 的 to %s 不存在", r.Name, r.To))
		}
	}
	return len(issues) == 0, issues
}

func evalSpec(t *testing.T, id, source, channel string, specJSON []byte) evalResult {
	t.Helper()
	var sp pkgspec.Spec
	if err := json.Unmarshal(specJSON, &sp); err != nil {
		return evalResult{ID: id, Source: source, Channel: channel, Issues: "spec 解析失败: " + err.Error()}
	}
	ok, issues := structCheck(&sp)
	rep := qualitygate.Check(&sp, qualitygate.DefaultConfig())
	sort.Strings(issues)
	return evalResult{ID: id, Source: source, Channel: channel, Overall: rep.Score.Overall, Concepts: len(sp.Concepts),
		Relations: len(sp.Relations), StructOK: ok, Issues: strings.Join(issues, "; ")}
}

func TestOntoEval(t *testing.T) {
	var results []evalResult

	// ① 种子（embed 形态从 examples 目录读——测试 cwd=ontology-service/internal/evaldata）
	entries, err := os.ReadDir("../../internal/seed/examples")
	if err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			b, rerr := os.ReadFile("../../internal/seed/examples/" + e.Name())
			if rerr != nil {
				t.Logf("seed %s 读取失败: %v", e.Name(), rerr)
				continue
			}
			results = append(results, evalSpec(t, strings.TrimSuffix(e.Name(), ".json"), "seed", "seed", b)) // 种子目录=seed 渠道（文件名无 onto_ 前缀，派生规则不适用）
		}
	}

	// ② 真实三份（构建平面直拉；未启动则跳过并注记）
	buildURL := os.Getenv("BUILD_SVC_URL")
	if buildURL == "" {
		buildURL = "http://127.0.0.1:8091"
	}
	for _, oid := range []string{"onto_k8s_ops", "onto_med_common", "onto_gene_core"} {
		resp, err := http.Get(buildURL + "/api/ontologies/" + oid + "/spec")
		if err != nil {
			t.Logf("真实本体 %s 拉取失败（构建平面未启动即跳过）: %v", oid, err)
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Logf("真实本体 %s 返回 %d，跳过", oid, resp.StatusCode)
				return
			}
			buf := make([]byte, 0, 1<<16)
			tmp := make([]byte, 4096)
			for {
				n, rerr := resp.Body.Read(tmp)
				buf = append(buf, tmp[:n]...)
				if rerr != nil {
					break
				}
			}
			// REQ-257 渠道派生：meta.forked_from + artifacts（original 形态存在=import）
			forked, hasOrig := "", false
			if mresp, merr := http.Get(buildURL + "/api/ontologies/" + oid); merr == nil {
				var meta struct {
					ForkedFrom string `json:"forked_from"`
				}
				_ = json.NewDecoder(mresp.Body).Decode(&meta)
				mresp.Body.Close()
				forked = meta.ForkedFrom
			}
			if aresp, aerr := http.Get(buildURL + "/api/ontologies/" + oid + "/artifacts"); aerr == nil {
				var arts []struct {
					Format string `json:"format"`
				}
				_ = json.NewDecoder(aresp.Body).Decode(&arts)
				aresp.Body.Close()
				for _, a := range arts {
					if a.Format == "turtle" || a.Format == "owl_rdfxml" || a.Format == "jsonld" {
						hasOrig = true
					}
				}
			}
			results = append(results, evalSpec(t, oid, "real", DeriveChannel(oid, forked, hasOrig), buf))
		}()
	}

	if len(results) == 0 {
		t.Skip("无可用评测对象（种子目录缺失且构建平面不可达）")
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Source != results[j].Source {
			return results[i].Source == "seed"
		}
		return results[i].ID < results[j].ID
	})
	t.Logf("%-28s %-6s %8s %6s %6s  %s", "ID", "来源", "Overall", "概念", "关系", "结构/问题")
	failCount := 0
	for _, r := range results {
		status := "✓"
		if !r.StructOK {
			status = "✗ " + r.Issues
			failCount++
		}
		t.Logf("%-28s %-6s %8.2f %6d %6d  %s", r.ID, r.Source, r.Overall, r.Concepts, r.Relations, status)
	}
	// 硬断言：全部对象结构校验必须通过（结构信号）；质量分只报告不做硬门禁（阈值校准后续轮）
	if failCount > 0 {
		t.Errorf("%d/%d 个本体结构校验未通过", failCount, len(results))
	}

	// REQ-257 分渠道质量画像（60 号 H6）：按渠道分组出质量分布——产品侧「路径选择建议」数据基础。
	groups := map[string][]evalResult{}
	for _, r := range results {
		groups[r.Channel] = append(groups[r.Channel], r)
	}
	channels := make([]string, 0, len(groups))
	for ch := range groups {
		channels = append(channels, ch)
	}
	sort.Strings(channels)
	t.Logf("")
	t.Logf("== 分渠道质量画像（REQ-257；custom 含 AI 创建/OntoChat/KB 构建——v1 不细分，生成时落 source_path 标记为观察项）==")
	t.Logf("%-10s %4s %10s %10s %10s %8s", "渠道", "数量", "均分", "最低", "最高", "结构通过")
	for _, ch := range channels {
		rs := groups[ch]
		st := ChannelStat{Channel: ch, Count: len(rs), MinScore: rs[0].Overall, MaxScore: rs[0].Overall}
		sum := 0.0
		okN := 0
		for _, r := range rs {
			sum += r.Overall
			if r.Overall < st.MinScore {
				st.MinScore = r.Overall
			}
			if r.Overall > st.MaxScore {
				st.MaxScore = r.Overall
			}
			if r.StructOK {
				okN++
			}
		}
		st.AvgScore = sum / float64(len(rs))
		st.StructOK = okN
		t.Logf("%-10s %4d %10.2f %10.2f %10.2f %d/%d", st.Channel, st.Count, st.AvgScore, st.MinScore, st.MaxScore, st.StructOK, st.Count)
	}
}
