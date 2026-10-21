// REQ-354/M154 OntoChat 能力赋予智能体：对话式本体构建工具组（ontochat_start/turn/save）。
// P2（M160）增补：ontochat_export_ttl 草稿 TTL 导出（只读）+会话归属审计（start 经 tool.Caller
// 落构建平面 source/agent_id 两列）。
// 包装构建平面既有 /api/ontochat/* REST（92 号 §5.3 设计）——旁路三原则：
// ①只发起 HTTP 调用不进 runner 主链路；②注册进 Registry 但不进任何默认 agent 工具集
// （Agent.Tools 显式勾选才生效）；③零新表零新状态，会话与产物全由构建平面存储承载。
// 直连 BUILD_SVC_URL（沿 internal/ontology.Service 同一 env 与缺省，backend/agentd 双进程可用）。
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	ontoChatDefaultBase = "http://127.0.0.1:8091" // BUILD_SVC_URL 缺省（同 internal/ontology）
)

var (
	ontoChatPollInterval  = 1500 * time.Millisecond // 沿前端 1.5s 轮询节奏
	ontoChatDefaultBudget = 300 * time.Second       // ONTOCHAT_LLM_TIMEOUT 缺省秒数（同构建平面引擎）
)

func ontoChatBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("BUILD_SVC_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return ontoChatDefaultBase
}

