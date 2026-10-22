// Package cqbench CQ 基准集服务化（REQ-346/M146 v1）：数据集清单加载 + 「用基准集测我」
// 运行器——把 evaldata 的 TestCQSignal 评测链（LLM 翻译 SELECT→只读白名单→gold 对齐）
// 从 build tag 测试提升为可服务化调用（REST 202 job 承载），报告落 artifact 供前端呈现。
// REQ-367/M165 评测纵深（r2）：数据集 60 条=5 种子×12；CQ 增双维标注 kind（fact/relation/
// inference 题型）×difficulty（easy/medium/hard 难度），既有 type（R/E/I 结构层）保留；
// Report 增分层跑分（strata）与口径声明（MetricDefinition，REQ-352 收编——报告自带判定/
// 统计口径与数据集 revision，防口径漂移跨版本误读）。
// Judge 复用预留注：rubric 化评分（LLM Judge 判定「答案内容正确性」而非仅 gold IRI 引用）
// 待与 backend/internal/agenteval judge.go 的 ParseJudgeOutput/rubric 骨架共契——跨模块
// 共享形态（pkg 抽取 vs 契约复刻）未拍板，v1 仅 gold IRI 对齐口径（REQ-345③ 同源）。
package cqbench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

// 难度与题型枚举（REQ-367 双维分层；type 结构层沿 CQ4OE 保留）。
const (
	DifficultyEasy   = "easy"
	DifficultyMedium = "medium"
	DifficultyHard   = "hard"
	KindFact         = "fact"
	KindRelation     = "relation"
	KindInference    = "inference"
)

// ValidDifficulty / ValidKind 标注枚举校验（LoadDataset 严格口径——基准集是产品资产，
// 漂移即缺陷；一致性测试另行断言分层齐备）。
func ValidDifficulty(v string) bool {
	return v == DifficultyEasy || v == DifficultyMedium || v == DifficultyHard
}

func ValidKind(v string) bool {
	return v == KindFact || v == KindRelation || v == KindInference
}

// CQ 数据集单条（REQ-367：增 kind/difficulty 双维标注）。
type CQ struct {
	ID         string   `json:"id"`
	CQ         string   `json:"cq"`
	Gold       []string `json:"gold"`
	Type       string   `json:"type"`       // 结构层 R|E|I（CQ4OE 口径，沿 r1）
	Kind       string   `json:"kind"`       // 题型 fact|relation|inference（r2 双维之一）
	Difficulty string   `json:"difficulty"` // 难度 easy|medium|hard（r2 双维之二）
}

// Dataset 数据集文件（seeds/evaldata/ontology-cq/<name>.json，REQ-345 形态 + r2 分层）。
type Dataset struct {
	Name     string `json:"name"` // 文件名（去扩展名）
	Note     string `json:"note"`
	Ontology string `json:"ontology"` // 源种子名
	Revision string `json:"revision"` // 数据集版本（r1=REQ-345 50 条 / r2=REQ-367 60 条分层）——口径声明防漂移（REQ-352 收编）
	CQs      []CQ   `json:"cqs"`
}

// DatasetMeta 清单行（管理面列表用；REQ-367 透出 revision 与分层分布）。
type DatasetMeta struct {
	Name     string `json:"name"`
	Ontology string `json:"ontology"`
	Count    int    `json:"count"`
	Revision string `json:"revision,omitempty"`
	Note     string `json:"note,omitempty"`
	// 分层分布（difficulty/kind 各档计数——「测我」前探明基准构成）
	Difficulties map[string]int `json:"difficulties,omitempty"`
	Kinds        map[string]int `json:"kinds,omitempty"`
}

