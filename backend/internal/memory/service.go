package memory

// REQ-551/M358 工作区记忆作用域服务（Q-24 分层批④收官；Q-23 衍生②转正载体）。
// 写入侧=对话收尾候选生成（沿伴生 OnRunComplete 归属校验+游标增量+GenerateStructuredTemp 结构化
// 抽取范式——companion/service.go 先例）：仅作用域会话触发（直挂 workspace_id 或项目归属工作区），
// 无工作区会话不参与读写（存量行为零变化）；全部候选须人工确认（无自动入记忆——可审计口径，
// 黑箱记忆不做）；非阻塞零错误上抛（失败仅 memory.* 事件+日志，对话主链路无感知）。
// 读取侧在 chat.recallMemory（经 store 直读 accepted，chat→memory 包环约束下不反向依赖）。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// 单窗预算（对照伴生 WinMsgs=16/WinChars=8k 收敛口径；记忆抽取单窗足量——跨会话记忆为稀疏事实）
const (
	winMsgs     = 16
	winChars    = 8000
	maxMemories = 5 // 单轮候选上限（成本护栏：稀疏事实不该每轮涌出大量候选）
)

type Service struct {
	Store *store.Store
	Box   *secrets.Box

	mu      sync.Mutex
	running map[string]bool
	pending map[string]string // convID → runID（抽取进行中新收尾的补抽标记，沿 REQ-282 B1 范式）
}

func NewService(st *store.Store, box *secrets.Box) *Service {
	return &Service{Store: st, Box: box, running: map[string]bool{}, pending: map[string]string{}}
}

// resolveWorkspaceID 会话作用域解析：直挂 workspace_id 优先，项目会话转发 project.workspace_id；
// 无归属返回空（调用方短路——无工作区会话不参与读写，106 号 §五 M3 口径）。
func (s *Service) resolveWorkspaceID(conv *store.Conversation) string {
	if conv == nil {
		return ""
	}
	if conv.WorkspaceID != nil && *conv.WorkspaceID != "" {
		return *conv.WorkspaceID
	}
	if conv.Scope == "project" && conv.ProjectID != nil && *conv.ProjectID != "" {
		if p, err := s.Store.GetProject(*conv.ProjectID); err == nil {
			return p.WorkspaceID
		}
	}
	return ""
}

// OnRunComplete Run/Resume 收尾触发点（API 层调用；非阻塞、零错误上抛——沿伴生同名范式）。
// 无工作区归属 / 无 agent → 静默返回。
func (s *Service) OnRunComplete(conv *store.Conversation, agent *store.Agent, runID string) {
	if s == nil || conv == nil || agent == nil {
		return
	}
	if s.resolveWorkspaceID(conv) == "" {
		return
	}
	s.mu.Lock()
	if s.running[conv.ID] {
		s.pending[conv.ID] = runID
		s.mu.Unlock()
		return
	}
	s.running[conv.ID] = true
	s.mu.Unlock()
	go s.extractRounds(conv, agent, runID)
}

func (s *Service) extractRounds(conv *store.Conversation, agent *store.Agent, runID string) {
	defer func() {
		s.mu.Lock()
		next := s.pending[conv.ID]
		delete(s.pending, conv.ID)
		delete(s.running, conv.ID)
		s.mu.Unlock()
		// 补抽一轮（限 1 轮防风暴——长抽取期间新收尾由下轮自然承接的口径与伴生一致）
		if next != "" {
			s.OnRunComplete(conv, agent, next)
		}
	}()
	n, err := s.ExtractNew(context.Background(), conv.ID, agent.ID, runID)
	if err != nil {
		log.Printf("[memory] 会话 %s 候选生成失败（不影响对话）: %v", conv.ID, err)
		return
	}
	if n > 0 {
		log.Printf("[memory] 会话 %s 新增 %d 条记忆候选待确认", conv.ID, n)
	}
}

type memOut struct {
	Memories []struct {
		Content string `json:"content"`
	} `json:"memories"`
}

var memorySchema = []byte(`{"type":"object","properties":{"memories":{"type":"array","maxItems":5,"items":{"type":"object","properties":{"content":{"type":"string"}},"required":["content"]}}},"required":["memories"]}`)

func memoryPrompt(corpus string) string {
	return `你是个人工作记忆整理助手。下面是一段工作/学习对话。请提取「值得跨会话长期记住的稳定事实」——用户的长期目标、领域背景、明确偏好、关键决定、项目约定。要求：
- 只提取稳定、可复用、跨会话有效的信息；寒暄/一次性任务细节/本轮即可丢弃的内容不要。
- 每条一句话陈述（≤120 字），使用第三人称视角（「用户…」）。
- 最多 5 条；没有值得记的就返回空数组。宁缺毋滥。
- 只输出一个 JSON 对象：{"memories":[{"content":"..."}]}

对话内容：
` + corpus
}

