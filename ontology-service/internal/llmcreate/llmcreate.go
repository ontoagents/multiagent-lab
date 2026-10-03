// Package llmcreate LLM 辅助创建（REQ-82）：生成-校验循环（最多 3 轮），草稿必须经用户预览确认。
// 模型能力归主平台（/api/ontology-llm/generate 代理），校验归构建平面（与人工编辑共用 Validate）。
// REQ-171 P1 循环升级（26 号方案 §9 P1 / 设计铁律）：校验源从结构 Validate 扩展为
// 「结构 Validate + 质量门禁 qualitygate」双源——错误级命中回喂修复（≤3 轮），
// 3 轮未过转人工预览（既有语义），全程不静默放行。
package llmcreate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/qualitygate"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/seed"
)

const specSchemaHint = `{
  "type": "object",
  "required": ["name", "concepts", "relations", "instances"],
  "properties": {
    "name": {"type": "string"},
    "description": {"type": "string"},
    "concepts": {"type": "array", "items": {"type": "object", "required": ["name"], "properties": {
      "name": {"type": "string"}, "label": {"type": "string"}, "definition": {"type": "string"},
      "parents": {"type": "array", "items": {"type": "string"}}}}},
    "relations": {"type": "array", "items": {"type": "object", "required": ["name", "from", "to"], "properties": {
      "name": {"type": "string"}, "label": {"type": "string"}, "definition": {"type": "string"},
      "from": {"type": "string"}, "to": {"type": "string"}}}},
    "data_properties": {"type": "array", "items": {"type": "object", "required": ["name"], "properties": {
      "name": {"type": "string"}, "label": {"type": "string"}, "definition": {"type": "string"},
      "domain": {"type": "string"}, "range": {"type": "string"}}}},
    "instances": {"type": "array", "items": {"type": "object", "required": ["name", "concept"], "properties": {
      "name": {"type": "string"}, "concept": {"type": "string"},
      "attributes": {"type": "object"},
      "relations": {"type": "array", "items": {"type": "object", "required": ["rel", "target"], "properties": {
        "rel": {"type": "string"}, "target": {"type": "string"}}}}}}}
  }
}`

type Creator struct {
	PlatformURL string // 主平台地址（/api/ontology-llm/generate）
	HTTP        *http.Client
	MaxRounds   int
}

func New(platformURL string) *Creator {
	return &Creator{PlatformURL: platformURL, HTTP: &http.Client{Timeout: 120 * time.Second}, MaxRounds: 3}
}

// GenerateResult 生成-校验循环结果。
type GenerateResult struct {
	Spec        *pkgspec.Spec       `json:"spec"`
	Rounds      int                 `json:"rounds"`
	Usage       any                 `json:"usage,omitempty"`
	Quality     *qualitygate.Report `json:"quality,omitempty"` // REQ-171 P1：终稿质量报告（告警级命中不阻断但透出）
	QualityPass bool                `json:"quality_pass"`      // 错误级命中是否清零（false=已达轮数上限转人工）
}

// Draft 领域描述 → spec_json 草稿；校验失败把错误列表回喂模型修正。
func (c *Creator) Draft(description, extraHint string) (*GenerateResult, error) {
	return c.DraftWithCQ(description, extraHint, nil)
}