// ListDatasets 数据集清单（dir 不存在/为空返回空表——文件形态部署随仓库，客户端包可不含）。
func ListDatasets(dir string) ([]DatasetMeta, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return []DatasetMeta{}, nil // 目录缺失=无数据集（诚实空表，不报错）
	}
	out := []DatasetMeta{}
	names := []string{}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		ds, derr := LoadDataset(filepath.Join(dir, n))
		if derr != nil {
			continue // 坏文件跳过不炸清单
		}
		out = append(out, DatasetMeta{
			Name: ds.Name, Ontology: ds.Ontology, Count: len(ds.CQs), Revision: ds.Revision, Note: ds.Note,
			Difficulties: strataCounts(len(ds.CQs), ds.CQs, func(c CQ) string { return c.Difficulty }),
			Kinds:        strataCounts(len(ds.CQs), ds.CQs, func(c CQ) string { return c.Kind }),
		})
	}
	return out, nil
}

// LoadDataset 读单份数据集（REQ-367 严格 schema 校验：revision 非空+双维标注齐备且枚举合法）。
func LoadDataset(path string) (*Dataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ds Dataset
	if err := json.Unmarshal(raw, &ds); err != nil {
		return nil, err
	}
	ds.Name = strings.TrimSuffix(filepath.Base(path), ".json")
	if len(ds.CQs) == 0 {
		return nil, fmt.Errorf("%s: 基准集为空", ds.Name)
	}
	if strings.TrimSpace(ds.Revision) == "" {
		return nil, fmt.Errorf("%s: 缺 revision（数据集版本口径声明必填，REQ-352）", ds.Name)
	}
	for _, c := range ds.CQs {
		if !ValidDifficulty(c.Difficulty) {
			return nil, fmt.Errorf("%s: %s difficulty %q 非法（须 easy|medium|hard）", ds.Name, c.ID, c.Difficulty)
		}
		if !ValidKind(c.Kind) {
			return nil, fmt.Errorf("%s: %s kind %q 非法（须 fact|relation|inference）", ds.Name, c.ID, c.Kind)
		}
	}
	return &ds, nil
}

// strataCounts 按取键函数分档计数（档序固定 easy<medium<hard / fact<relation<inference，稳定透出）。
func strataCounts(total int, cqs []CQ, key func(CQ) string) map[string]int {
	m := map[string]int{}
	for _, c := range cqs {
		m[key(c)]++
	}
	return m
}

