// rest_ontochat.go OntoChat 多轮引导端点（REQ-103 模式 A；REQ-271/M80 生成轮异步化）。
// 会话状态机：cq（领域描述+CQ）→ domain（逐轮补全）→ draft/refine（草稿+校验回喂）→ done（已入库）。
// 模型能力归主平台（复用 llmcreate.Creator → /api/ontology-llm/generate），校验归构建平面（spec.Validate）。
// REQ-271 定案口径：生成轮（ontochat.WillGenerate，含 refine 修正轮）走 202+job 轮询——job 与
// session 同库 SQLite 持久化（定案②），支持取消与生成-校验环轮次进度透出；同步轮（cq/domain 归纳）
// 与生成轮失败一律落 assistant 错误消息留痕（此前裸 500 仅 3s toast，用户消息成孤儿）。
package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/ontochat"
)

// listOntoChatSessions GET /api/ontochat/sessions → 会话列表（不含消息体）
func (s *Server) listOntoChatSessions(w http.ResponseWriter, _ *http.Request) {
	sessions, err := s.ontoChatStore().List()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// createOntoChatSession POST /api/ontochat/sessions {title?} → 201 新会话（stage=cq）
func (s *Server) createOntoChatSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		Mode  string `json:"mode"` // REQ-275 定案①：guided=访谈模式（story 阶段起）；缺省 quick=快速模式（cq 起，现状）
	}
	_ = decodeJSON(r, &req) // body 可省略
	sess, err := s.ontoChatStore().Create(newID(), strings.TrimSpace(req.Title))
	if err != nil {
		writeErr(w, err)
		return
	}
	if strings.TrimSpace(req.Mode) == "guided" {
		// 访谈模式：stage=story + 确定性发第 1 问（纯本地，无 LLM）
		stage := "story"
		sess.Stage = stage
		q := ontochat.StoryStepQuestion(0)
		if err := s.ontoChatStore().Append(sess.ID, ontochat.Message{Role: "assistant", Content: q + "\n\n（一问一轮；点「上一步」回退，无相关信息可点「跳过」）"}, &stage, nil, nil); err != nil {
			writeErr(w, err)
			return
		}
		sess, err = s.ontoChatStore().Get(sess.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, sess)
}

