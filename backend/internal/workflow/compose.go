package workflow

// ---------------------------------------------------------------------------
// REQ-566/M373 WF-⑤ 拓扑合成器（L2 前半；109 号 §WF-⑤ 切粒度）——
// LLM 从任务描述合成 DAG：结构化输出配方+校验闭环（WF-① Validate）+失败回喂 ≤2 轮
// （非法输出回喂修复）+连败 2 轮**降级线性链**交付（degraded 事件标注——验收④）+
// **同 DAG 指纹缓存**（任务描述 sha256 指纹——验收③二次命中不重合成；缓存载体经注入——
// store 迁移 088 表随装配轮消费，库层接口化不触 store 包）。
// 护栏：超护栏（节点数/并行度）合成期中性文案——Validate 人话即 400 透传形态（验收②）。
// ---------------------------------------------------------------------------

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ComposeMaxFeedback 合成失败回喂上限（沿 WF-③ GateMaxFeedback 口径——护栏④）。
const ComposeMaxFeedback = 2

// ComposeLLM 合成执行体（chat 装配层注入 LLM 调用——库层不触模型面）。
type ComposeLLM func(ctx context.Context, prompt string) (string, error)

// ComposeCache 同 DAG 指纹缓存载体（注入——store 迁移 088 表消费方；nil=不缓存）。
type ComposeCache interface {
	Load(fp string) (string, bool) // 命中返回 spec JSON
	Save(fp, specJSON string)
}

// ComposeResult 合成结果（degraded 标注——验收④事件素材）。
type ComposeResult struct {
	Spec     *Spec  `json:"spec"`
	Degraded bool   `json:"degraded,omitempty"` // 连败降级线性链
	FromCache bool  `json:"from_cache,omitempty"` // 指纹命中
	Notes    []string `json:"notes,omitempty"`   // 过程注记（回喂/降级——事件素材）
}