// DraftWithCQ REQ-248/G2：CQ 显式传入——并入 prompt 且**回写进草案 spec.CQ**（能力问题入资产可追溯）。
func (c *Creator) DraftWithCQ(description, extraHint string, cqs []string) (*GenerateResult, error) {
	if len(cqs) > 0 {
		var b strings.Builder
		b.WriteString(extraHint)
		b.WriteString("\n\n请重点让本体具备回答以下能力问题的潜力（据此补充概念/关系/属性建模）：")
		for i, q := range cqs {
			fmt.Fprintf(&b, "\n%d. %s", i+1, strings.TrimSpace(q))
		}
		extraHint = b.String()
	}
	prompt := buildPrompt(description, extraHint, nil, cqs)
	var usage any
	for round := 1; round <= c.MaxRounds; round++ {
		draftRaw, u, err := c.callGenerate(prompt)
		if err != nil {
			return nil, err
		}
		if u != nil {
			usage = u
		}
		var sp pkgspec.Spec
		if err := json.Unmarshal([]byte(draftRaw), &sp); err != nil {
			// 结构坏：把解析错误回喂
			prompt = buildPrompt(description, extraHint, []string{"输出不是合法 spec_json: " + err.Error() + "。请只输出 JSON 本体，不要多余文本。"}, cqs)
			continue
		}
		errs := sp.Validate()
		if len(errs) == 0 {
			// REQ-171 P1：结构合法后过质量门禁——错误级命中回喂修复，告警级透出不阻断
			rep := qualitygate.Check(&sp, nil)
			if fix := rep.ErrorMessages(); len(fix) == 0 {
				sp.CQ = append(sp.CQ, cqs...) // REQ-248：CQ 回写资产
				return &GenerateResult{Spec: &sp, Rounds: round, Usage: usage, Quality: rep, QualityPass: true}, nil
			} else if round == c.MaxRounds {
				return &GenerateResult{Spec: &sp, Rounds: round, Usage: usage, Quality: rep, QualityPass: false},
					fmt.Errorf("已达最大修正轮数，仍有 %d 处质量门禁错误级问题，草稿转人工确认（铁律：不静默放行）", len(fix))
			} else {
				// REQ-247/G4：warning 级命中也纳入回喂（拍板口径——提升草案质量，不设阈值不阻断）
				if warns := rep.WarningMessages(); len(warns) > 0 {
					fix = append(fix, warns...)
				}
				fix = append(fix, "（以上为质量门禁检查，请修正后重新输出完整 spec_json）")
				prompt = buildPrompt(description, extraHint, fix, cqs)
				continue
			}
		}
		if round == c.MaxRounds {
			sp.CQ = append(sp.CQ, cqs...)
			return &GenerateResult{Spec: &sp, Rounds: round, Usage: usage},
				fmt.Errorf("已达最大修正轮数，仍有 %d 处校验问题，草稿供预览参考", len(errs))
		}
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		prompt = buildPrompt(description, extraHint, msgs, cqs)
	}
	return nil, fmt.Errorf("生成循环异常退出")
}


// fewShotFor REQ-247/G4：按领域描述关键词挑选最相近种子本体的紧凑片段作 few-shot 范例
// （种子 5 份内置编译期；关键词命中失败返回空串——范例注入是增强不是依赖）。
// buildFewShot 生成范例段（截断防 prompt 膨胀；概念取前 4、关系取前 3）。
func buildFewShot(description string) string {
	pick := pickSeed(description)
	if pick == "" {
		return ""
	}
	raw, err := seed.ExampleRaw(strings.TrimSuffix(pick, ".json"))
	if err != nil {
		return ""
	}
	var sp pkgspec.Spec
	if json.Unmarshal(raw, &sp) != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n参考范例（同领域种子本体的结构与粒度，仅供参照——不要照抄概念名）：\n")
	b.WriteString(fmt.Sprintf("```json\n{\"concepts\":["))
	for i, c := range sp.Concepts {
		if i >= 4 {
			break
		}
		if i > 0 {
			b.WriteString(",")
		}
		cb, _ := json.Marshal(c)
		b.Write(cb)
	}
	b.WriteString("],\"relations\":[")
	for i, r := range sp.Relations {
		if i >= 3 {
			break
		}
		if i > 0 {
			b.WriteString(",")
		}
		rb, _ := json.Marshal(r)
		b.Write(rb)
	}
	b.WriteString("]}\n```\n")
	return b.String()
}