// getOntoChatSession GET /api/ontochat/sessions/{id} → 会话全量（含消息与上下文）
func (s *Server) getOntoChatSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.ontoChatStore().Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// deleteOntoChatSession DELETE /api/ontochat/sessions/{id}
func (s *Server) deleteOntoChatSession(w http.ResponseWriter, r *http.Request) {
	if err := s.ontoChatStore().Delete(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": r.PathValue("id")})
}

// ontoChatTurn POST /api/ontochat/sessions/{id}/turn
// {text, feedback?}：text 为用户输入；feedback 非空表示 refine 修正轮（意见回喂重新生成）。
// 同步轮（cq 首轮/domain 归纳）→ 200 {reply, stage, round, session}；
// 生成轮 → 202 {job_id, session}，终态经 GET /api/ontochat/jobs/{id} 轮询
// （done：result={reply,stage,round,session,draft?,warning?}；error/cancelled：错误留痕已落会话消息）。
func (s *Server) ontoChatTurn(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	var req struct {
		Text       string `json:"text"`
		Feedback   string `json:"feedback"`
		ImportFile *struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"import_file"` // REQ-277：文件材料导入（纯本地轮，零 LLM）
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	st := s.ontoChatStore()
	sess, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	isImport := req.ImportFile != nil && strings.TrimSpace(req.ImportFile.Name) != ""
	if !isImport && strings.TrimSpace(req.Text) == "" && strings.TrimSpace(req.Feedback) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text 不能为空"})
		return
	}
	// 同会话互斥：已有生成任务进行中则拒绝（并发保护）
	if active, aerr := st.ActiveJobBySession(sess.ID); aerr == nil && active != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "已有生成任务进行中，请等待完成或取消后再试"})
		return
	}
	// 用户消息落库（refine 轮 text 可为空，仅意见；导入轮存摘要防消息列表爆炸）
	if isImport {
		userMsg := fmt.Sprintf("📎 从文件「%s」导入材料（%d 字符）", req.ImportFile.Name, len([]rune(req.ImportFile.Content)))
		if err := st.Append(sess.ID, ontochat.Message{Role: "user", Content: userMsg}, nil, nil, nil); err != nil {
			writeErr(w, err)
			return
		}
		res, err := s.OntoChat.ImportFile(st, sess, req.ImportFile.Name, req.ImportFile.Content)
		if err != nil {
			if r.Context().Err() == nil {
				_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "本轮处理失败：" + err.Error()}, nil, nil, nil)
			}
			writeErr(w, err)
			return
		}
		fresh, gerr := st.Get(sess.ID)
		if gerr != nil {
			writeErr(w, gerr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reply": res.Reply, "stage": res.NextStage, "round": fresh.Round, "session": fresh})
		return
	}
	if strings.TrimSpace(req.Text) != "" {
		if err := st.Append(sess.ID, ontochat.Message{Role: "user", Content: req.Text}, nil, nil, nil); err != nil {
			writeErr(w, err)
			return
		}
	}
	// 生成轮（draft/refine 任意输入、domain 生成意图）→ 异步；其余同步
	async := strings.TrimSpace(req.Feedback) != "" || ontochat.WillGenerate(sess.Stage, req.Text)
	if !async {
		res, err := s.OntoChat.Turn(r.Context(), st, sess, req.Text)
		if err != nil {
			// REQ-271 错误留痕：同步轮失败落 assistant 消息（客户端已断连则跳过，避免噪音）
			if r.Context().Err() == nil {
				_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "本轮处理失败：" + err.Error() + "\n请重试或补充信息后重试。"}, nil, nil, nil)
			}
			writeErr(w, err)
			return
		}
		fresh, err := st.Get(sess.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		out := map[string]any{
			"reply":   res.Reply,
			"stage":   res.NextStage,
			"round":   fresh.Round,
			"session": fresh,
		}
		if res.Draft != nil {
			out["draft"] = res.Draft
		}
		if res.Warning != "" {
			out["warning"] = res.Warning
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	job, err := st.CreateJob(newOntoJobID(), sess.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	fresh, gerr := st.Get(sess.ID)
	if gerr != nil {
		fresh = sess
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "session": fresh})
	go s.runOntoChatJob(job.ID, sess, req.Text, req.Feedback)
}

// registerOntoJobCancel 运行期取消句柄注册（返回收尾函数：cancel+摘除）。
func (s *Server) registerOntoJobCancel(jobID string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s.jobCancelsMu.Lock()
	s.jobCancels[jobID] = cancel
	s.jobCancelsMu.Unlock()
	return ctx, func() {
		cancel()
		s.jobCancelsMu.Lock()
		delete(s.jobCancels, jobID)
		s.jobCancelsMu.Unlock()
	}
}

// runOntoChatJob 后台执行生成轮：进度透出 → Turn/Refine → 终态落 job + 错误留痕。
func (s *Server) runOntoChatJob(jobID string, sess *ontochat.Session, text, feedback string) {
	st := s.ontoChatStore()
	ctx, release := s.registerOntoJobCancel(jobID)
	defer release()
	_ = st.UpdateJobStatus(jobID, "running", "")
	progress := func(_ int, msg string) {
		_ = st.UpdateJobProgress(jobID, msg)
	}
	var (
		res *ontochat.TurnResult
		err error
	)
	if strings.TrimSpace(feedback) != "" {
		res, err = s.OntoChat.Refine(ctx, st, sess, feedback, progress)
	} else {
		res, err = s.OntoChat.Turn(ctx, st, sess, text, progress)
	}
	if err != nil && ctx.Err() != nil {
		// 取消：留痕 + 终态 cancelled（cancel 端点已置状态，此处幂等）
		_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "已取消生成。可点击「生成草稿」重新发起，或继续补充信息。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(jobID, "cancelled", "")
		return
	}
	if err != nil {
		// REQ-271 错误留痕：失败落 assistant 消息（此前仅 3s toast、聊天区零反馈、用户消息成孤儿——
		// 真机会话 4 条孤儿「生成草稿」即此形态）
		_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "生成失败：" + err.Error() + "\n可点击「生成草稿」重新生成，或补充信息后重试。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(jobID, "error", err.Error())
		return
	}
	result := map[string]any{"reply": res.Reply, "stage": res.NextStage}
	if fresh, ferr := st.Get(sess.ID); ferr == nil {
		result["round"] = fresh.Round
		result["session"] = fresh
	}
	if res.Draft != nil {
		result["draft"] = res.Draft
	}
	if res.Warning != "" {
		result["warning"] = res.Warning
	}
	bts, _ := json.Marshal(result)
	_ = st.SetJobResult(jobID, bts)
	_ = st.UpdateJobStatus(jobID, "done", "")
}

// getOntoChatJob GET /api/ontochat/jobs/{id} → 任务状态（前端 1.5s 轮询）。
func (s *Server) getOntoChatJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.ontoChatStore().GetJob(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// getOntoChatSessionJob GET /api/ontochat/sessions/{id}/job → 会话当前活跃任务（无则 job:null；
// 前端重进会话/刷新后据此恢复轮询，REQ-271）。
func (s *Server) getOntoChatSessionJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.ontoChatStore().ActiveJobBySession(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

// cancelOntoChatJob POST /api/ontochat/jobs/{id}/cancel → 取消生成（运行中即时中止上游 LLM 调用）。
func (s *Server) cancelOntoChatJob(w http.ResponseWriter, r *http.Request) {
	st := s.ontoChatStore()
	job, err := st.GetJob(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if job.Status != "queued" && job.Status != "running" {
		writeJSON(w, http.StatusOK, job) // 终态幂等返回
		return
	}
	s.jobCancelsMu.Lock()
	cancel := s.jobCancels[job.ID]
	s.jobCancelsMu.Unlock()
	if cancel != nil {
		cancel() // goroutine 收尾：留痕 assistant 消息 + 置 cancelled
	} else {
		// 运行时句柄缺失（进程重启悬挂）：直接收敛，消息留痕由 ActiveJobBySession 自愈兜底
		_ = st.Append(job.SessionID, ontochat.Message{Role: "assistant", Content: "已取消生成（任务中断收敛）。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(job.ID, "cancelled", "")
	}
	fresh, gerr := st.GetJob(job.ID)
	if gerr != nil {
		writeErr(w, gerr)
		return
	}
	writeJSON(w, http.StatusOK, fresh)
}

// ontoChatCQExtract POST /api/ontochat/sessions/{id}/cq-extract → 202 job（REQ-272）：
// 从领域描述+累积补充信息抽 CQ 候选（两净化算子），终态 result={cqs,duplicate_count}；
// assistant 留痕提示确认。与生成轮共用 job 表与会话互斥。
func (s *Server) ontoChatCQExtract(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	st := s.ontoChatStore()
	sess, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if sess.Stage == "done" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "会话已结束，如需继续请新建会话"})
		return
	}
	if active, aerr := st.ActiveJobBySession(sess.ID); aerr == nil && active != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "已有生成任务进行中，请等待完成或取消后再试"})
		return
	}
	job, err := st.CreateJob(newOntoJobID(), sess.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID})
	go s.runOntoChatCQExtract(job.ID, sess)
}

// runOntoChatCQExtract 后台执行 CQ 抽取：进度 → ExtractCQs → 终态落 job + 错误留痕。
func (s *Server) runOntoChatCQExtract(jobID string, sess *ontochat.Session) {
	st := s.ontoChatStore()
	ctx, release := s.registerOntoJobCancel(jobID)
	defer release()
	_ = st.UpdateJobStatus(jobID, "running", "")
	progress := func(_ int, msg string) {
		_ = st.UpdateJobProgress(jobID, msg)
	}
	cqs, dup, err := s.OntoChat.ExtractCQs(ctx, sess, progress)
	if err != nil && ctx.Err() != nil {
		_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "已取消能力问题抽取。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(jobID, "cancelled", "")
		return
	}
	if err != nil {
		_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "CQ 抽取失败：" + err.Error() + "\n可重试，或手动在首轮输入中补充能力问题。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(jobID, "error", err.Error())
		return
	}
	result := map[string]any{"cqs": cqs, "duplicate_count": dup}
	bts, _ := json.Marshal(result)
	_ = st.SetJobResult(jobID, bts)
	_ = st.UpdateJobStatus(jobID, "done", "")
	var b strings.Builder
	fmt.Fprintf(&b, "已抽取能力问题候选 %d 条", len(cqs))
	if dup > 0 {
		fmt.Fprintf(&b, "（与已有重复滤除 %d 条）", dup)
	}
	b.WriteString("。请在确认卡中逐条编辑/勾选后写入会话——确认后生成草稿将据此补充建模，并把能力问题回写进本体资产（spec.CQ 可追溯）。")
	_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: b.String()}, nil, nil, nil)
}

// ontoChatSetCQs POST /api/ontochat/sessions/{id}/cqs {cqs: [...]} → 人工确认写入会话上下文
// （analyze 确认步定案保留）；生成轮自此经 DraftWithCQ 回写 spec.CQ（REQ-248 缺口闭合）。
func (s *Server) ontoChatSetCQs(w http.ResponseWriter, r *http.Request) {
	st := s.ontoChatStore()
	sess, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if sess.Stage == "done" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "会话已结束，无法修改能力问题"})
		return
	}
	var req struct {
		CQs []string `json:"cqs"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	cqs := make([]string, 0, len(req.CQs))
	seen := map[string]bool{}
	for _, c := range req.CQs {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		cqs = append(cqs, c)
	}
	if len(cqs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cqs 不能为空（清空请逐条删除后不提交）"})
		return
	}
	if err := st.UpdateContext(sess.ID, func(c *ontochat.Context) { c.CQs = cqs }); err != nil {
		writeErr(w, err)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "已确认能力问题 %d 条：", len(cqs))
	for i, q := range cqs {
		fmt.Fprintf(&b, "\n%d. %s", i+1, q)
	}
	b.WriteString("\n生成草稿时将据此补充建模，并回写 spec.CQ 入资产可追溯。")
	if err := st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: b.String()}, nil, nil, nil); err != nil {
		writeErr(w, err)
		return
	}
	fresh, err := st.Get(sess.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": fresh})
}

// ontoChatCQAnalyze POST /api/ontochat/sessions/{id}/cq-analyze {max_clusters?} → 202 job（REQ-273）：
// 会话 CQs 去重+主题聚类，终态 result={clusters,dedup_count}；应用写回复用 POST /cqs（人工确认步）。
func (s *Server) ontoChatCQAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	st := s.ontoChatStore()
	sess, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if sess.Stage == "done" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "会话已结束，如需继续请新建会话"})
		return
	}
	if len(sess.Context.CQs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "会话尚无能力问题：请先「抽取 CQ」或手动补充后再分析"})
		return
	}
	var req struct {
		MaxClusters int `json:"max_clusters"`
	}
	_ = decodeJSON(r, &req)
	if active, aerr := st.ActiveJobBySession(sess.ID); aerr == nil && active != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "已有生成任务进行中，请等待完成或取消后再试"})
		return
	}
	job, err := st.CreateJob(newOntoJobID(), sess.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID})
	go s.runOntoChatCQAnalyze(job.ID, sess, req.MaxClusters)
}