// ExtractNew 游标续抽（沿伴生增量范式）：读 last_message_id 之后增量（仅用户/助手文本消息）→
// 单窗 LLM 结构化抽取 → 去重落候选 → 游标推进。返回新增候选数。
func (s *Service) ExtractNew(ctx context.Context, convID, agentID, runID string) (int, error) {
	agent, err := s.Store.GetAgent(agentID)
	if err != nil {
		return 0, err
	}
	conv, err := s.Store.GetConversation(convID)
	if err != nil {
		return 0, err
	}
	wsID := s.resolveWorkspaceID(conv)
	if wsID == "" {
		return 0, nil
	}
	cursor, err := s.Store.GetMemoryCursor(convID)
	if err != nil {
		return 0, err
	}
	msgs, err := s.Store.ListMessages(convID)
	if err != nil {
		return 0, err
	}
	// 增量定位：游标之后（游标空=全量首抽，cap 单窗防首抽风暴）
	fresh := messagesAfter(msgs, cursor)
	if len(fresh) == 0 {
		return 0, nil
	}
	if len(fresh) > winMsgs {
		fresh = fresh[len(fresh)-winMsgs:]
	}
	corpus := renderCorpus(fresh)
	if len(corpus) > winChars {
		corpus = corpus[len(corpus)-winChars:]
	}
	s.emitEvent(convID, runID, "memory.extract", map[string]any{"phase": "started", "fresh": len(fresh)})
	connID := ""
	if agent.ModelConnID != nil {
		connID = *agent.ModelConnID
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	res, err := chat.GenerateStructuredTemp(ctx, s.Store, s.Box, connID, memoryPrompt(corpus), string(memorySchema), ptr(0.2))
	if err != nil {
		s.emitEvent(convID, runID, "memory.extract", map[string]any{"phase": "error", "message": err.Error()})
		return 0, fmt.Errorf("LLM 记忆抽取失败（游标未推进可重试）: %w", err)
	}
	var out memOut
	if perr := json.Unmarshal([]byte(res.DraftJSON), &out); perr != nil {
		s.emitEvent(convID, runID, "memory.extract", map[string]any{"phase": "error", "message": "输出解析失败: " + perr.Error()})
		return 0, fmt.Errorf("记忆抽取输出解析失败: %w", perr)
	}
	// 溯源锚点：本窗最后一条用户/助手消息（行级溯源=可审计口径；空窗不落行）
	lastID := ""
	for i := len(fresh) - 1; i >= 0; i-- {
		if fresh[i].Role == "user" || fresh[i].Role == "assistant" {
			lastID = fresh[i].ID
			break
		}
	}
	added := 0
	for _, m := range out.Memories {
		if added >= maxMemories {
			break
		}
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if _, created, cerr := s.Store.CreateMemory(&store.Memory{
			WorkspaceID:          wsID,
			Content:              m.Content,
			SourceConversationID: convID,
			SourceMessageID:      lastID,
			Status:               "candidate",
		}); cerr != nil {
			return added, cerr
		} else if created {
			added++
		}
	}
	if lastID != "" {
		if aerr := s.Store.AdvanceMemoryCursor(convID, lastID); aerr != nil {
			return added, aerr
		}
	}
	s.emitEvent(convID, runID, "memory.candidates", map[string]any{"count": added, "total_extracted": len(out.Memories)})
	return added, nil
}

// messagesAfter 游标增量（last_message_id 之后的用户/助手文本消息；游标空=全量）。
func messagesAfter(msgs []*store.Message, lastID string) []*store.Message {
	if lastID == "" {
		return textMsgs(msgs)
	}
	idx := -1
	for i, m := range msgs {
		if m.ID == lastID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return textMsgs(msgs) // 游标消息已被清理：退化为全量（去重兜底幂等）
	}
	return textMsgs(msgs[idx+1:])
}

func textMsgs(msgs []*store.Message) []*store.Message {
	out := make([]*store.Message, 0, len(msgs))
	for _, m := range msgs {
		if (m.Role == "user" || m.Role == "assistant") && strings.TrimSpace(m.Content) != "" {
			out = append(out, m)
		}
	}
	return out
}

func renderCorpus(msgs []*store.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		b.WriteString(role + "：" + m.Content + "\n")
	}
	return b.String()
}

func (s *Service) emitEvent(convID, runID, eventType string, data map[string]any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	if _, err := s.Store.InsertEvent(&store.RunEvent{ConversationID: convID, RunID: runID, Type: eventType, Data: string(b), SchemaVersion: 2}); err != nil {
		log.Printf("[memory] 过程事件写入失败（%s）: %v", eventType, err)
	}
}

func ptr(f float64) *float64 { return &f }
