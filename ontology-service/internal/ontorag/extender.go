// extender.go 片段生成段（REQ-343/OE-1，64 号 §4.3 模块 D+E）：
// CQ + 检索片段（只读上下文，按来源分组渲染）→ LLM 生成 spec 片段（双 profile）→
// 两阶段校验（① 结构 spec.Validate ② 治理约束 profile 检查）——失败回喂重试 ≤2 轮，
// 仍失败标记 failed 不静默入库（论文不变式：失败片段要么重试要么标记）。
//
// prompt 资产（64 号 §2.2/§2.5 逆向，翻译中文适配；仓库无 LICENSE 零代码引入）：
// SYS_PROMPT（复用优先）/USER 7 INSTRUCTIONS/12 ImportantNotes 精选/3 pitfalls/双 profile 指令。
package ontorag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pkgspec "github.com/ontoagents/ontoagents/pkg/ontology/spec"

	"github.com/ontoagents/ontoagents/ontology-service/internal/llmcreate"
)

// 生成 profile（论文双部署 profile 适配 spec_json 形态）：
//   - structure 结构型（对应论文 SHACL profile）：只要求概念/关系/数据属性声明齐备（关系显式
//     from/to=显式 domain/range 治理约束的 spec 形态），不要求公理——下游确定性消费结构。
//   - restriction 限制型（对应论文 EU 通用限制 profile）：结构齐备之上用 parents 层级与
//     axioms（disjoint_with/equivalent_class）表达 CQ 暗示的约束，要求 label+definition 齐备
//     （=rdfs:label/rdfs:comment 要求）。
const (
	ProfileStructure   = "structure"
	ProfileRestriction = "restriction"
)

// ValidProfile profile 合法性。
func ValidProfile(p string) bool {
	return p == ProfileStructure || p == ProfileRestriction
}

// fragmentSchema 片段生成输出契约（spec_json 片段子集）。
const fragmentSchema = `{"type":"object","required":["concepts","relations"],"properties":{
"name":{"type":"string"},
"concepts":{"type":"array","items":{"type":"object","required":["name","label","definition"],"properties":{"name":{"type":"string"},"label":{"type":"string"},"definition":{"type":"string"},"parents":{"type":"array","items":{"type":"string"}}}}},
"relations":{"type":"array","items":{"type":"object","required":["name","label","definition","from","to"],"properties":{"name":{"type":"string"},"label":{"type":"string"},"definition":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"}}}},
"data_properties":{"type":"array","items":{"type":"object","required":["name","label","definition"],"properties":{"name":{"type":"string"},"label":{"type":"string"},"definition":{"type":"string"},"domain":{"type":"string"},"range":{"type":"string"}}}},
"axioms":{"type":"array","items":{"type":"object","required":["type","subject","targets"],"properties":{"type":{"type":"string","enum":["disjoint_with","equivalent_class"]},"subject":{"type":"string"},"targets":{"type":"array","items":{"type":"string"}}}}}}}`

// extenderSystem 系统提示（论文 SYS_PROMPT 中文适配：复用优先+只输出 JSON）。
const extenderSystem = `你是本体工程师。请尽可能复用已提供目标本体中的元素；确需新建元素时保持与目标本体一致的命名风格。只输出一个 JSON 对象（spec 片段），不要 markdown 代码块或其他文本。`

// extenderUser 用户提示模板（论文 USER_TMPL 要点翻译：任务声明/复用优先/只读上下文/指令/注意事项/陷阱）。
const extenderUser = `任务：基于能力问题（CQ）与目标本体上下文，生成建模该 CQ 的本体片段——要求未来存在 SPARQL 查询能从该本体取出这条 CQ 的答案。

目标本体已有元素（只读上下文——优先复用，不要重复声明同名元素）：
{elements}

能力问题：{cq}

建模指令：
1. 分析 CQ 所需的概念/关系/数据属性；
2. 映射到已有元素清单——优先复用语义相似度高的元素，非必要不新建；
3. 只输出 TBox（概念/关系/数据属性/公理），禁止输出实例（instances）；
4. CQ 暗示约束时才添加公理；参考本体未表达类似约束时不必强行添加；
5. 每条关系（relations[]）的 from/to 必须显式给出，且引用已定义概念名；
6. 语法正确且自包含：片段引用的概念名，要么与目标本体已有元素同名（=复用），要么在本片段 concepts 内定义。

重要注意事项：
- 概念层级用 concepts[].parents 特化（引用已定义概念名）；对象连接用 relations 的 from/to；
- 目标本体已有的元素不要再次声明（复用即引用其名称）；
- 每个新建元素写 label 与 definition；
- 命名风格与目标本体保持一致，避免「hasName」vs「has_name」式同义异名；
- 仅 CQ 需要时才添加公理，不新建无依据的逆向关系；
- 需要关系自身携带属性/身份时，优先具体化为关联概念；
- 宁缺毋滥：不要为凑数生成与 CQ 无关的元素。

常见陷阱（必须避免）：
1. 禁止 concepts[].parents 循环引用；
2. 每个新建概念/关系/数据属性至少有 label 与 definition；
3. 复用已有关系时保持其 from/to 原方向，不要臆改。

{profileDirective}

只输出 JSON 对象：{"name":"片段名","concepts":[...],"relations":[...],"data_properties":[...],"axioms":[...]}（无内容的数组留空）。`

