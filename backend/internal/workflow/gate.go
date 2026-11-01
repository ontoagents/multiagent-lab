package workflow

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// REQ-564/M371 WF-③ 门禁断言/人审卡点（109 号 §WF-③ a 路线拍板）——
// gate 节点断言求值（产出文本/变量两域四断言类型）+失败回喂 ≤GateMaxFeedback 轮（护栏④
// 防死循环）+attempt 计数入节点事件；人审卡点=RequireApproval 标记（审批三档 UI 复用经
// chat 装配层 stepApprovalWrap 接线〔WF-⑥ 白名单新值〕；库层 v1 出 approval 占位事件）。
// ---------------------------------------------------------------------------

// GateMaxFeedback 门禁失败回喂上限（护栏④防死循环——109 号「失败回喂 ≤2 轮上限」）。
const GateMaxFeedback = 2

// EvaluateGate 断言求值：全部通过=过闸；返回未通过清单（人话 reason——回喂与事件透出双用）。
func EvaluateGate(n Node, output string, vars *VarTable) []string {
	var failed []string
	for _, a := range n.Assertions {
		switch a.Type {
		case AssertContains:
			if !strings.Contains(output, a.Value) {
				failed = append(failed, gateReason(a, fmt.Sprintf("产出不含 %q", a.Value)))
			}
		case AssertNotContains:
			if strings.Contains(output, a.Value) {
				failed = append(failed, gateReason(a, fmt.Sprintf("产出含禁词 %q", a.Value)))
			}
		case AssertEquals:
			if strings.TrimSpace(output) != strings.TrimSpace(a.Value) {
				failed = append(failed, gateReason(a, fmt.Sprintf("产出 %q ≠ 期望 %q", trunc(output, 80), a.Value)))
			}
		case AssertVarEquals:
			v, ok := vars.Get(a.Name)
			if !ok || !strings.Contains(string(v), a.Value) {
				failed = append(failed, gateReason(a, fmt.Sprintf("变量 %s 未命中 %q", a.Name, a.Value)))
			}
		default:
			failed = append(failed, gateReason(a, fmt.Sprintf("断言类型 %q 非法（须 contains|not_contains|equals|var_equals）", a.Type)))
		}
	}
	return failed
}

func gateReason(a GateAssertion, detail string) string {
	if a.Desc != "" {
		return fmt.Sprintf("断言[%s] %s：%s", a.Type, a.Desc, detail)
	}
	return fmt.Sprintf("断言[%s]：%s", a.Type, detail)
}

func trunc(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) > n {
		return string(rs[:n]) + "…"
	}
	return string(rs)
}

// ValidateGateSpec gate 节点断言形态校验（Validate 主链的 gate 专项补充——断言类型合法）。
// 返回首个非法断言的人话错误（无=通过）。
func ValidateGateSpec(s *Spec) error {
	for _, n := range s.Nodes {
		if n.Type != NodeGate {
			continue
		}
		for _, a := range n.Assertions {
			switch a.Type {
			case AssertContains, AssertNotContains, AssertEquals, AssertVarEquals:
			default:
				return fmt.Errorf("节点 %s 断言类型 %q 非法（须 contains|not_contains|equals|var_equals）", n.ID, a.Type)
			}
			if a.Type == AssertVarEquals && strings.TrimSpace(a.Name) == "" {
				return fmt.Errorf("节点 %s var_equals 断言缺 name（判定对象变量必填）", n.ID)
			}
		}
	}
	return nil
}