// ontoChatBudget 工具内 202+job 轮询与单轮调用的总预算（秒）；与构建平面引擎共用
// ONTOCHAT_LLM_TIMEOUT 语义——引擎侧 LLM 调用超时上限与工具侧等待上限一致调。
func ontoChatBudget() time.Duration {
	if v := os.Getenv("ONTOCHAT_LLM_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return ontoChatDefaultBudget
}

// ontoCall 单次构建平面调用（错误回执文本化，不炸 run；不伪造结果）。
func ontoCall(ctx context.Context, method, path string, body any) (int, []byte, error) {
	status, _, bts, err := ontoCallHdr(ctx, method, path, body)
	return status, bts, err
}

// ontoCallHdr 同 ontoCall 并回传响应头（export 工具读 X-Onto-Triples 计数头用）。
func ontoCallHdr(ctx context.Context, method, path string, body any) (int, http.Header, []byte, error) {
	url := ontoChatBaseURL() + path
	var rd io.Reader
	if body != nil {
		bts, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, err
		}
		rd = bytes.NewReader(bts)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return 0, nil, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, b, nil
}

func ontoErr(format string, a ...any) string {
	return fmt.Sprintf("构建平面暂不可达或调用失败（BUILD_SVC_URL=%s）：%s。请稍后重试，或引导用户前往本体构建页面操作；不得编造结果。",
		ontoChatBaseURL(), fmt.Sprintf(format, a...))
}

// ontoDecodeErr 业务错误信封（构建平面 writeErr/校验 400 形态 {error, validation_errors?}）。
func ontoDecodeErr(status int, body []byte) (string, []ontoValidation) {
	var env struct {
		Error       string           `json:"error"`
		Validations []ontoValidation `json:"validation_errors"`
	}
	_ = json.Unmarshal(body, &env)
	if env.Error == "" {
		env.Error = fmt.Sprintf("HTTP %d: %s", status, truncateRunes(string(body), 300))
	}
	return env.Error, env.Validations
}

type ontoValidation struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ontoSessionSnap 会话快照（GET session / turn result.session 共用最小投影）。
type ontoSessionSnap struct {
	ID       string `json:"id"`
	Stage    string `json:"stage"`
	Round    int    `json:"round"`
	OntoID   string `json:"ontology_id,omitempty"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Context struct {
		CQs        []string          `json:"cqs,omitempty"`
		TestReport *ontoTestSummary  `json:"test_report,omitempty"`
		DraftSpec  *json.RawMessage  `json:"draft_spec,omitempty"`
	} `json:"context"`
}

func (s *ontoSessionSnap) lastAssistant() string {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == "assistant" {
			return s.Messages[i].Content
		}
	}
	return ""
}

type ontoDraftSummary struct {
	Name           string `json:"name,omitempty"`
	Description    string `json:"description,omitempty"`
	Concepts       int    `json:"concepts"`
	Relations      int    `json:"relations"`
	DataProperties int    `json:"data_properties,omitempty"`
	Instances      int    `json:"instances,omitempty"`
}

type ontoQualitySummary struct {
	Overall         float64 `json:"overall"`
	Completeness    float64 `json:"completeness"`
	Consistency     float64 `json:"consistency"`
	Maintainability float64 `json:"maintainability"`
	ErrorCount      int     `json:"error_count"`
	WarningCount    int     `json:"warning_count"`
}

type ontoTestSummary struct {
	Passed   int     `json:"passed"`
	Total    int     `json:"total"`
	PassRate float64 `json:"pass_rate"`
}

func draftSummaryOf(raw json.RawMessage) *ontoDraftSummary {
	if len(raw) == 0 {
		return nil
	}
	var sp struct {
		Name           string `json:"name"`
		Description    string `json:"description"`
		Concepts       []any  `json:"concepts"`
		Relations      []any  `json:"relations"`
		DataProperties []any  `json:"data_properties"`
		Instances      []any  `json:"instances"`
	}
	if json.Unmarshal(raw, &sp) != nil {
		return nil
	}
	return &ontoDraftSummary{
		Name: sp.Name, Description: truncateRunes(sp.Description, 120),
		Concepts: len(sp.Concepts), Relations: len(sp.Relations),
		DataProperties: len(sp.DataProperties), Instances: len(sp.Instances),
	}
}

func qualityOf(raw json.RawMessage) *ontoQualitySummary {
	if len(raw) == 0 {
		return nil
	}
	var q struct {
		Score struct {
			Overall         float64 `json:"overall"`
			Completeness    float64 `json:"completeness"`
			Consistency     float64 `json:"consistency"`
			Maintainability float64 `json:"maintainability"`
		} `json:"score"`
		ErrorCount   int `json:"error_count"`
		WarningCount int `json:"warning_count"`
	}
	if json.Unmarshal(raw, &q) != nil {
		return nil
	}
	return &ontoQualitySummary{
		Overall: q.Score.Overall, Completeness: q.Score.Completeness,
		Consistency: q.Score.Consistency, Maintainability: q.Score.Maintainability,
		ErrorCount: q.ErrorCount, WarningCount: q.WarningCount,
	}
}

// ---------------------------------------------------------------------------

type ontoStartIn struct {
	Topic string `json:"topic" jsonschema:"required" jsonschema_description:"领域主题（会话标题）"`
	Mode  string `json:"mode,omitempty" jsonschema_description:"guided=分步访谈模式（story 起，缺省，推荐）；quick=快速模式（cq 起，直接给领域描述）"`
}

type ontoStartOut struct {
	Error     string `json:"error,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Stage     string `json:"stage,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Opening   string `json:"opening,omitempty"` // 开场首问/引导（转述给用户的主体）
	Hint      string `json:"hint,omitempty"`    // 下一步纪律提示
}

func ontoStartFn(ctx context.Context, in ontoStartIn) (*ontoStartOut, error) {
	topic := strings.TrimSpace(in.Topic)
	if topic == "" {
		return &ontoStartOut{Error: "topic 必填：请先向用户确认领域主题再开始会话"}, nil
	}
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		mode = "guided"
	}
	if mode != "guided" && mode != "quick" {
		return &ontoStartOut{Error: fmt.Sprintf("mode 仅支持 guided/quick，收到 %q", in.Mode)}, nil
	}
	// REQ-354/P2·G4：会话归属审计两列——调用者身份（tool.WithCaller 注入）随建会话落
	// 构平面 source=agent_chat+agent_id；非运行期（无身份）不传，构建平面缺省归 page。
	payload := map[string]string{"title": topic, "mode": mode}
	if ci, ok := CallerFrom(ctx); ok && ci.AgentID != "" {
		payload["source"] = "agent_chat"
		payload["agent_id"] = ci.AgentID
	}
	status, body, err := ontoCall(ctx, http.MethodPost, "/api/ontochat/sessions", payload)
	if err != nil {
		return &ontoStartOut{Error: ontoErr("%v", err)}, nil
	}
	if status != http.StatusCreated {
		msg, _ := ontoDecodeErr(status, body)
		return &ontoStartOut{Error: msg}, nil
	}
	var sess ontoSessionSnap
	if err := json.Unmarshal(body, &sess); err != nil {
		return &ontoStartOut{Error: "会话创建响应解析失败: " + err.Error()}, nil
	}
	out := &ontoStartOut{SessionID: sess.ID, Stage: sess.Stage, Mode: mode}
	if mode == "guided" {
		out.Opening = sess.lastAssistant() // guided 建会话即确定性落第 1 问
		// 92 号 §5.3：story 模式顺取引导卡首卡，供转述参考（不替代首问）
		if _, tb, terr := ontoCall(ctx, http.MethodGet, "/api/ontochat/story-templates", nil); terr == nil {
			var cards []map[string]string
			if json.Unmarshal(tb, &cards) == nil && len(cards) > 0 {
				out.Opening += "\n（参考引导卡·" + cards[0]["label"] + "：" + cards[0]["text"] + "）"
			}
		}
		out.Hint = "把开场首问转述给用户；一次只问一个问题，逐轮将用户回答经 ontochat_turn(message=…) 推进。"
	} else {
		out.Hint = "快速模式无开场白：请让用户直接给出领域描述，经 ontochat_turn(message=领域描述) 推进。"
	}
	return out, nil
}