// pickSeed 关键词匹配（领域描述包含种子主题词即命中；顺序即优先级）。
func pickSeed(description string) string {
	d := strings.ToLower(description)
	type kv struct {
		keys []string
		file string
	}
	table := []kv{
		{[]string{"医学", "疾病", "症状", "药物", "临床"}, "med_common.json"},
		{[]string{"基因", "蛋白", "转录", "生物", "细胞"}, "gene_core.json"},
		{[]string{"软件", "缺陷", "bug", "项目", "迭代", "测试"}, "defects.json"},
		{[]string{"组织", "人员", "部门", "员工", "公司"}, "orgs.json"},
		{[]string{"k8s", "kubernetes", "集群", "容器", "pod", "部署"}, "onto_k8s_ops.json"},
		{[]string{"设备", "故障", "运维", "工单"}, "failure.json"},
	}
	for _, e := range table {
		for _, k := range e.keys {
			if strings.Contains(d, k) {
				return e.file
			}
		}
	}
	return ""
}

func buildPrompt(description, extraHint string, fixErrors []string, cqs []string) string {
	var b strings.Builder
	b.WriteString("你是本体建模专家。请根据领域描述生成一个本体 spec_json，严格遵循以下 JSON Schema：\n")
	b.WriteString(specSchemaHint)
	b.WriteString("\n\n规则：\n")
	b.WriteString("- concepts[].name 唯一且非空；relations[].from/to 必须引用已定义概念；instances[].concept 必须引用已定义概念；instances[].relations[].rel/target 必须引用已定义关系/实例。\n")
	b.WriteString("- data_properties（可选，REQ-268）：实例 attributes 中值得声明类型的字面量属性——name 与 attributes 键同名、domain 引用概念 name（可省）、range 用 string|number|integer|boolean|date 短名。\n")
	b.WriteString("- 只输出 JSON，不要 markdown 代码块或其他文本。\n\n领域描述：\n")
	b.WriteString(description)
	if extraHint != "" {
		b.WriteString("\n\n补充要求：\n" + extraHint)
	}
	if len(fixErrors) > 0 {
		b.WriteString("\n\n上一稿存在以下校验错误，请修正后重新输出完整 spec_json：\n")
		for _, e := range fixErrors {
			b.WriteString("- " + e + "\n")
		}
	}
	return b.String()
}

// RawChat 自由文本对话（REQ-103 模式 A 补全轮归纳用）：同一平台代理，不做 schema 约束。
func (c *Creator) RawChat(prompt string) (reply string, usage any, err error) {
	body, _ := json.Marshal(map[string]any{"prompt": prompt, "schema": `{"type":"object","properties":{"reply":{"type":"string"}},"required":["reply"]}`})
	resp, err := c.HTTP.Post(strings.TrimRight(c.PlatformURL, "/")+"/api/ontology-llm/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("调用主平台模型代理失败: %w", err)
	}
	defer resp.Body.Close()
	var res struct {
		DraftJSON string `json:"draft_json"`
		Usage     any    `json:"usage"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", nil, fmt.Errorf("主平台响应解析失败: %w", err)
	}
	if res.Error != "" {
		return "", nil, fmt.Errorf("主平台模型代理错误: %s", res.Error)
	}
	// 期望 {"reply": "..."}；容忍模型直接输出纯文本
	var wrapped struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal([]byte(res.DraftJSON), &wrapped) == nil && strings.TrimSpace(wrapped.Reply) != "" {
		return wrapped.Reply, res.Usage, nil
	}
	return res.DraftJSON, res.Usage, nil
}

func (c *Creator) callGenerate(prompt string) (draft string, usage any, err error) {
	body, _ := json.Marshal(map[string]any{"prompt": prompt, "schema": specSchemaHint})
	resp, err := c.HTTP.Post(strings.TrimRight(c.PlatformURL, "/")+"/api/ontology-llm/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("调用主平台模型代理失败: %w", err)
	}
	defer resp.Body.Close()
	var res struct {
		DraftJSON string `json:"draft_json"`
		Usage     any    `json:"usage"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", nil, fmt.Errorf("主平台响应解析失败: %w", err)
	}
	if res.Error != "" {
		return "", nil, fmt.Errorf("主平台模型代理错误: %s", res.Error)
	}
	return res.DraftJSON, res.Usage, nil
}
