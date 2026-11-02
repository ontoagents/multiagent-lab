package datasource

// LLM 窄调用（REQ-595 管线步④，99 号 v1.2 手法 A1/A2/A4/A5 借鉴）：
// A5 窄职责拆分=显示名/描述、隐含 FK 三类各独立调用互不污染；A1 画像证据注入=采样值/唯一性/
// 枚举集喂进 prompt 不让模型猜；A4 保守范式=不改结构名/只取明示/显式排除清单；A2 消毒+确定性回退=
// 输出逐项白名单校验（表/列必须真实存在），失败回退规则结果，批永不中断。
// 温度随模型连接配置（平台代理通道不单独注入——与 llmcreate 既有口径一致）；FK 建议必人审。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

// refineStats LLM 窄调用结果统计。
type refineStats struct {
	Used          bool
	Skipped       string
	Fallback      bool // 任一批两轮修复仍失败→该批回退确定性结果（如实注记）
	FKSuggestions []FKSuggestion
	Warnings      []string
}

// 窄调用批上限（防 prompt 膨胀；批内失败只回退该批）。
const (
	refineBatchTables   = 15 // 概念显示名/描述批
	refineBatchProps    = 20 // 数据属性描述批
	refineFKSuggestMax  = 20 // 隐含 FK 建议条数上限
	refineLabelMax      = 60
	refineDefinitionMax = 200
)

const refineNameSchema = `{"type":"object","required":["items"],"properties":{"items":{"type":"array","items":{"type":"object","required":["key"],"properties":{"key":{"type":"string"},"label":{"type":"string"},"definition":{"type":"string"}}}}}}`

const refineFKSchema = `{"type":"object","required":["items"],"properties":{"items":{"type":"array","items":{"type":"object","required":["table","column","target_table"],"properties":{"table":{"type":"string"},"column":{"type":"string"},"target_table":{"type":"string"},"reason":{"type":"string"}}}}}}`

// refine LLM 窄调用编排（creator 非空才进入；三类批各自独立、失败互不传染）。
func refine(ctx context.Context, creator *llmcreate.Creator, schema *Schema, profiles []TableProfile, sk *skeletonBuildResult, baseWarnings []string) refineStats {
	st := refineStats{Warnings: append([]string{}, baseWarnings...)}
	// 消毒底册：真实表/列白名单（A2——LLM 输出逐项对照，幻觉名丢弃）
	tableCols := map[string]map[string]bool{}
	var tableList []string
	for _, td := range schema.Tables {
		tableList = append(tableList, td.Name)
		cols := map[string]bool{}
		for _, c := range td.Columns {
			cols[c.Name] = true
		}
		tableCols[td.Name] = cols
	}

	// 批①：概念显示名+业务描述（≤15 表/批）
	tableLabel := map[string]string{}   // 表名 → LLM 显示名
	tableDesc := map[string]string{}    // 表名 → LLM 业务描述
	concepts := sk.Spec.Concepts
	for start := 0; start < len(concepts); start += refineBatchTables {
		end := start + refineBatchTables
		if end > len(concepts) {
			end = len(concepts)
		}
		batch := concepts[start:end]
		ok := refineNameBatch(ctx, creator, "表", batchKeysConcept(batch, profiles), tableCols, st.wfn(), func(key, label, definition string) {
			tableLabel[key] = label
			tableDesc[key] = definition
		})
		if !ok {
			st.Fallback = true
		}
	}
	// 批②：数据属性业务描述（≤20/批）——key 形态 "表.列"
	propDesc := map[string]string{} // "表.列" → 描述
	props := sk.Spec.DataProperties
	for start := 0; start < len(props); start += refineBatchProps {
		end := start + refineBatchProps
		if end > len(props) {
			end = len(props)
		}
		batch := props[start:end]
		ok := refineNameBatch(ctx, creator, "数据属性", propKeys(batch, schema), tableCols, st.wfn(), func(key, _, definition string) {
			propDesc[key] = definition
		})
		if !ok {
			st.Fallback = true
		}
	}
	// 应用回骨架（消毒：key 必须命中真实概念/属性——白名单过滤，幻觉丢弃）
	for i := range sk.Spec.Concepts {
		c := &sk.Spec.Concepts[i]
		// 概念 key=原表名（Label 初始=表名）——先存原键再写显示名（防覆盖后查键落空）
		orig := c.Label
		if lbl, ok := tableLabel[orig]; ok && lbl != "" {
			c.Label = truncateRunes(lbl, refineLabelMax)
		}
		if d, ok := tableDesc[orig]; ok && d != "" {
			c.Definition = truncateRunes(d, refineDefinitionMax)
		}
	}
	for i := range sk.Spec.DataProperties {
		dp := &sk.Spec.DataProperties[i]
		key := dp.Domain + "." + dp.Label // Domain=概念名（=原表名）、Label=原始列名
		if d, ok := propDesc[key]; ok && d != "" {
			dp.Definition = truncateRunes(d, refineDefinitionMax)
		}
	}

	// 批③：隐含 FK 建议（保守 prompt：_id 后缀+唯一性证据+显式排除清单；输出白名单过滤）
	sugg := refineFKBatch(ctx, creator, schema, tableCols, profiles, st.wfn())
	st.FKSuggestions = sugg
	st.Used = true
	return st
}