// ---------------------------------------------------------------------------

type ontoTurnIn struct {
	SessionID string `json:"session_id" jsonschema:"required"`
	Message   string `json:"message,omitempty" jsonschema_description:"用户本轮发言（chat 动作必填）"`
	Feedback  string `json:"feedback,omitempty" jsonschema_description:"草稿修正意见（refine 轮：对上一稿的修改要求）"`
	Action    string `json:"action,omitempty" jsonschema_description:"chat=对话轮（默认）；analyze=CQ 聚类去重分析；test=草稿覆盖测试快筛"`
	JobID     string `json:"job_id,omitempty" jsonschema_description:"续查模式：只轮询该任务至终态（上次超时返回的 job_id），忽略其他参数"`
}

type ontoTurnOut struct {
	Error      string             `json:"error,omitempty"`
	SessionID  string             `json:"session_id,omitempty"`
	Stage      string             `json:"stage,omitempty"`
	Round      int                `json:"round,omitempty"`
	Reply      string             `json:"reply,omitempty"` // assistant 本轮回复（转述给用户的主体）
	CQs        []string           `json:"cqs,omitempty"`   // 会话当前 CQ 清单（确认用，列号呈现）
	Draft      *ontoDraftSummary  `json:"draft_summary,omitempty"`
	Quality    *ontoQualitySummary `json:"quality,omitempty"`
	TestReport *ontoTestSummary   `json:"test_report,omitempty"`
	Clusters   int                `json:"clusters,omitempty"`      // analyze 动作：主题簇数
	DedupCount int                `json:"dedup_count,omitempty"`   // analyze 动作：去重合并条数
	Warning    string             `json:"warning,omitempty"`
	JobID      string             `json:"job_id,omitempty"` // 超时未终态时返回，供 job_id 续查
}

func ontoTurnFn(ctx context.Context, in ontoTurnIn) (*ontoTurnOut, error) {
	ctx, cancel := context.WithTimeout(ctx, ontoChatBudget()+15*time.Second)
	defer cancel()
	if strings.TrimSpace(in.JobID) != "" {
		return ontoPollJob(ctx, in.JobID, in.SessionID)
	}
	action := strings.TrimSpace(in.Action)
	if action == "" {
		action = "chat"
	}
	switch action {
	case "chat":
		if strings.TrimSpace(in.Message) == "" && strings.TrimSpace(in.Feedback) == "" {
			return &ontoTurnOut{Error: "chat 动作需要 message（用户发言）或 feedback（修正意见）至少一项"}, nil
		}
		return ontoTurnChat(ctx, in)
	case "analyze":
		return ontoTurnAction(ctx, in.SessionID, "/cq-analyze", map[string]int{})
	case "test":
		return ontoTurnAction(ctx, in.SessionID, "/test", nil)
	default:
		return &ontoTurnOut{Error: fmt.Sprintf("action 仅支持 chat/analyze/test，收到 %q", in.Action)}, nil
	}
}

