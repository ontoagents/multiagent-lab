package chat

// REQ-623-4371/M433：@ 点名定向路由（机制层）＋ mention 分散模式（模式层）。
// 机制层：任意项目协作模式下消息 @ 成员 → 被点名成员直接装配执行（跳过协调者首轮路由）；
// 模式层：collab_mode=mention（分散模式）——项目不设协调者执行循环，@ 是唯一触发
//（无 @ 消息拒绝并提示）。多 @ 按提及顺序全部执行（ADK SequentialAgent——后者可见前者回答，
// 群聊语义）；外部成员（协调者 a2a/cli 连接器）与本地成员同列。
// 解析在 API 层（结构化 mention_agent_ids 优先，文本 @ 前缀兜底），经 ctx 传入装配层——
// runCtx 与请求 ctx 解耦（REQ-527 detached），handler 在 runCtx 上注入；TurnLoop steer/resume
// 复用同 ctx 链自动携带，运行中重装配语义不丢。

import (
	"context"
	"sort"
	"strings"
)

type mentionCtxKey struct{}

// MentionTarget @ 点名目标：本地成员（AgentID 非空）或外部成员（协调者连接器名 ExtName）。
type MentionTarget struct {
	AgentID string // 本地成员 agent.id（project_agent 在册）
	ExtName string // 外部成员名（协调者 a2a/cli 连接器实例名——workflow/transfer 同名口径）
}

// WithMentionTargets 点名目标入 ctx（handler 在 runCtx 上注入；nil/空=无点名原样返回）。
func WithMentionTargets(ctx context.Context, ts []MentionTarget) context.Context {
	if len(ts) == 0 {
		return ctx
	}
	return context.WithValue(ctx, mentionCtxKey{}, ts)
}

// MentionTargetsFromCtx 读点名目标（未注入返回 nil）。
func MentionTargetsFromCtx(ctx context.Context) []MentionTarget {
	ts, _ := ctx.Value(mentionCtxKey{}).([]MentionTarget)
	return ts
}

// ParseMentionNames 文本 @ 前缀解析（兜底路径——前端结构化 mention_agent_ids 优先）：
// 扫描词首 @<名>，按「名最长优先」命中（@Alice 与 @AliceChen 同文命中长者防互吞），
// 按出现顺序去重返回；本地成员名 nameToID 映射为 AgentID，外部成员名集合 extNames 映射为 ExtName。
func ParseMentionNames(input string, nameToID map[string]string, extNames map[string]bool) []MentionTarget {
	if !strings.Contains(input, "@") {
		return nil
	}
	names := make([]string, 0, len(nameToID)+len(extNames))
	for n := range nameToID {
		names = append(names, n)
	}
	for n := range extNames {
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	var out []MentionTarget
	seen := map[string]bool{}
	for i := 0; i+1 < len(input); i++ {
		if input[i] != '@' {
			continue
		}
		rest := input[i+1:]
		for _, n := range names {
			if !strings.HasPrefix(rest, n) {
				continue
			}
			if extNames[n] {
				if !seen["ext:"+n] {
					seen["ext:"+n] = true
					out = append(out, MentionTarget{ExtName: n})
				}
			} else if id := nameToID[n]; id != "" {
				if !seen[id] {
					seen[id] = true
					out = append(out, MentionTarget{AgentID: id})
				}
			}
			break // 命中最长名即止
		}
	}
	return out
}