// wfn warning 收集器。
func (st *refineStats) wfn() func(string) {
	return func(msg string) { st.Warnings = append(st.Warnings, msg) }
}

// batchKeysConcept 概念批素材（key=表名即 Label；证据=列画像摘录）。
func batchKeysConcept(batch []pkgspec.Concept, profiles []TableProfile) []nameItem {
	idx := map[string]TableProfile{}
	for _, tp := range profiles {
		idx[tp.Table] = tp
	}
	items := make([]nameItem, 0, len(batch))
	for _, c := range batch {
		it := nameItem{Key: c.Label} // Label=原始表名
		if tp, ok := idx[c.Label]; ok {
			it.Evidence = profileEvidence(&tp)
		}
		items = append(items, it)
	}
	return items
}

// propKeys 属性批素材（key="表.列"；Domain=概念名、Label=原始列名）。
func propKeys(batch []pkgspec.DataProperty, schema *Schema) []nameItem {
	items := make([]nameItem, 0, len(batch))
	for _, dp := range batch {
		items = append(items, nameItem{Key: dp.Domain + "." + dp.Label, Evidence: "类型 " + mapRangeLabel(dp.Range)})
	}
	return items
}

func mapRangeLabel(r string) string {
	if r == "" {
		return "string"
	}
	return r
}

// nameItem 窄调用素材条目。
type nameItem struct {
	Key      string `json:"key"`
	Evidence string `json:"evidence,omitempty"`
}