// ontoTurnChat 对话轮：同步 200 直接出结果；202 轮询 job 至终态（工具内等待对用户透明）。
func ontoTurnChat(ctx context.Context, in ontoTurnIn) (*ontoTurnOut, error) {
	payload := map[string]string{"text": in.Message, "feedback": in.Feedback}
	status, body, err := ontoCall(ctx, http.MethodPost,
		"/api/ontochat/sessions/"+in.SessionID+"/turn", payload)
	if err != nil {
		return &ontoTurnOut{Error: ontoErr("%v", err)}, nil
	}
	if status == http.StatusAccepted {
		var env struct {
			JobID string `json:"job_id"`
		}
		if json.Unmarshal(body, &env) != nil || env.JobID == "" {
			return &ontoTurnOut{Error: "202 响应缺少 job_id"}, nil
		}
		return ontoPollJob(ctx, env.JobID, in.SessionID)
	}
	if status != http.StatusOK {
		msg, _ := ontoDecodeErr(status, body)
		return &ontoTurnOut{Error: msg}, nil
	}
	return ontoTurnOutFromResult(body, in.SessionID)
}

// ontoTurnAction analyze/test 动作：202+job 轮询（前置校验失败 400 直接回执）。
func ontoTurnAction(ctx context.Context, sessionID, suffix string, body any) (*ontoTurnOut, error) {
	status, raw, err := ontoCall(ctx, http.MethodPost, "/api/ontochat/sessions/"+sessionID+suffix, body)
	if err != nil {
		return &ontoTurnOut{Error: ontoErr("%v", err)}, nil
	}
	if status != http.StatusAccepted {
		msg, _ := ontoDecodeErr(status, raw)
		return &ontoTurnOut{Error: msg}, nil
	}
	var env struct {
		JobID string `json:"job_id"`
	}
	if json.Unmarshal(raw, &env) != nil || env.JobID == "" {
		return &ontoTurnOut{Error: "202 响应缺少 job_id"}, nil
	}
	return ontoPollJob(ctx, env.JobID, sessionID)
}

