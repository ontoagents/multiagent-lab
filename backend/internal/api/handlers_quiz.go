package api

// REQ-524/M331（Q-23 衍生③ 转正——学习动线出题·批改·掌握度最小闭环，docs/03 §2.71）：
// 出题=LLM 按本体 spec 概念子图出题（chat.GenerateText 直调平台模型——与 assistantOptimize
// 同设施），可选 check_answerable 覆盖度预检随行（M331 详设：预检覆盖度）；
// 作答→LLM 批改（对照参考答案按 rubric 打分 0~1）→关联概念掌握度落表（0.7 旧+0.3 新加权）。
// 地基复用：rest_cqbench Dataset/gold 判定链为题库判分器雏形（详设口径），本件取 GenerateText
// 最小直调面（与平台助手同族，非对话链路）。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/chat"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// quizGenerateTimeout 出题/批改 LLM 直调超时（与 optimizeTimeout 同档）。
const quizGenerateTimeout = 90 * time.Second

// quizLLM LLM 直调注入点（默认 chat.GenerateText；handler 级单测注入桩——REQ-524 冒烟
// 走 mock 命中时掌握度真链以单测承载）。签名同 GenerateText。
var quizLLM = chat.GenerateText

type quizConcept struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Definition string   `json:"definition"`
	Synonyms   []string `json:"synonyms"`
}

// quizSpec concepts 面最小形状（反代读构建平面 spec 的解析目标）。
type quizSpec struct {
	Concepts []quizConcept `json:"concepts"`
}

// quizGenerate POST /api/quiz/{ontology_id}/generate：{difficulty?, count?, conn_id?}
// → {items:[{id,question,answer,difficulty,concept_names}]}（answer 前端作答后才展示）。
func (s *Server) quizGenerate(w http.ResponseWriter, r *http.Request) {
	ontologyID := r.PathValue("id")
	var in struct {
		Difficulty string `json:"difficulty"`
		Count      int    `json:"count"`
		ConnID     string `json:"conn_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.Count <= 0 || in.Count > 5 {
		in.Count = 3
	}
	if in.Difficulty != "applied" {
		in.Difficulty = "basic"
	}
	spec, err := s.fetchOntologySpec(r.Context(), ontologyID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(spec.Concepts) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该本体无概念（spec.concepts 为空）——先补概念再出题"})
		return
	}
	cj, _ := json.Marshal(spec.Concepts)
	system := "你是学习助手的出题器。根据给定本体的概念列表出题：只输出一个 JSON 数组，每项 {\"question\":问题, \"answer\":参考答案（含依据的概念与理由）, \"concepts\":[关联概念名（须严格取自概念列表的 name）]}。题目类型=围绕概念定义与概念间关系的简答题；不要编造列表外的概念。"
	user := "难度=" + in.Difficulty + "（basic=概念定义与直接关系；applied=需组合多个概念推理应用）。出 " +
		fmt.Sprintf("%d", in.Count) + " 题。概念列表（name/label/definition/synonyms）：\n" + string(cj)
	ctx, cancel := context.WithTimeout(r.Context(), quizGenerateTimeout)
	defer cancel()
	raw, err := quizLLM(ctx, s.Store, s.Box, in.ConnID, system, user, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	var questions []struct {
		Question string   `json:"question"`
		Answer   string   `json:"answer"`
		Concepts []string `json:"concepts"`
	}
	if err := json.Unmarshal(chat.ExtractJSON(raw), &questions); err != nil || len(questions) == 0 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出题输出解析失败（模型未返回 JSON 数组）——请重试或更换模型连接"})
		return
	}
	valid := map[string]bool{}
	for _, c := range spec.Concepts {
		valid[c.Name] = true
	}
	out := []*store.QuizItem{}
	for _, q := range questions {
		if q.Question == "" {
			continue
		}
		concepts := []string{}
		for _, c := range q.Concepts {
			if valid[c] {
				concepts = append(concepts, c)
			}
		}
		item, ierr := s.Store.InsertQuizItem(&store.QuizItem{
			OntologyID: ontologyID, ConceptNames: concepts,
			Question: q.Question, Answer: q.Answer, Difficulty: in.Difficulty,
		})
		if ierr != nil {
			writeErr(w, ierr)
			return
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出题输出无有效题目——请重试"})
		return
	}
	// REQ-545/M352：check_answerable 出题预检接线——逐题确定性预检（运行平面 REST 口），
	// 裁决落 quiz_item.coverage_verdict（answerable|not_answerable；失败/无运行方案=留空「未预检」
	// 如实不冒充）。异步非阻塞：出题响应即刻返回，预检结果前端刷新可见。
	go s.quizCoveragePreset(ontologyID, out)
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// quizCoveragePreset REQ-545/M352：逐题预检裁决回写（best-effort——任何失败静默留空）。
func (s *Server) quizCoveragePreset(ontologyID string, items []*store.QuizItem) {
	rtURL := os.Getenv("RUNTIME_MGR_URL")
	if rtURL == "" {
		rtURL = "http://127.0.0.1:8090"
	}
	client := &http.Client{Timeout: 15 * time.Second}
	for _, it := range items {
		body, _ := json.Marshal(map[string]string{"ontology_id": ontologyID, "question": it.Question})
		resp, err := client.Post(strings.TrimRight(rtURL, "/")+"/api/check-answerable", "application/json", bytes.NewReader(body))
		if err != nil {
			return // 运行平面不可达：整批留空（未预检如实态）
		}
		var out struct {
			Answerable bool `json:"answerable"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		verdict := "not_answerable"
		if out.Answerable {
			verdict = "answerable"
		}
		if err := s.Store.UpdateQuizCoverage(it.ID, verdict); err != nil {
			return
		}
	}
}