// runOntoChatCQAnalyze 后台执行 CQ 分析：进度 → AnalyzeCQs → 终态落 job + 错误留痕。
func (s *Server) runOntoChatCQAnalyze(jobID string, sess *ontochat.Session, maxClusters int) {
	st := s.ontoChatStore()
	ctx, release := s.registerOntoJobCancel(jobID)
	defer release()
	_ = st.UpdateJobStatus(jobID, "running", "")
	progress := func(_ int, msg string) {
		_ = st.UpdateJobProgress(jobID, msg)
	}
	clusters, dedup, err := s.OntoChat.AnalyzeCQs(ctx, sess, maxClusters, progress)
	if err != nil && ctx.Err() != nil {
		_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "已取消能力问题分析。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(jobID, "cancelled", "")
		return
	}
	if err != nil {
		_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "CQ 分析失败：" + err.Error() + "\n可重试，或直接人工整理能力问题。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(jobID, "error", err.Error())
		return
	}
	result := map[string]any{"clusters": clusters, "dedup_count": dedup}
	bts, _ := json.Marshal(result)
	_ = st.SetJobResult(jobID, bts)
	_ = st.UpdateJobStatus(jobID, "done", "")
	var b strings.Builder
	fmt.Fprintf(&b, "CQ 分析完成：%d 个主题簇，去重合并 %d 条（语义等价保留最清晰表述）。请在分析卡中确认后应用写回——聚类仅供参考分组，不能单独支撑完整分析。", len(clusters), dedup)
	_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: b.String()}, nil, nil, nil)
}