// Signal 单条 CQ 评测信号（与 evaldata.cqSignal 同口径——服务化形态；REQ-367 透出双维标注供分层消费）。
type Signal struct {
	ID          string `json:"id"`
	CQ          string `json:"cq"`
	Kind        string `json:"kind,omitempty"`       // 题型（r2）
	Difficulty  string `json:"difficulty,omitempty"` // 难度（r2）
	Translated  bool   `json:"translated"`
	WhitelistOK bool   `json:"whitelist_ok"`
	GoldAligned bool   `json:"gold_aligned"`
	Query       string `json:"query,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Stratum 单档分层跑分（三率同总口径）。
type Stratum struct {
	Total      int `json:"total"`
	Translated int `json:"translated"`
	Whitelist  int `json:"whitelist"`
	GoldAlign  int `json:"gold_aligned"`
}

// MetricDefinition 口径声明（REQ-352 收编：报告自带判定/统计口径与数据集版本——防口径漂移
// 跨版本误读；与 README 口径声明节同源文案）。
type MetricDefinition struct {
	Translated   string `json:"translated"`    // 翻译率判定口径
	Whitelist    string `json:"whitelist"`     // 白名单判定口径
	GoldAligned  string `json:"gold_aligned"`  // gold 对齐判定口径（含跨域预期说明）
	Stratified   string `json:"stratified"`    // 分层口径（双维定义）
	DatasetRev   string `json:"dataset_rev"`   // 数据集版本（本报告所用）
	GeneratorVer string `json:"generator_ver"` // 评测器口径版本（r2=REQ-367 分层跑分）
}

// Report 一次「测我」运行报告（落 artifact cq-benchmark-report，runs 追加式历史）。
type Report struct {
	OntologyID string   `json:"ontology_id"`
	Version    int      `json:"version"`
	Dataset    string   `json:"dataset"`
	TS         string   `json:"ts"`
	Signals    []Signal `json:"signals"`
	Translated int      `json:"translated"`
	Whitelist  int      `json:"whitelist"`
	GoldAlign  int      `json:"gold_aligned"`
	Total      int      `json:"total"`
	// REQ-367：双维分层跑分（difficulty/kind 各档三率——分层可观测）
	ByDifficulty map[string]*Stratum `json:"by_difficulty,omitempty"`
	ByKind       map[string]*Stratum `json:"by_kind,omitempty"`
	// REQ-352 收编：口径声明随报告落库（版本对比视图消费该字段防口径漂移误读）
	MetricDefinition *MetricDefinition `json:"metric_definition,omitempty"`
	Note             string            `json:"note,omitempty"`
}

// Run 「用基准集测我」：对目标本体逐条翻译数据集 CQ（其 schema 上下文）→白名单→gold 对齐。
// gold 对齐语义=查询引用了基准 CQ 答案依赖的概念/关系 URN——跨域本体该率低是预期信号
// （基准集按种子域编写，域外本体不覆盖该域=合理的负信号，非缺陷）。
func Run(ctx context.Context, llm *llmcreate.Creator, target *pkgspec.Spec, ontologyID string, version int, ds *Dataset, onProgress func(string)) *Report {
	rep := &Report{OntologyID: ontologyID, Version: version, Dataset: ds.Name, TS: time.Now().UTC().Format(time.RFC3339), Total: len(ds.CQs)}
	conc := make([]string, 0, len(target.Concepts))
	for _, c := range target.Concepts {
		conc = append(conc, c.Name)
	}
	rel := make([]string, 0, len(target.Relations))
	for _, r := range target.Relations {
		rel = append(rel, r.Name)
	}
	for i, dc := range ds.CQs {
		if onProgress != nil {
			onProgress(fmt.Sprintf("基准 CQ %d/%d：翻译中", i+1, len(ds.CQs)))
		}
		sig := Signal{ID: dc.ID, CQ: dc.CQ, Kind: dc.Kind, Difficulty: dc.Difficulty}
		query, err := translate(ctx, llm, ontologyID, conc, rel, dc.CQ)
		if err != nil {
			// 瞬时故障重试一次（沿 cqsignal 先例）
			time.Sleep(time.Second)
			query, err = translate(ctx, llm, ontologyID, conc, rel, dc.CQ)
		}
		if err != nil {
			sig.Error = err.Error()
			rep.tally(sig)
			continue
		}
		sig.Query = query
		sig.Translated = strings.TrimSpace(query) != ""
		if sig.Translated {
			if verr := VetReadonlySelect(query); verr != nil {
				sig.Error = verr.Error()
			} else {
				sig.WhitelistOK = true
				aligned := 0
				for _, g := range dc.Gold {
					if strings.Contains(query, SanitizeURN(g)) {
						aligned++
					}
				}
				sig.GoldAligned = aligned > 0
			}
		}
		rep.tally(sig)
	}
	rep.Note = "gold 对齐=查询引用基准 CQ 依赖的概念/关系 URN；跨域本体该率低为预期负信号（基准集按源种子域编写）"
	rep.MetricDefinition = &MetricDefinition{
		Translated:   "LLM 翻译产出非空 SELECT 查询即计通过（重试一次后仍失败计失败）",
		Whitelist:    "词法级只读白名单（首关键词 SELECT+变更关键字黑名单，字符串字面量/IRI/注释不误伤）",
		GoldAligned:  "查询文本引用基准 CQ gold 概念/关系 URN（任一命中即对齐）——非答案内容比对；跨域本体该率低为预期负信号非缺陷；全枚举型 CQ 不引用具体概念 IRI 与该口径结构性不适用（REQ-348 信号边界）",
		Stratified:   "kind 题型（fact=模式枚举/relation=单跳关系/inference=多跳链或类层归约）×difficulty 难度（easy/medium/hard）双维分层；type 结构层（R/E/I）沿 CQ4OE 口径随信号透出",
		DatasetRev:   ds.Revision,
		GeneratorVer: "r2-req367",
	}
	return rep
}

// tally 三率总账+双维分层分账+信号追加（REQ-367：分层可观测——difficulty/kind 各档独立计数）。
func (r *Report) tally(sig Signal) {
	r.Signals = append(r.Signals, sig)
	if sig.Translated {
		r.Translated++
	}
	if sig.WhitelistOK {
		r.Whitelist++
	}
	if sig.GoldAligned {
		r.GoldAlign++
	}
	for dim, key := range map[string]string{"difficulty": sig.Difficulty, "kind": sig.Kind} {
		if key == "" {
			continue
		}
		var m map[string]*Stratum
		if dim == "difficulty" {
			if r.ByDifficulty == nil {
				r.ByDifficulty = map[string]*Stratum{}
			}
			m = r.ByDifficulty
		} else {
			if r.ByKind == nil {
				r.ByKind = map[string]*Stratum{}
			}
			m = r.ByKind
		}
		st, ok := m[key]
		if !ok {
			st = &Stratum{}
			m[key] = st
		}
		st.Total++
		if sig.Translated {
			st.Translated++
		}
		if sig.WhitelistOK {
			st.Whitelist++
		}
		if sig.GoldAligned {
			st.GoldAlign++
		}
	}
}

func translate(ctx context.Context, llm *llmcreate.Creator, ontologyID string, concepts, relations []string, cq string) (string, error) {
	var b strings.Builder
	b.WriteString("你是本体查询专家。把能力问题翻译为一条只读 SPARQL 1.1 SELECT 查询。\n")
	fmt.Fprintf(&b, "本体 IRI 模板：概念=<urn:o:%s:concept:名称>，关系=<urn:o:%s:relation:名称>，实例=<urn:o:%s:instance:名称>。\n", ontologyID, ontologyID, ontologyID)
	b.WriteString("可用概念：" + strings.Join(concepts, "、") + "\n")
	b.WriteString("可用关系：" + strings.Join(relations, "、") + "\n")
	b.WriteString("模式：?x a <概念IRI> 取实例；<实例IRI> <关系IRI> <实例IRI> 为对象属性断言；加 LIMIT ≤50。只输出查询全文，不要 markdown。\n\n")
	b.WriteString("能力问题：" + cq)
	reply, _, err := llm.RawChat(ctx, b.String())
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(reply)
	if idx := strings.Index(t, "```"); idx >= 0 {
		inner := strings.TrimPrefix(t[idx:], "```sparql")
		inner = strings.TrimPrefix(inner, "```")
		if end := strings.Index(inner, "```"); end >= 0 {
			inner = inner[:end]
		}
		t = strings.TrimSpace(inner)
	}
	return t, nil
}

// VetReadonlySelect 只读白名单（rest.vetReadonlySelect 同口径的服务化副本——bench 包不依赖 rest）。
func VetReadonlySelect(query string) error {
	toks := tokens(query)
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
		return fmt.Errorf("仅允许 SELECT 查询，得到首关键词 %q", toks[min(i, len(toks)-1)])
	}
	for _, t := range toks {
		switch strings.ToUpper(t) {
		case "LOAD", "CLEAR", "CREATE", "DROP", "MOVE", "COPY", "ADD", "INSERT", "DELETE", "UPDATE":
			return fmt.Errorf("含变更关键字 %q，已拒绝", t)
		}
	}
	return nil
}

func tokens(q string) []string {
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
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			cur.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return toks
}

// SanitizeURN 与 pkg/ontology/spec.Sanitize 同规则（URN IRI 对齐口径的服务化副本）。
func SanitizeURN(s string) string {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