// quizSubmit POST /api/quiz/{ontology_id}/items/{qid}/submit：{answer, conn_id?}
// → {score, verdict, feedback, concepts}（批改+掌握度落表）。
func (s *Server) quizSubmit(w http.ResponseWriter, r *http.Request) {
	ontologyID := r.PathValue("id")
	qid := r.PathValue("qid")
	var in struct {
		Answer string `json:"answer"`
		ConnID string `json:"conn_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	q, err := s.Store.GetQuizItem(qid)
	if err != nil {
		writeErr(w, err)
		return
	}
	if q.OntologyID != ontologyID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "出题不属于该本体"})
		return
	}
	system := "你是学习助手的批改器。对照参考答案评阅用户作答，只输出 JSON：{\"score\":0到1的数字, \"verdict\":\"correct\"|\"partial\"|\"wrong\", \"feedback\":一两句中文点评（答对了给确认，答错了指出缺口）}。评分口径：要点全覆盖且正确=0.9~1；部分要点=0.4~0.8；答非所问/错误=0~0.3。"
	user := "题目：" + q.Question + "\n\n参考答案：" + q.Answer + "\n\n用户作答：" + in.Answer
	ctx, cancel := context.WithTimeout(r.Context(), quizGenerateTimeout)
	defer cancel()
	raw, err := quizLLM(ctx, s.Store, s.Box, in.ConnID, system, user, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	var grading struct {
		Score    float64 `json:"score"`
		Verdict  string  `json:"verdict"`
		Feedback string  `json:"feedback"`
	}
	if err := json.Unmarshal(chat.ExtractJSON(raw), &grading); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "批改输出解析失败（模型未返回 JSON）——请重试"})
		return
	}
	if grading.Verdict != "correct" && grading.Verdict != "partial" && grading.Verdict != "wrong" {
		grading.Verdict = "partial"
	}
	concepts := q.ConceptNames
	if len(concepts) == 0 {
		concepts = []string{"（未标注概念）"}
	}
	for _, c := range concepts {
		if uerr := s.Store.UpsertMastery(ontologyID, c, grading.Verdict == "correct", grading.Score); uerr != nil {
			writeErr(w, uerr)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"score": grading.Score, "verdict": grading.Verdict, "feedback": grading.Feedback, "concepts": concepts,
	})
}

// quizMastery GET /api/quiz/{ontology_id}/mastery：概念级掌握度（score 升序=薄弱优先）。
func (s *Server) quizMastery(w http.ResponseWriter, r *http.Request) {
	ms, err := s.Store.ListMastery(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mastery": ms})
}

// quizHistory GET /api/quiz/{ontology_id}/items：最近出题记录。
func (s *Server) quizHistory(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListQuizItems(r.PathValue("id"), 20)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// fetchOntologySpec 经构建平面读本体 spec（concepts 面）——反代同源地址直调（服务间，非透传）。
func (s *Server) fetchOntologySpec(ctx context.Context, ontologyID string) (*quizSpec, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Ontology.BuildURL+"/api/ontologies/"+ontologyID+"/spec", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &store.HTTPError{Status: http.StatusBadGateway, Msg: "本体服务不可达: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &store.HTTPError{Status: http.StatusBadGateway, Msg: "读取本体 spec 失败（HTTP " + fmt.Sprintf("%d", resp.StatusCode) + "）"}
	}
	var spec quizSpec
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return nil, &store.HTTPError{Status: http.StatusBadGateway, Msg: "spec 解析失败: " + err.Error()}
	}
	return &spec, nil
}