// ontoCoverageTest POST /api/ontologies/{id}/cq-coverage {cqs?} → 202 job（REQ-274）：
// CQ 覆盖测试快筛（默认关——本端点即质量卡显式开启动作）；cqs 缺省用 spec.CQ。
// job session_id 用合成标识 onto:{ontologyID}（本体级任务，无会话消息留痕）。
func (s *Server) ontoCoverageTest(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	oid := r.PathValue("id")
	raw, _, err := s.Store.GetArtifact(oid, "spec_json")
	if err != nil {
		writeErr(w, err)
		return
	}
	var sp pkgspec.Spec
	if err := json.Unmarshal([]byte(raw), &sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "spec 解析失败: " + err.Error()})
		return
	}
	var req struct {
		CQs []string `json:"cqs"`
	}
	_ = decodeJSON(r, &req)
	cqs := req.CQs
	if len(cqs) == 0 {
		cqs = sp.CQ
	}
	if len(cqs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "本体的 spec.CQ 为空：请先在构建会话抽取并确认能力问题（生成草稿时回写），或显式传入 cqs"})
		return
	}
	if active, aerr := s.ontoChatStore().ActiveJobBySession("onto:" + oid); aerr == nil && active != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "该本体已有测试任务进行中，请等待完成或取消后再试"})
		return
	}
	job, err := s.ontoChatStore().CreateJob(newOntoJobID(), "onto:"+oid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID, "total": len(cqs)})
	go s.runOntoCoverageTest(job.ID, &sp, cqs)
}