// profileDirectives 双 profile 指令（论文 style directive 位）。
var profileDirectives = map[string]string{
	ProfileStructure:   "建模形式（structure 结构型）：只要求概念/关系/数据属性声明齐备（关系显式 from/to），不要输出公理（axioms 留空）。",
	ProfileRestriction: "建模形式（restriction 限制型）：结构齐备之上，用 parents 层级与 axioms（disjoint_with/equivalent_class）表达 CQ 暗示的约束；每个 data_properties[].domain 必填。",
}

// RenderExtenderPrompt 组装生成 prompt（elements 按来源分组渲染——单目标本体场景即一组）。
// 返回（user prompt, 命中元素的可读摘要清单）。
func RenderExtenderPrompt(cq string, hits []Hit, profile string) string {
	var b strings.Builder
	if len(hits) == 0 {
		b.WriteString("（检索无命中——目标本体中没有语义相近元素，请全部新建并保持命名风格一致）")
	} else {
		for i, h := range hits {
			fmt.Fprintf(&b, "%d. %s\n   ```json\n   %s\n   ```\n", i+1, h.Element.SearchText(), h.Element.Snippet)
		}
	}
	pd := profileDirectives[profile]
	if pd == "" {
		pd = profileDirectives[ProfileStructure]
	}
	p := strings.Replace(extenderUser, "{elements}", b.String(), 1)
	p = strings.Replace(p, "{cq}", cq, 1)
	p = strings.Replace(p, "{profileDirective}", pd, 1)
	return p
}

// GenerateResult 单条 CQ 的生成结果。
type GenerateResult struct {
	CQ       string        `json:"cq"`
	Status   string        `json:"status"` // ok | failed
	Fragment *FragmentSpec `json:"fragment,omitempty"`
	Error    string        `json:"error,omitempty"` // failed 时的人话原因（两阶段校验明细）
	Rounds   int           `json:"rounds"`          // 生成-校验轮数
	Used     []Hit         `json:"used,omitempty"`  // 检索命中（检索日志/前端展示）
}

// Generate 单条 CQ 生成+两阶段校验主循环（回喂重试 ≤2 轮；仍失败标记 failed）。
func Generate(ctx context.Context, llm *llmcreate.Creator, target *pkgspec.Spec, cq, profile string, hits []Hit, onProgress func(msg string)) *GenerateResult {
	res := &GenerateResult{CQ: cq, Used: hits}
	prompt := RenderExtenderPrompt(cq, hits, profile)
	const maxRounds = 2
	for round := 1; round <= maxRounds; round++ {
		if onProgress != nil {
			onProgress(fmt.Sprintf("生成片段（第 %d/%d 轮）", round, maxRounds))
		}
		raw, _, err := llm.Chat(ctx, prompt, fragmentSchema)
		if err != nil {
			res.Status = "failed"
			res.Error = "模型调用失败: " + err.Error()
			return res
		}
		var frag FragmentSpec
		if perr := json.Unmarshal([]byte(raw), &frag); perr != nil {
			if round == maxRounds {
				res.Status = "failed"
				res.Error = fmt.Sprintf("片段不是合法 JSON（%s）——标记失败不静默入库", perr)
				return res
			}
			if onProgress != nil {
				onProgress("输出不是合法 JSON，回喂重试")
			}
			prompt += "\n\n注意：上一次输出不是合法 JSON 对象（" + perr.Error() + "）。请严格只输出该 JSON 结构。"
			continue
		}
		// 两阶段校验①：结构 + 引用完整性——对「目标本体∪片段」合并视图校验
		// （片段复用已有元素只引用名称不重复声明，对片段单独校验会误报悬空引用；
		// 重复声明已有元素则合并视图撞名报「名称重复」=强复用约束回喂修正）
		if errs := ValidateAgainstTarget(target, &frag); len(errs) > 0 {
			msgs := make([]string, 0, len(errs))
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			if round == maxRounds {
				res.Status = "failed"
				res.Error = fmt.Sprintf("片段结构校验未通过（%d 处）：%s——标记失败不静默入库", len(errs), strings.Join(msgs, "；"))
				return res
			}
			if onProgress != nil {
				onProgress(fmt.Sprintf("片段结构校验 %d 处问题，回喂重试", len(errs)))
			}
			prompt += "\n\n上一稿片段存在以下校验错误，请修正后重新输出完整片段：\n- " + strings.Join(msgs, "\n- ")
			continue
		}
		// 两阶段校验②：治理约束（论文 profile 治理约束的 spec 形态——
		// relation 显式 from/to 已由①引用完整性强制；restriction profile 附加 data_property.domain 必填）
		if profile == ProfileRestriction {
			var missing []string
			for _, dp := range frag.DataProperties {
				if strings.TrimSpace(dp.Domain) == "" {
					missing = append(missing, dp.Name)
				}
			}
			if len(missing) > 0 {
				if round == maxRounds {
					res.Status = "failed"
					res.Error = fmt.Sprintf("限制型 profile 治理约束：数据属性缺 domain（%s）——标记失败不静默入库", strings.Join(missing, "、"))
					return res
				}
				if onProgress != nil {
					onProgress(fmt.Sprintf("%d 个数据属性缺 domain，回喂重试", len(missing)))
				}
				prompt += "\n\n上一稿片段中以下数据属性缺少 domain（限制型 profile 要求必填）：" + strings.Join(missing, "、") + "。请补全后重新输出完整片段。"
				continue
			}
		}
		res.Status = "ok"
		res.Fragment = &frag
		res.Rounds = round
		return res
	}
	res.Status = "failed"
	res.Error = "生成循环异常退出"
	return res
}