// refineNameBatch 通用命名批：prompt+调用+两轮修复+白名单消毒。
// apply 回调仅对通过消毒的条目触发；返回 false=两轮后仍失败（调用方注记回退）。
func refineNameBatch(ctx context.Context, creator *llmcreate.Creator, kind string, items []nameItem, tableCols map[string]map[string]bool, warn func(string), apply func(key, label, definition string)) bool {
	if len(items) == 0 {
		return true
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("你是本体建模工程师。以下是一组数据库%s（来自 SQLite 只读内省），请为每条给出中文显示名（label）与一句话业务描述（definition）。\n\n要求：\n- 只依据给出的名称与采样证据，不虚构证据外的业务含义；没有把握就写中性描述（如「%s 的标识」）或不写；\n- 不得修改 key 本身；label 不超过 %d 字；definition 不超过 %d 字；\n- 证据中列出 top 高频值/唯一性/枚举候选/日期格式，供你判断业务含义；\n- 只输出 JSON 对象（items 数组，每条含 key/label/definition），不要 markdown 代码块。\n\n%s清单：\n", kind, kind, refineLabelMax, refineDefinitionMax, kind))
	for _, it := range items {
		fmt.Fprintf(&b, "- key=%s", it.Key)
		if it.Evidence != "" {
			fmt.Fprintf(&b, "｜证据：%s", it.Evidence)
		}
		b.WriteString("\n")
	}
	prompt := b.String()
	var lastErr string
	for round := 0; round < 2; round++ {
		p := prompt
		if lastErr != "" {
			p += "\n\n上一稿问题：" + lastErr + "，请修正后重新输出完整 JSON。"
		}
		raw, _, err := creator.Chat(ctx, p, refineNameSchema)
		if err != nil {
			warn(fmt.Sprintf("%s 显示名/描述批调用失败：%v（该批回退确定性结果）", kind, err))
			return false
		}
		var parsed struct {
			Items []struct {
				Key        string `json:"key"`
				Label      string `json:"label"`
				Definition string `json:"definition"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
			lastErr = "输出不是合法 JSON: " + err.Error()
			continue
		}
		got := 0
		for _, it := range parsed.Items {
			// A2 消毒：key 必须命中批内素材（防幻觉/越批）；长度截断
			for _, src := range items {
				if src.Key != it.Key {
					continue
				}
				apply(it.Key, truncateRunes(strings.TrimSpace(it.Label), refineLabelMax), truncateRunes(strings.TrimSpace(it.Definition), refineDefinitionMax))
				got++
				break
			}
		}
		if got > 0 {
			return true
		}
		lastErr = "items 为空或 key 全部未命中清单"
	}
	warn(fmt.Sprintf("%s 显示名/描述批两轮未获有效输出（该批回退确定性结果）", kind))
	return false
}

// refineFKBatch 隐含 FK 建议批（保守 prompt A4：候选列=未声明 FK 的疑似引用列；
// 显式排除=主键/已声明 FK/度量数值/布尔/时间戳；建议条数上限护栏；输出逐条白名单过滤）。
func refineFKBatch(ctx context.Context, creator *llmcreate.Creator, schema *Schema, tableCols map[string]map[string]bool, profiles []TableProfile, warn func(string)) []FKSuggestion {
	// 候选池：列名以 _id/id 结尾且非主键且未声明 FK；目标猜测=去 _id 后匹配真实表名
	declared := map[string]bool{} // "table.column"
	for _, td := range schema.Tables {
		for _, fk := range td.FKs {
			declared[td.Name+"."+fk.Column] = true
		}
	}
	tableSet := map[string]bool{}
	for _, td := range schema.Tables {
		tableSet[td.Name] = true
	}
	profIdx := map[string]TableProfile{}
	for _, tp := range profiles {
		profIdx[tp.Table] = tp
	}
	type cand struct{ table, column string }
	var candidates []cand
	for _, td := range schema.Tables {
		for _, c := range td.Columns {
			if c.IsPK || declared[td.Name+"."+c.Name] {
				continue
			}
			low := strings.ToLower(c.Name)
			// 保守候选池：_id 后缀 / id·uuid·guid·uid 本名 / camel 的 Id 后缀
			// （防 paid/valid 等伪 id 噪声；进一步排除交由证据证据与人工裁决）
			if strings.HasSuffix(low, "_id") || low == "id" || low == "uuid" || low == "guid" || low == "uid" ||
				(strings.HasSuffix(c.Name, "Id") && len(c.Name) > 2) {
				candidates = append(candidates, cand{td.Name, c.Name})
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("你是本体建模工程师。以下数据库存在「命名上疑似外键但未声明 FOREIGN KEY 约束」的列，请判断哪些真正构成表间引用关系。\n\n要求（保守判断）：\n- 只在目标表确实存在于清单中时才建议；宁缺毋滥，不确定就不要输出；\n- 数值度量列（金额/数量/计数）、名称列、时间戳列、布尔列即使名字带数字后缀也不是外键；\n- reason 用一句话说明依据（命名对应/采样值证据）；\n- 只输出 JSON 对象（items 数组：table/column/target_table/reason），不要 markdown 代码块。\n\n表清单：" + strings.Join(sortedKeys(tableSet), "、") + "\n\n候选列：\n")
	for _, c := range candidates {
		ev := ""
		if tp, ok := profIdx[c.table]; ok {
			for _, cp := range tp.Columns {
				if cp.Column == c.column {
					ev = profileEvidenceCol(&cp)
					break
				}
			}
		}
		if ev != "" {
			fmt.Fprintf(&b, "- %s.%s｜证据：%s\n", c.table, c.column, ev)
		} else {
			fmt.Fprintf(&b, "- %s.%s\n", c.table, c.column)
		}
	}
	raw, _, err := creator.Chat(ctx, b.String(), refineFKSchema)
	if err != nil {
		warn(fmt.Sprintf("隐含 FK 建议批调用失败：%v（跳过——声明 FK 不受影响）", err))
		return nil
	}
	var parsed struct {
		Items []struct {
			Table       string `json:"table"`
			Column      string `json:"column"`
			TargetTable string `json:"target_table"`
			Reason      string `json:"reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		warn("隐含 FK 建议批输出不是合法 JSON（跳过——声明 FK 不受影响）")
		return nil
	}
	var out []FKSuggestion
	seen := map[string]bool{}
	for _, it := range parsed.Items {
		// A2 白名单过滤：表/列/目标必须真实存在；不得重复；不得与声明 FK 重叠；上限护栏
		if !tableSet[it.Table] || !tableSet[it.TargetTable] {
			continue
		}
		if !tableCols[it.Table][it.Column] {
			continue
		}
		if declared[it.Table+"."+it.Column] {
			continue
		}
		key := it.Table + "." + it.Column
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, FKSuggestion{
			Table: it.Table, Column: it.Column, TargetTable: it.TargetTable,
			Reason: truncateRunes(strings.TrimSpace(it.Reason), 120),
		})
		if len(out) >= refineFKSuggestMax {
			warn(fmt.Sprintf("隐含 FK 建议超上限 %d 条，已截断", refineFKSuggestMax))
			break
		}
	}
	return out
}

// profileEvidence 表级证据摘录（喂 prompt 用，紧凑单行）。
func profileEvidence(tp *TableProfile) string {
	if tp.Error != "" || len(tp.Columns) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	for i, cp := range tp.Columns {
		if i >= 4 {
			parts = append(parts, fmt.Sprintf("…共 %d 列", len(tp.Columns)))
			break
		}
		parts = append(parts, profileEvidenceCol(&cp))
	}
	return fmt.Sprintf("采样 %d 行；%s", tp.SampledRows, strings.Join(parts, "；"))
}

// profileEvidenceCol 列级证据摘录。
func profileEvidenceCol(cp *ColumnProfile) string {
	var bits []string
	if cp.IsUnique {
		bits = append(bits, "采样内唯一")
	}
	if len(cp.EnumCandidates) > 0 && len(cp.EnumCandidates) <= 8 {
		bits = append(bits, "枚举候选["+strings.Join(cp.EnumCandidates, "|")+"]")
	} else if len(cp.TopValues) > 0 {
		tv := make([]string, 0, len(cp.TopValues))
		for _, v := range cp.TopValues {
			tv = append(tv, v.Value)
		}
		bits = append(bits, "高频值["+strings.Join(tv, "|")+"]")
	}
	if len(cp.DateFormats) > 0 {
		bits = append(bits, "日期格式 "+cp.DateFormats[0])
	}
	res := cp.Column
	if len(bits) > 0 {
		res += "(" + strings.Join(bits, ",") + ")"
	}
	return res
}

// truncateRunes 按字符截断（prompt 长度护栏）。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// sortedKeys map 键排序（prompt 稳定序）。
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