// runOntoCoverageTest 后台执行覆盖测试：逐条进度 → TestCoverage → 终态落 job（无会话留痕）。
func (s *Server) runOntoCoverageTest(jobID string, sp *pkgspec.Spec, cqs []string) {
	ctx, release := s.registerOntoJobCancel(jobID)
	defer release()
	_ = s.ontoChatStore().UpdateJobStatus(jobID, "running", "")
	progress := func(done, total int, msg string) {
		_ = s.ontoChatStore().UpdateJobProgress(jobID, fmt.Sprintf("%s（%d/%d）", msg, done, total))
	}
	verdicts, err := s.OntoChat.TestCoverage(ctx, sp, cqs, progress)
	if err != nil && ctx.Err() != nil {
		_ = s.ontoChatStore().UpdateJobStatus(jobID, "cancelled", "")
		return
	}
	if err != nil {
		_ = s.ontoChatStore().UpdateJobStatus(jobID, "error", err.Error())
		return
	}
	sum := ontochat.SummarizeCoverage(verdicts)
	bts, _ := json.Marshal(sum)
	_ = s.ontoChatStore().SetJobResult(jobID, bts)
	_ = s.ontoChatStore().UpdateJobStatus(jobID, "done", "")
}

// ontoChatStoryBack POST /api/ontochat/sessions/{id}/story-back → 访气回退一步（REQ-275 不变式③）。
func (s *Server) ontoChatStoryBack(w http.ResponseWriter, r *http.Request) {
	st := s.ontoChatStore()
	sess, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if sess.Stage != "story" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "仅访谈阶段支持回退"})
		return
	}
	res, err := s.OntoChat.StoryBack(st, sess)
	if err != nil {
		writeErr(w, err)
		return
	}
	fresh, gerr := st.Get(sess.ID)
	if gerr != nil {
		writeErr(w, gerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reply": res.Reply, "stage": res.NextStage, "session": fresh})
}

// ontoChatStoryFinish POST /api/ontochat/sessions/{id}/story-finish → 202 job（REQ-275）：
// 访谈完成——以用户故事为材料抽 CQ 候选（REQ-272 复用）并推进 story→cq；候选经确认卡写回。
func (s *Server) ontoChatStoryFinish(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM 辅助创建未配置（PLATFORM_URL）"})
		return
	}
	st := s.ontoChatStore()
	sess, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if sess.Stage != "story" || strings.TrimSpace(sess.Context.DraftStory) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "访谈尚未生成用户故事，无法完成"})
		return
	}
	if active, aerr := st.ActiveJobBySession(sess.ID); aerr == nil && active != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "已有生成任务进行中，请等待完成或取消后再试"})
		return
	}
	job, err := st.CreateJob(newOntoJobID(), sess.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job.ID})
	go s.runOntoChatStoryFinish(job.ID, sess)
}

