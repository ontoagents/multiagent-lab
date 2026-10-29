package tool

// REQ-202/M38 Harness 执行面：todo_write 原语（模型自写任务清单，进度外显的机器可读侧）。
// 存储：conversation.todo_json（全量覆写，REQ-204/M39 进度产物双轨的机器侧雏形）；
// 过程可见：模型回执即清单渲染，前端经既有 tool.call/result 事件渲染（REQ-204 再做专用卡）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

var todoStatuses = map[string]bool{"pending": true, "in_progress": true, "done": true}

type todoItem struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

type todoWriteIn struct {
	Todos []todoItem `json:"todos" jsonschema:"required"`
	// REQ-385/M181 C2 增量有界化：patch=按 title 合并（只传变更项——长任务清单免全量重写，
	// 每次调用的 token 成本与清单长度解耦）；缺省 full=全量覆写（存量语义零变化）。
	Mode string `json:"mode,omitempty"`
}

// TodoTitleCap 单项标题上限（rune；有界化第二维——防超长标题膨胀存储与回执）。
const TodoTitleCap = 200

type todoWriteOut struct {
	Error    string   `json:"error,omitempty"` // 业务错误回执（参数/落库——文本回喂模型，不炸 run）
	Count    int      `json:"count"`
	Todos    []string `json:"todos"`
	Receipt  string   `json:"receipt"`
}

// TodoDeps todo_write 依赖：会话清单落库（conversation.todo_json）。
type TodoDeps struct {
	Store          *store.Store
	ConversationID string
}

// NewTodoWriteTool 写任务清单：全量覆写当前会话的 todo 列表（pending/in_progress/done），
// 返回渲染回执供模型确认；非破坏性（清单可随时重写），状态词汇表受限。
func NewTodoWriteTool(deps TodoDeps) (einotool.BaseTool, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("todo_write 需要 Store")
	}
	return utils.InferTool("todo_write",
		"写当前会话的任务清单：逐项 {title, status}，status∈pending|in_progress|done。mode 缺省 full=全量覆写；mode=\"patch\"=按 title 增量合并（只传变更项，其余保持原状）。多步任务先列清单，每完成一项就更新状态；用于向用户外显进度。",
		func(_ context.Context, in todoWriteIn) (*todoWriteOut, error) {
			if len(in.Todos) == 0 {
				return &todoWriteOut{Error: "todos 不能为空（保持至少一项或不再调用）"}, nil
			}
			if len(in.Todos) > 50 {
				return &todoWriteOut{Error: "todo 数量上限 50"}, nil
			}
			// REQ-385 C2：patch 模式——读现有清单按 title 合并（新项追加/已有项改状态），其余原状
			base := make([]todoItem, 0, 8)
			if in.Mode == "patch" {
				if conv, err := deps.Store.GetConversation(deps.ConversationID); err == nil && conv.TodoJSON != "" {
					_ = json.Unmarshal([]byte(conv.TodoJSON), &base)
				}
			}
			out := make([]string, 0, len(in.Todos))
			clean := make([]todoItem, 0, len(in.Todos))
			for i, it := range in.Todos {
				title := strings.TrimSpace(it.Title)
				if title == "" {
					return &todoWriteOut{Error: fmt.Sprintf("第 %d 项 title 为空", i+1)}, nil
				}
				if r := []rune(title); len(r) > TodoTitleCap {
					return &todoWriteOut{Error: fmt.Sprintf("第 %d 项 title 超上限（%d/%d rune）——拆短或并入描述", i+1, len(r), TodoTitleCap)}, nil
				}
				status := strings.TrimSpace(it.Status)
				if status == "" {
					status = "pending"
				}
				if !todoStatuses[status] {
					return &todoWriteOut{Error: fmt.Sprintf("第 %d 项 status 非法: %q（仅 pending|in_progress|done）", i+1, it.Status)}, nil
				}
				clean = append(clean, todoItem{Title: title, Status: status})
				mark := "[ ]"
				switch status {
				case "in_progress":
					mark = "[~]"
				case "done":
					mark = "[x]"
				}
				out = append(out, fmt.Sprintf("%s %s", mark, title))
			}
			// patch 合并：base 中未出现在本次变更集的项保持原状（增量语义），变过的以新状态覆盖
			if in.Mode == "patch" && len(base) > 0 {
				changed := map[string]bool{}
				for _, it := range clean {
					changed[it.Title] = true
				}
				merged := make([]todoItem, 0, len(base)+len(clean))
				merged = append(merged, clean...)
				for _, it := range base {
					if !changed[it.Title] {
						merged = append(merged, it)
					}
				}
				if len(merged) > 50 {
					return &todoWriteOut{Error: fmt.Sprintf("合并后 todo 数量超上限 50（现 %d）——清理已完成项", len(merged))}, nil
				}
				clean = merged
				out = out[:0]
				for _, it := range clean {
					mark := "[ ]"
					switch it.Status {
					case "in_progress":
						mark = "[~]"
					case "done":
						mark = "[x]"
					}
					out = append(out, fmt.Sprintf("%s %s", mark, it.Title))
				}
			}
			b, err := json.Marshal(clean)
			if err != nil {
				return &todoWriteOut{Error: err.Error()}, nil
			}
			if err := deps.Store.SaveConversationTodo(deps.ConversationID, string(b)); err != nil {
				return &todoWriteOut{Error: err.Error()}, nil
			}
			return &todoWriteOut{Count: len(clean), Todos: out, Receipt: strings.Join(out, "\n")}, nil
		})
}