// Fingerprint 任务描述指纹（归一空白后 sha256——同义改写不保证命中，同描述幂等命中）。
func Fingerprint(task string) string {
	norm := strings.Join(strings.Fields(task), " ")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// ComposePrompt 合成配方（结构化输出——schema 说明+任务+护栏告知；三先例同构形态）。
func ComposePrompt(task string) string {
	var b strings.Builder
	b.WriteString("你是工作流编排专家。把任务分解为一个 DAG 执行计划，只输出 JSON（不要 markdown 围栏）。\n")
	b.WriteString(`JSON 形态：{"name":"计划名","nodes":[{"id":"n1","type":"llm|tool|gate","name":"节点名","instruction":"该节点做什么","var_writes":["变量名"]}],"edges":[{"from":"n1","to":"n2","condition":"可选：var:变量名==值"}],"vars":[{"name":"变量名","type":"string"}]}`)
	b.WriteString("\n规则：\n1. 节点数 ≤" + fmt.Sprint(MaxNodes) + "，禁止环（有向无环）；多入度节点须写 join:\"all\" 或 \"any\"。\n")
	b.WriteString("2. var_writes 引用的变量必须在 vars 中声明；type 须 string|number|bool|json。\n")
	b.WriteString("3. 每个节点 instruction 具体可执行（步骤级，不复述总任务）。\n")
	b.WriteString("4. 条件分支用 condition 表达；顺序依赖用 edges 表达。\n")
	b.WriteString("\n任务：" + task + "\n\n只输出 JSON 全文。")
	return b.String()
}

// Synthesize 合成 DAG（验收①②③④）：指纹命中→直返；未命中→LLM 合成→Validate（非法回喂
// 原始输出+错误 ≤ComposeMaxFeedback 轮）→连败降级线性链（degraded）；成功写缓存。
func Synthesize(ctx context.Context, task string, llm ComposeLLM, cache ComposeCache) (*ComposeResult, error) {
	if strings.TrimSpace(task) == "" {
		return nil, fmt.Errorf("任务描述为空——无法合成执行计划")
	}
	if llm == nil {
		return nil, fmt.Errorf("合成执行体未注入")
	}
	fp := Fingerprint(task)
	if cache != nil {
		if raw, ok := cache.Load(fp); ok {
			var s Spec
			if json.Unmarshal([]byte(raw), &s) == nil && s.Validate() == nil {
				return &ComposeResult{Spec: &s, FromCache: true}, nil
			}
			// 缓存损坏按未命中处理（不阻塞合成）
		}
	}
	prompt := ComposePrompt(task)
	var lastErr error
	for attempt := 0; attempt <= ComposeMaxFeedback; attempt++ {
		out, err := llm(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}
		spec, perr := parseSpecJSON(out)
		if perr == nil {
			if verr := spec.Validate(); verr != nil {
				lastErr = verr // 校验失败人话——回喂修复
			} else if gerr := ValidateGateSpec(spec); gerr != nil {
				lastErr = gerr
			} else {
				if cache != nil {
					b, _ := json.Marshal(spec)
					cache.Save(fp, string(b))
				}
				return &ComposeResult{Spec: spec}, nil
			}
		} else {
			lastErr = perr
		}
		// 回喂：原输出+错误清单附加（≤ComposeMaxFeedback 轮）
		prompt = ComposePrompt(task) + "\n\n【上次输出未通过校验，请修正】\n错误：" + errText(lastErr) + "\n上次输出：" + truncStr(out, 2000)
	}
	// 连败降级：线性链（验收④——degraded 标注）
	spec := linearChain(task)
	if cache != nil {
		b, _ := json.Marshal(spec)
		cache.Save(fp, string(b))
	}
	return &ComposeResult{
		Spec: spec, Degraded: true,
		Notes: []string{fmt.Sprintf("合成校验连败 %d 轮，已降级线性链（degraded）：%s", ComposeMaxFeedback+1, errText(lastErr))},
	}, nil
}

// linearChain 降级线性链（三节点通用链——计划/执行/总结；degraded 语义可审计）。
func linearChain(task string) *Spec {
	taskShort := truncStr(task, 60)
	return &Spec{
		Name: "降级线性链（" + taskShort + "）",
		Nodes: []Node{
			{ID: "plan", Type: NodeLLM, Name: "拆解", Instruction: "把任务拆为 2~4 个可执行步骤，逐行列出：" + task},
			{ID: "exec", Type: NodeLLM, Name: "执行", Instruction: "按上游拆解逐步执行并汇总过程产出"},
			{ID: "sum", Type: NodeLLM, Name: "总结", Instruction: "对执行结果给出最终结论与未尽事项"},
		},
		Edges: []Edge{{From: "plan", To: "exec"}, {From: "exec", To: "sum"}},
	}
}

// parseSpecJSON 宽容解析（剥 markdown 围栏/前后杂讯——结构化输出三先例同构）。
func parseSpecJSON(out string) (*Spec, error) {
	t := strings.TrimSpace(out)
	if i := strings.Index(t, "{"); i > 0 {
		t = t[i:]
	}
	if i := strings.LastIndex(t, "}"); i >= 0 && i < len(t)-1 {
		t = t[:i+1]
	}
	var s Spec
	if err := json.Unmarshal([]byte(t), &s); err != nil {
		return nil, fmt.Errorf("合成输出非合法 JSON: %w", err)
	}
	return &s, nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncStr(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n]) + "…"
	}
	return s
}

// MemComposeCache 进程内缓存（单机形态默认实现——WF-⑥ 装配可换 store 表载体）。
type MemComposeCache struct {
	m map[string]string
}

func NewMemComposeCache() *MemComposeCache {
	return &MemComposeCache{m: map[string]string{}}
}

func (c *MemComposeCache) Load(fp string) (string, bool) {
	v, ok := c.m[fp]
	return v, ok
}

func (c *MemComposeCache) Save(fp, specJSON string) {
	if c.m == nil {
		c.m = map[string]string{}
	}
	c.m[fp] = specJSON
	c.stamp(fp)
}

func (c *MemComposeCache) stamp(fp string) { _ = fp; _ = time.Now() }