// runOntoChatStoryFinish 后台：以故事为材料抽 CQ → 阶段推进 story→cq + 留痕。
func (s *Server) runOntoChatStoryFinish(jobID string, sess *ontochat.Session) {
	st := s.ontoChatStore()
	ctx, release := s.registerOntoJobCancel(jobID)
	defer release()
	_ = st.UpdateJobStatus(jobID, "running", "")
	progress := func(_ int, msg string) {
		_ = st.UpdateJobProgress(jobID, msg)
	}
	cqs, dup, err := s.OntoChat.ExtractCQs(ctx, sess, progress)
	if err != nil && ctx.Err() != nil {
		_ = st.UpdateJobStatus(jobID, "cancelled", "")
		return
	}
	if err != nil {
		_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: "CQ 抽取失败：" + err.Error() + "\n可重试，或手动在输入框按「能力问题：」格式补充。"}, nil, nil, nil)
		_ = st.UpdateJobStatus(jobID, "error", err.Error())
		return
	}
	// 阶段推进 story→domain（描述与 CQ 已就绪：候选经 REQ-272 确认卡写回后即可「生成草稿」；
	// 进 domain 而非 cq——cq 是快速模式首轮「录入描述」语义，避免后续输入被当新领域描述）
	stage := "domain"
	_ = st.Append(sess.ID, ontochat.Message{Role: "assistant", Content: fmt.Sprintf("访谈完成。已从用户故事抽取能力问题候选 %d 条（去重滤除 %d 条），请在确认卡中勾选编辑后写入——确认后可直接「生成草稿」或继续补充领域信息。", len(cqs), dup)}, &stage, nil, nil)
	result := map[string]any{"cqs": cqs, "duplicate_count": dup}
	bts, _ := json.Marshal(result)
	_ = st.SetJobResult(jobID, bts)
	_ = st.UpdateJobStatus(jobID, "done", "")
}

// ontoChatStoryTemplates GET /api/ontochat/story-templates → 引导卡模板（REQ-275，P3 中文适配只读透出）。
func (s *Server) ontoChatStoryTemplates(w http.ResponseWriter, _ *http.Request) {
	out := make([]map[string]string, 0, len(ontochat.StoryTemplates))
	for _, t := range ontochat.StoryTemplates {
		out = append(out, map[string]string{"label": t.Label, "text": t.Text})
	}
	writeJSON(w, http.StatusOK, out)
}

// listOntoChatPrompts GET /api/ontochat/prompts → 提示词清单（只读，REQ-271⑥ 定案：只显示不可修改；
// 返回值即运行时注入的同一批常量，页面显示零复制）。
func (s *Server) listOntoChatPrompts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ontochat.Prompts())
}

// ontoChatSave POST /api/ontochat/sessions/{id}/save {name} → 草稿入库（预览确认门控，REQ-82）
func (s *Server) ontoChatSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	st := s.ontoChatStore()
	sess, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if sess.Context.DraftSpec == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "会话尚无草稿，请先完成生成轮"})
		return
	}
	var sp pkgspec.Spec
	if err := json.Unmarshal(*sess.Context.DraftSpec, &sp); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "草稿解析失败: " + err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = sp.Name
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请提供本体名称"})
		return
	}
	sp.Name = name
	if errs := sp.Validate(); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "草稿校验未通过", "validation_errors": errs})
		return
	}
	o, err := s.Store.CreateOntology(newID(), name, sp.Description)
	if err != nil {
		writeErr(w, err)
		return
	}
	bts, _ := json.Marshal(sp)
	if err := s.Store.PutArtifact(o.ID, "spec_json", string(bts), true); err != nil {
		writeErr(w, err)
		return
	}
	_ = s.Store.SaveVersion(o.ID, 1, string(bts), "", "")
	// REQ-275：访谈制品归档——用户故事落 artifact story_md 供学习中心/任务卡援引（REQ-91）
	if sess.Context.DraftStory != "" {
		_ = s.Store.PutArtifact(o.ID, "story_md", sess.Context.DraftStory, true)
	}
	_ = st.BindOntology(sess.ID, o.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"ontology": o, "session": mustSession(st, sess.ID)})
}

// ontoChatStore 惰性初始化会话存储（表由 migrations/003_ontochat.sql 建）。
func (s *Server) ontoChatStore() *ontochat.Store {
	if s.ontoChatDB == nil {
		s.ontoChatDB = ontochat.New(s.Store.DB())
	}
	return s.ontoChatDB
}

func mustSession(st *ontochat.Store, id string) *ontochat.Session {
	sess, err := st.Get(id)
	if err != nil {
		return nil
	}
	return sess
}

func newOntoJobID() string {
	return fmt.Sprintf("ontoj_%d", time.Now().UnixNano())
}