// ontoPollJob 轮询至终态；预算耗尽任务仍在进行 → 返回 job_id 供续查（92 号 §8 R5）。
func ontoPollJob(ctx context.Context, jobID, sessionID string) (*ontoTurnOut, error) {
	for {
		status, body, err := ontoCall(ctx, http.MethodGet, "/api/ontochat/jobs/"+jobID, nil)
		if err != nil {
			if ctx.Err() != nil {
				return &ontoTurnOut{JobID: jobID, Error: fmt.Sprintf(
					"生成超时未完成（等待上限 %s），任务仍在构建平面进行中：job_id=%s。可稍后用 ontochat_turn(job_id=%q) 续查结果。",
					ontoChatBudget(), jobID, jobID)}, nil
			}
			return &ontoTurnOut{JobID: jobID, Error: ontoErr("%v", err)}, nil
		}
		if status != http.StatusOK {
			msg, _ := ontoDecodeErr(status, body)
			return &ontoTurnOut{JobID: jobID, Error: msg}, nil
		}
		var job struct {
			Status string          `json:"status"`
			Error  string          `json:"error"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(body, &job) != nil {
			return &ontoTurnOut{JobID: jobID, Error: "job 响应解析失败"}, nil
		}
		switch job.Status {
		case "done":
			return ontoTurnOutFromResult(job.Result, sessionID)
		case "error":
			return &ontoTurnOut{JobID: jobID, Error: "生成失败：" + job.Error + "。可重试或引导用户在构建页面操作；如实转述不得编造。"}, nil
		case "cancelled":
			return &ontoTurnOut{JobID: jobID, Error: "任务已取消。"}, nil
		}
		select {
		case <-ctx.Done():
			return &ontoTurnOut{JobID: jobID, Error: fmt.Sprintf(
				"生成超时未完成（等待上限 %s），任务仍在构建平面进行中：job_id=%s。可稍后用 ontochat_turn(job_id=%q) 续查结果。",
				ontoChatBudget(), jobID, jobID)}, nil
		case <-time.After(ontoChatPollInterval):
		}
	}
}

// ontoTurnOutFromResult 统一从终态 result（turn 200/turn job.done）组装输出；
// analyze/test 的 result 形态不同，此处先按 turn 形态解，字段缺省即自然为空。
func ontoTurnOutFromResult(raw json.RawMessage, sessionID string) (*ontoTurnOut, error) {
	var res struct {
		Reply   string          `json:"reply"`
		Stage   string          `json:"stage"`
		Round   int             `json:"round"`
		Session json.RawMessage `json:"session"`
		Quality json.RawMessage `json:"quality"`
		Draft   json.RawMessage `json:"draft"`
		Warning string          `json:"warning"`
		// analyze 动作 result
		Clusters   int `json:"clusters"`
		DedupCount int `json:"dedup_count"`
		// test 动作 result（CoverageSummary）
		Passed   int     `json:"passed"`
		Total    int     `json:"total"`
		PassRate float64 `json:"pass_rate"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return &ontoTurnOut{Error: "结果解析失败: " + err.Error()}, nil
	}
	out := &ontoTurnOut{
		SessionID: sessionID, Stage: res.Stage, Round: res.Round, Reply: res.Reply,
		Quality: qualityOf(res.Quality), Draft: draftSummaryOf(res.Draft), Warning: res.Warning,
		Clusters: res.Clusters, DedupCount: res.DedupCount,
	}
	if res.Total > 0 {
		out.TestReport = &ontoTestSummary{Passed: res.Passed, Total: res.Total, PassRate: res.PassRate}
	}
	// session 缺失（analyze/test result 无 session）或续查路径 → 补拉会话取 CQ 与最新 assistant 留痕
	snap := res.Session
	if len(snap) == 0 && out.SessionID != "" {
		if status, body, err := ontoCall(context.Background(), http.MethodGet,
			"/api/ontochat/sessions/"+out.SessionID, nil); err == nil && status == http.StatusOK {
			snap = body
		}
	} else if len(snap) > 0 && out.SessionID == "" {
		var s ontoSessionSnap
		if json.Unmarshal(snap, &s) == nil {
			out.SessionID = s.ID
		}
	}
	if len(snap) > 0 {
		var s ontoSessionSnap
		if json.Unmarshal(snap, &s) == nil {
			if out.SessionID == "" {
				out.SessionID = s.ID
			}
			if out.Stage == "" {
				out.Stage = s.Stage
			}
			if out.Round == 0 {
				out.Round = s.Round
			}
			out.CQs = s.Context.CQs
			if out.TestReport == nil {
				out.TestReport = s.Context.TestReport
			}
			if out.Reply == "" {
				out.Reply = s.lastAssistant()
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------

type ontoSaveIn struct {
	SessionID   string `json:"session_id" jsonschema:"required"`
	Name        string `json:"name,omitempty" jsonschema_description:"本体资产名（缺省用草稿名）"`
	ConfirmNote string `json:"confirm_note" jsonschema:"required" jsonschema_description:"用户确认说明——调用前必须已向用户展示质量报告摘要与入库内容清单并获肯定答复"`
}

type ontoSaveOut struct {
	Error        string           `json:"error,omitempty"`
	Saved        bool             `json:"saved,omitempty"`
	AssetID      string           `json:"asset_id,omitempty"`
	Name         string           `json:"name,omitempty"`
	Description  string           `json:"description,omitempty"`
	Stage        string           `json:"stage,omitempty"`
	Validations  []ontoValidation `json:"validation_errors,omitempty"`
	ConfirmNote  string           `json:"confirm_note,omitempty"`
}

func ontoSaveFn(ctx context.Context, in ontoSaveIn) (*ontoSaveOut, error) {
	if strings.TrimSpace(in.ConfirmNote) == "" {
		return &ontoSaveOut{Error: "confirm_note 必填：调用前必须已向用户展示质量报告摘要与入库内容清单，并得到用户对「确认入库」的肯定答复——确认纪律不因自由对话而豁免。"}, nil
	}
	payload := map[string]string{"name": strings.TrimSpace(in.Name)}
	status, body, err := ontoCall(ctx, http.MethodPost,
		"/api/ontochat/sessions/"+in.SessionID+"/save", payload)
	if err != nil {
		return &ontoSaveOut{Error: ontoErr("%v", err)}, nil
	}
	if status == http.StatusBadRequest {
		msg, vals := ontoDecodeErr(status, body)
		return &ontoSaveOut{Error: "入库未通过：" + msg + "。请向用户逐条转述门禁明细，不得隐瞒或淡化。",
			Validations: vals, ConfirmNote: in.ConfirmNote}, nil
	}
	if status != http.StatusCreated {
		msg, _ := ontoDecodeErr(status, body)
		return &ontoSaveOut{Error: msg, ConfirmNote: in.ConfirmNote}, nil
	}
	var env struct {
		Ontology struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"ontology"`
		Session ontoSessionSnap `json:"session"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return &ontoSaveOut{Error: "入库响应解析失败: " + err.Error()}, nil
	}
	return &ontoSaveOut{
		Saved: true, AssetID: env.Ontology.ID, Name: env.Ontology.Name,
		Description: truncateRunes(env.Ontology.Description, 120),
		Stage:       env.Session.Stage, ConfirmNote: in.ConfirmNote,
	}, nil
}

// ---------------------------------------------------------------------------

// ontoExportTTLCap 工具输出 TTL 上限（模型上下文保护；超出截断并如实标注——
// 全文可经深链在构建平面/资产页获取）。
const ontoExportTTLCap = 24 << 10

type ontoExportIn struct {
	SessionID string `json:"session_id" jsonschema:"required"`
}

type ontoExportOut struct {
	Error        string `json:"error,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	Triples      int    `json:"triples,omitempty"`
	TTL          string `json:"ttl,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	SavedAssetID string `json:"saved_asset_id,omitempty"` // 会话已入库时回填（资产页有权威导出）
	Hint         string `json:"hint,omitempty"`
}

// ontoExportTTLFn 草稿 TTL 导出（REQ-354/P2③：构建平面 GET sessions/{id}/export——
// spec→TTL Go 导出器与资产导出同一实现，X-Onto-Source: ontochat_draft）。
// 只读动作：导出≠入库，无质量门禁环节——入库仍须走 ontochat_save（确认纪律不豁免）。
func ontoExportTTLFn(ctx context.Context, in ontoExportIn) (*ontoExportOut, error) {
	status, hdr, body, err := ontoCallHdr(ctx, http.MethodGet,
		"/api/ontochat/sessions/"+in.SessionID+"/export", nil)
	if err != nil {
		return &ontoExportOut{Error: ontoErr("%v", err)}, nil
	}
	if status != http.StatusOK {
		msg, _ := ontoDecodeErr(status, body)
		return &ontoExportOut{Error: msg}, nil
	}
	out := &ontoExportOut{SessionID: in.SessionID, TTL: string(body)}
	if n, cerr := strconv.Atoi(hdr.Get("X-Onto-Triples")); cerr == nil && n > 0 {
		out.Triples = n
	}
	if len(out.TTL) > ontoExportTTLCap {
		out.TTL = out.TTL[:ontoExportTTLCap]
		out.Truncated = true
	}
	// 已入库会话顺带回填资产指引（一次 GET；失败不阻断导出主链路）
	if status, body, err := ontoCall(ctx, http.MethodGet, "/api/ontochat/sessions/"+in.SessionID, nil); err == nil && status == http.StatusOK {
		var sess ontoSessionSnap
		if json.Unmarshal(body, &sess) == nil {
			out.SavedAssetID = sess.OntoID
		}
	}
	if out.SavedAssetID != "" {
		out.Hint = "会话已入库资产 " + out.SavedAssetID + "（资产页可下载权威导出与版本快照）；本 TTL 为当前草稿投影。把 TTL 交给用户或按需转述，不要无必要地全文复述。"
	} else {
		out.Hint = "草稿 TTL（未经质量门禁）。如需入库走 ontochat_save（须先获用户确认）；把 TTL 交给用户或按需转述，不要无必要地全文复述。"
	}
	return out, nil
}

// ---------------------------------------------------------------------------

// OnchatToolIDs REQ-360/M161：对话建本体能力开关（Agent.OnchatMode）开启时装配层自动并入的
// 工具组清单——与 Agent.Tools 手勾取并集（assembler 装配期按此清单从注册表实例化，来源标注
// onchat）。注册表仍仅注册不进任何默认工具集，挂载语义由能力开关承载（92 号 §5.2）。
var OnchatToolIDs = []string{"ontochat_start", "ontochat_turn", "ontochat_save", "ontochat_export_ttl"}

// registerOntoChatTools 注册 OntoChat 工具组（REQ-354）。仅注册——挂载由 Agent.Tools 显式勾选
// 或 REQ-360 能力开关（onchat_mode）自动并入，不进任何内置助手默认工具集（92 号 §5.2 默认关原则；
// REQ-360 决策注：平台助手 builtin 行迁移默认 onchat_mode=guided——显式引导面定位，例外见 03 §2.54）。
func registerOntoChatTools(r *Registry) error {
	entries := []*Entry{
		{ID: "ontochat_start", Description: "开始一次对话式本体构建会话（OntoChat 引擎，对接本体构建平面）。topic=领域主题（必填）；mode 缺省 guided=分步访谈（story 起，推荐），quick=快速模式（cq 起）。返回 session_id 与开场首问——把首问转述给用户，一次只问一个问题。",
			Source: SourceBuiltin, New: func(ctx context.Context) (einotool.BaseTool, error) {
				return utils.InferTool("ontochat_start",
					"开始对话式本体构建会话（OntoChat）。先向用户确认领域主题再调用；返回开场首问需转述给用户。",
					ontoStartFn)
			}},
		{ID: "ontochat_turn", Description: "推进对话式本体构建会话：用户发言（chat，默认）/草稿修正意见（feedback）/CQ 聚类分析（analyze）/草稿覆盖测试（test）/超时任务续查（job_id）。CQ 候选必须列号（CQ-1、CQ-2…）呈现并逐条等用户确认，不得自行取舍；生成草稿后完整转述质量三维分与结构摘要。",
			Source: SourceBuiltin, New: func(ctx context.Context) (einotool.BaseTool, error) {
				return utils.InferTool("ontochat_turn",
					"推进对话式本体构建会话（用户发言/CQ 分析/覆盖测试/续查）。CQ 候选必须列号呈现待用户确认。",
					ontoTurnFn)
			}},
		{ID: "ontochat_save", Description: "把会话草稿入库为本体资产（走构建平面质量门禁）。确认纪律：调用前必须已向用户展示质量报告摘要与入库内容清单并获肯定答复；失败时逐条转述门禁明细，不得隐瞒或淡化。",
			Source: SourceBuiltin, New: func(ctx context.Context) (einotool.BaseTool, error) {
				return utils.InferTool("ontochat_save",
					"会话草稿入库为本体资产（质量门禁）。必须先向用户展示质量报告与入库清单并获确认（confirm_note 必填）。",
					ontoSaveFn)
			}},
		{ID: "ontochat_export_ttl", Description: "把会话当前草稿导出为 Turtle（TTL）文本（只读，不经质量门禁——导出≠入库，入库仍须 ontochat_save）。用户要「导出 TTL/交给外部工具」时使用；返回 TTL 全文（超 24KB 截断并标注）与三元组计数。",
			Source: SourceBuiltin, New: func(ctx context.Context) (einotool.BaseTool, error) {
				return utils.InferTool("ontochat_export_ttl",
					"会话草稿导出为 TTL 文本（只读）。导出不等价入库；入库走 ontochat_save。",
					ontoExportTTLFn)
			}},
	}
	for _, e := range entries {
		if err := r.Register(e); err != nil {
			return err
		}
	}
	return nil
}
