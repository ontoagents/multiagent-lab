// precheck.go CQ 质量预检（REQ-344/OE-5，83 号 N9 信号：「框架不能补救坏需求」——工业聚焦 CQ
// Correctness 4.91 vs EU 开放 CQ 3.66 的核心教训）。CQ 驱动扩展前置：LLM 逐维评估候选 CQ 的
// ①可答性（本体 schema 能否支撑结构化答案）②原子性（单一焦点，复合问题致片段发散）
// ③具体性（开放/欠定义预警——建模决策全留生成器=EU 场景低分根因）。
// 口径：预检是信号不是闸门——低分 CQ 警告透出、不阻断扩展（工程师决策；沿「机器产报告人做决策」）。
package ontorag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

// precheckSchema 预检输出契约（逐条三 维布尔+一句话依据）。
const precheckSchema = `{"type":"object","required":["items"],"properties":{"items":{"type":"array","items":{"type":"object",
"required":["cq","answerable","atomic","specific"],"properties":{
"cq":{"type":"string"},
"answerable":{"type":"boolean","description":"本体 schema（概念/关系/数据属性）能否结构化支撑该问题的答案"},
"atomic":{"type":"boolean","description":"是否单一焦点（复合多焦点问题=false）"},
"specific":{"type":"boolean","description":"是否具体可建模（开放/欠定义/无明确答案边界=false）"},
"note":{"type":"string","description":"一句话依据（指出缺什么概念/关系，或复合了哪些焦点）"}}}}}}`

// precheckPrompt 预检提示词（N9 信号三维度）。
const precheckPrompt = `你是本体需求评审专家。下方是待用于驱动本体扩展的能力问题（CQ）清单与本体现有 schema。请逐条评估每个 CQ 的质量：

1. answerable 可答性：以现有 schema（可用概念/关系/数据属性）加上合理的新建建模，该问题能否有结构化的答案形态？（完全超出本体领域=不可答）
2. atomic 原子性：问题是否单一焦点？（「A 的 X 和 Y 分别是什么」这类复合问题=false，应拆分）
3. specific 具体性：问题是否具体到可以建模？（「本体应该涵盖什么」这类开放/欠定义问题=false）

注意：评估针对问题质量本身，不要因为「当前 schema 还没有答案所需概念」就判不可答——新建建模可覆盖的算可答；只有完全脱离本体领域才判不可答。

只输出 JSON 对象，不要 markdown 代码块或其他文本。

本体 schema：
- 概念：{concepts}
- 关系：{relations}
- 数据属性：{dataProperties}

CQ 清单：
{cqs}`

// PrecheckItem 单条 CQ 预检信号。
type PrecheckItem struct {
	CQ         string `json:"cq"`
	Answerable bool   `json:"answerable"`
	Atomic     bool   `json:"atomic"`
	Specific   bool   `json:"specific"`
	Note       string `json:"note,omitempty"`
}

// PrecheckReport 预检报告（透出 cqrag job result；信号非闸门）。
type PrecheckReport struct {
	Items         []PrecheckItem `json:"items"`
	NotAnswerable int            `json:"not_answerable"`
	NotAtomic     int            `json:"not_atomic"`
	NotSpecific   int            `json:"not_specific"`
	Note          string         `json:"note,omitempty"`
}

// Precheck CQ 质量预检主循环（解析失败回喂 ≤2 轮；失败降级 nil=无信号不阻断——预检是增强不是依赖）。
func Precheck(ctx context.Context, llm *llmcreate.Creator, sp *pkgspec.Spec, cqs []string, onProgress func(string)) *PrecheckReport {
	if len(cqs) == 0 {
		return nil
	}
	concepts := make([]string, 0, len(sp.Concepts))
	for _, c := range sp.Concepts {
		concepts = append(concepts, c.Name)
	}
	relations := make([]string, 0, len(sp.Relations))
	for _, r := range sp.Relations {
		relations = append(relations, r.Name)
	}
	dps := make([]string, 0, len(sp.DataProperties))
	for _, d := range sp.DataProperties {
		dps = append(dps, d.Name)
	}
	items := make([]string, len(cqs))
	for i, q := range cqs {
		items[i] = fmt.Sprintf("%d. %s", i+1, q)
	}
	prompt := strings.Replace(strings.Replace(strings.Replace(strings.Replace(
		precheckPrompt,
		"{concepts}", fallback(concepts, "（无）"), 1),
		"{relations}", fallback(relations, "（无）"), 1),
		"{dataProperties}", fallback(dps, "（无）"), 1),
		"{cqs}", strings.Join(items, "\n"), 1)
	for round := 1; round <= 2; round++ {
		if onProgress != nil {
			onProgress(fmt.Sprintf("CQ 质量预检（第 %d/2 轮）", round))
		}
		raw, _, err := llm.Chat(ctx, prompt, precheckSchema)
		if err != nil {
			return nil // 模型通道失败降级：无信号不阻断
		}
		var out struct {
			Items []PrecheckItem `json:"items"`
		}
		if perr := json.Unmarshal([]byte(raw), &out); perr != nil || len(out.Items) == 0 {
			if round == 2 {
				return nil
			}
			prompt += "\n\n注意：上一次输出不是合法 JSON 对象，请严格只输出该 JSON 结构。"
			continue
		}
		rep := &PrecheckReport{Items: out.Items}
		// 与输入清单对齐（模型漏项补 Unknown 形态=false 三维+漏评标注；按构造只对齐一次）
		byCQ := map[string]*PrecheckItem{}
		for i := range rep.Items {
			byCQ[strings.TrimSpace(rep.Items[i].CQ)] = &rep.Items[i]
		}
		aligned := make([]PrecheckItem, 0, len(cqs))
		for _, q := range cqs {
			if it, ok := byCQ[strings.TrimSpace(q)]; ok {
				aligned = append(aligned, *it)
			} else {
				aligned = append(aligned, PrecheckItem{CQ: q, Note: "（漏评——按不通过保守计）"})
			}
		}
		rep.Items = aligned
		for _, it := range rep.Items {
			if !it.Answerable {
				rep.NotAnswerable++
			}
			if !it.Atomic {
				rep.NotAtomic++
			}
			if !it.Specific {
				rep.NotSpecific++
			}
		}
		if rep.NotAnswerable+rep.NotAtomic+rep.NotSpecific > 0 {
			rep.Note = "预检为质量信号非闸门：低分 CQ 仍将执行扩展（结果供工程师决策参考）；建议复合问题拆分后再试。"
		}
		return rep
	}
	return nil
}

func fallback(list []string, empty string) string {
	if len(list) == 0 {
		return empty
	}
	return strings.Join(list, "、")
}
