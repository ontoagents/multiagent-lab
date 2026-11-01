package api

// REQ-524/M331（Q-23 衍生③ 转正——学习动线出题·批改·掌握度最小闭环，docs/03 §2.71）：
// 出题=LLM 按本体 spec 概念子图出题（chat.GenerateText 直调平台模型——与 assistantOptimize
// 同设施），可选 check_answerable 覆盖度预检随行（M331 详设：预检覆盖度）；
// 作答→LLM 批改（对照参考答案按 rubric 打分 0~1）→关联概念掌握度落表（0.7 旧+0.3 新加权）。
// REQ-572/M380（Q-23 衍生③ 排期转正——闭环化三段，01 §3.24）：①出题定向与禁重复=掌握度薄弱
// （score<0.5）与从未考察概念优先 + 历史题干注入禁重复（归一精确判重；语义级禁重复由提示承载
// ——诚实边界）+题型混合（choice 单选/judge 判断=客观确定性判定不走 LLM，short_answer 简答=
// 主观 LLM 批改）②批改链落档=quiz_attempt 每次提交一条（题目/作答/裁决/概念快照可审计——沿
// 本体审计叙事）③概念子图掌握度=mastery-graph 端点（spec concepts+relations 与 mastery_level
// 合并出节点/边；前端子图渲染），出题定向即其消费面（下一轮出题按掌握度定向）。
// 地基复用：rest_cqbench Dataset/gold 判定链为题库判分器雏形（详设口径），本件取 GenerateText
// 最小直调面（与平台助手同族，非对话链路）。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

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
	Parents    []string `json:"parents"`
}

// quizRelation 关系面（REQ-572 mastery-graph 子图边——仅取概念层对象属性 From/To）。
type quizRelation struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// quizSpec concepts/relations 面最小形状（反代读构建平面 spec 的解析目标）。
type quizSpec struct {
	Concepts  []quizConcept  `json:"concepts"`
	Relations []quizRelation `json:"relations"`
}

// quizGenerate POST /api/quiz/{ontology_id}/generate：{difficulty?, count?, conn_id?}
// → {items:[{id,question,answer,difficulty,question_type,options,concept_names}]}（answer 前端作答后才展示）。
// REQ-572：目标概念定向（薄弱/从未考察优先）+历史题干禁重复（归一判重）+题型混合。
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
	// REQ-572 定向与禁重复数据源：掌握度（薄弱概念）+ 最近出题（已考察概念+历史题干）。
	mastery, _ := s.Store.ListMastery(ontologyID)
	recent, _ := s.Store.ListQuizItems(ontologyID, 20)
	targets := quizTargetConcepts(spec, mastery, recent)
	banned := make([]string, 0, len(recent))
	for _, it := range recent {
		banned = append(banned, it.Question)
	}
	cj, _ := json.Marshal(spec.Concepts)
	system := "你是学习助手的出题器。根据给定本体的概念列表出题：只输出一个 JSON 数组，每项 {\"type\":题型, \"question\":问题, \"answer\":参考答案（含依据的概念与理由）, \"options\":[选项（仅客观题给）], \"concepts\":[关联概念名（须严格取自概念列表的 name）]}。题型三选一：\"short_answer\"=围绕概念定义与概念间关系的简答题；\"choice\"=单选题（options 给 3~4 个选项，answer=正确选项原文）；\"judge\"=判断题（options 固定 [\"正确\",\"错误\"]，answer=正确选项）。不要编造列表外的概念。给出的题目不要与「禁止重复」列表中任何一题重复（换说法重复同一考点也算）。"
	user := "难度=" + in.Difficulty + "（basic=概念定义与直接关系；applied=需组合多个概念推理应用）。出 " +
		fmt.Sprintf("%d", in.Count) + " 题，题型混合：至少 1 道客观题（choice 或 judge），其余简答。" +
		quizTargetPrompt(targets) +
		"概念列表（name/label/definition/synonyms）：\n" + string(cj)
	if len(banned) > 0 {
		bj, _ := json.Marshal(banned)
		user += "\n禁止重复（近期已出题目，勿再出相同或同考点换说法）：\n" + string(bj)
	}
	ctx, cancel := context.WithTimeout(r.Context(), quizGenerateTimeout)
	defer cancel()
	raw, err := quizLLM(ctx, s.Store, s.Box, in.ConnID, system, user, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	var questions []struct {
		Type     string   `json:"type"`
		Question string   `json:"question"`
		Answer   string   `json:"answer"`
		Options  []string `json:"options"`
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
	seen := map[string]bool{} // 本批内去重
	for _, b := range banned {
		seen[quizNormQ(b)] = true
	}
	out := []*store.QuizItem{}
	for _, q := range questions {
		if q.Question == "" {
			continue
		}
		if seen[quizNormQ(q.Question)] { // REQ-572 禁重复：历史+本批归一精确判重
			continue
		}
		qtype, options := quizNormalizeType(q.Type, q.Options, q.Answer)
		concepts := []string{}
		for _, c := range q.Concepts {
			if valid[c] {
				concepts = append(concepts, c)
			}
		}
		item, ierr := s.Store.InsertQuizItem(&store.QuizItem{
			OntologyID: ontologyID, ConceptNames: concepts,
			Question: q.Question, Answer: q.Answer, Difficulty: in.Difficulty,
			QuestionType: qtype, Options: options,
		})
		if ierr != nil {
			writeErr(w, ierr)
			return
		}
		seen[quizNormQ(q.Question)] = true
		out = append(out, item)
	}
	if len(out) == 0 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "出题输出无有效题目（均与近期题目重复或解析失败）——请重试"})
		return
	}
	// REQ-545/M352：check_answerable 出题预检接线——逐题确定性预检（运行平面 REST 口），
	// 裁决落 quiz_item.coverage_verdict（answerable|not_answerable；失败/无运行方案=留空「未预检」
	// 如实不冒充）。异步非阻塞：出题响应即刻返回，预检结果前端刷新可见。
	go s.quizCoveragePreset(ontologyID, out)
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// quizTargetConcepts REQ-572 定向选点：薄弱（掌握度<0.5）优先，其次从未考察概念（无掌握度记录
// 且近期未出过），各取前 6 合并 ≤12；全已覆盖且无薄弱=空（提示词不限定向，自由出题）。
func quizTargetConcepts(spec *quizSpec, mastery []*store.MasteryLevel, recent []*store.QuizItem) []string {
	weakSet := map[string]bool{}
	for _, m := range mastery {
		if m.Score < 0.5 {
			weakSet[m.ConceptName] = true
		}
	}
	askedSet := map[string]bool{}
	for _, it := range recent {
		for _, c := range it.ConceptNames {
			askedSet[c] = true
		}
	}
	ranked := make([]string, 0, len(spec.Concepts))
	never := make([]string, 0)
	for _, c := range spec.Concepts {
		if weakSet[c.Name] {
			ranked = append(ranked, c.Name)
		} else if !askedSet[c.Name] {
			matched := false
			for _, m := range mastery {
				if m.ConceptName == c.Name {
					matched = true
					break
				}
			}
			if !matched {
				never = append(never, c.Name)
			}
		}
	}
	sort.Strings(never) // 确定性序
	out := ranked
	if len(out) > 6 {
		out = out[:6]
	}
	for _, c := range never {
		if len(out) >= 12 {
			break
		}
		out = append(out, c)
	}
	return out
}

// quizTargetPrompt 定向指令（targets 空=不限定向）。
func quizTargetPrompt(targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	tj, _ := json.Marshal(targets)
	return "定向要求：优先围绕这些概念出题（掌握度薄弱或从未考察）：" + string(tj) + "。\n"
}

// quizNormalizeType REQ-572 题型归一：choice 须 ≥2 选项且答案在选项内（否则降级 short_answer
// 如实不冒充客观题）；judge 选项固定 [正确,错误]。
func quizNormalizeType(t string, options []string, answer string) (string, []string) {
	trimmed := make([]string, 0, len(options))
	for _, o := range options {
		o = strings.TrimSpace(o)
		if o != "" {
			trimmed = append(trimmed, o)
		}
	}
	switch t {
	case "choice":
		if len(trimmed) >= 2 && quizContains(trimmed, strings.TrimSpace(answer)) {
			return "choice", trimmed
		}
	case "judge":
		if quizContains(trimmed, strings.TrimSpace(answer)) {
			return "judge", []string{"正确", "错误"}
		}
		if strings.TrimSpace(answer) == "正确" || strings.TrimSpace(answer) == "错误" {
			return "judge", []string{"正确", "错误"}
		}
	}
	return "short_answer", nil
}

func quizContains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// quizNormQ 题干归一（禁重复判重：去全部空白+小写——CJK 无语义空格变体一并归一）。
func quizNormQ(q string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, q))
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
// → {score, verdict, feedback, concepts}（批改+落档+掌握度落表）。
// REQ-572：客观题（choice/judge）确定性判定不走 LLM；主观题 LLM 批改；两轨均落 quiz_attempt 可审计。
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
	var grading struct {
		Score    float64 `json:"score"`
		Verdict  string  `json:"verdict"`
		Feedback string  `json:"feedback"`
	}
	if q.QuestionType == "choice" || q.QuestionType == "judge" {
		// REQ-572 客观题确定性判定：选项原文精确比对（前端 radio 只回选项原文）。
		given := strings.TrimSpace(in.Answer)
		want := strings.TrimSpace(q.Answer)
		if given == want {
			grading = struct {
				Score    float64 `json:"score"`
				Verdict  string  `json:"verdict"`
				Feedback string  `json:"feedback"`
			}{1, "correct", "回答正确"}
		} else {
			grading = struct {
				Score    float64 `json:"score"`
				Verdict  string  `json:"verdict"`
				Feedback string  `json:"feedback"`
			}{0, "wrong", "正确答案：" + want}
		}
	} else {
		system := "你是学习助手的批改器。对照参考答案评阅用户作答，只输出 JSON：{\"score\":0到1的数字, \"verdict\":\"correct\"|\"partial\"|\"wrong\", \"feedback\":一两句中文点评（答对了给确认，答错了指出缺口）}。评分口径：要点全覆盖且正确=0.9~1；部分要点=0.4~0.8；答非所问/错误=0~0.3。"
		user := "题目：" + q.Question + "\n\n参考答案：" + q.Answer + "\n\n用户作答：" + in.Answer
		ctx, cancel := context.WithTimeout(r.Context(), quizGenerateTimeout)
		defer cancel()
		raw, err := quizLLM(ctx, s.Store, s.Box, in.ConnID, system, user, nil)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := json.Unmarshal(chat.ExtractJSON(raw), &grading); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "批改输出解析失败（模型未返回 JSON）——请重试"})
			return
		}
		if grading.Verdict != "correct" && grading.Verdict != "partial" && grading.Verdict != "wrong" {
			grading.Verdict = "partial"
		}
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
	// REQ-572 批改落档：题目/作答/裁决/概念快照一条不落（可审计——掌握度影响可回溯）。
	if _, aerr := s.Store.InsertQuizAttempt(&store.QuizAttempt{
		OntologyID: ontologyID, QuizItemID: qid, QuestionType: q.QuestionType,
		Answer: in.Answer, Score: grading.Score, Verdict: grading.Verdict,
		Feedback: grading.Feedback, ConceptNames: concepts,
	}); aerr != nil {
		writeErr(w, aerr)
		return
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

// quizMasteryGraph GET /api/quiz/{ontology_id}/mastery-graph：REQ-572 概念子图掌握度——
// spec concepts+relations 与 mastery_level 合并出节点（covered=有掌握度：score/asked/correct；
// uncovered=从未考察）与边（声明关系+父概念继承两族），前端子图渲染；出题定向即其消费面。
func (s *Server) quizMasteryGraph(w http.ResponseWriter, r *http.Request) {
	ontologyID := r.PathValue("id")
	spec, err := s.fetchOntologySpec(r.Context(), ontologyID)
	if err != nil {
		writeErr(w, err)
		return
	}
	mastery, err := s.Store.ListMastery(ontologyID)
	if err != nil {
		writeErr(w, err)
		return
	}
	mb := map[string]*store.MasteryLevel{}
	for _, m := range mastery {
		mb[m.ConceptName] = m
	}
	type graphNode struct {
		Name    string   `json:"name"`
		Label   string   `json:"label"`
		Covered bool     `json:"covered"`
		Score   float64  `json:"score"`
		Asked   int      `json:"asked"`
		Correct int      `json:"correct"`
		Parents []string `json:"parents,omitempty"`
	}
	nodes := []graphNode{}
	inSpec := map[string]bool{}
	for _, c := range spec.Concepts {
		inSpec[c.Name] = true
		n := graphNode{Name: c.Name, Label: c.Label, Parents: c.Parents}
		if m, ok := mb[c.Name]; ok {
			n.Covered = true
			n.Score = m.Score
			n.Asked = m.AskedCount
			n.Correct = m.CorrectCount
		}
		nodes = append(nodes, n)
	}
	type graphEdge struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Kind   string `json:"kind"` // relation | parent
		Rel    string `json:"rel,omitempty"`
		RelLbl string `json:"rel_label,omitempty"`
	}
	edges := []graphEdge{}
	for _, rel := range spec.Relations {
		if inSpec[rel.From] && inSpec[rel.To] { // 悬空引用不入图（spec 校验兜底外的防御）
			edges = append(edges, graphEdge{From: rel.From, To: rel.To, Kind: "relation", Rel: rel.Name, RelLbl: rel.Label})
		}
	}
	for _, c := range spec.Concepts {
		for _, p := range c.Parents {
			if inSpec[p] {
				edges = append(edges, graphEdge{From: c.Name, To: p, Kind: "parent"})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": edges})
}

// quizAttempts GET /api/quiz/{ontology_id}/attempts：REQ-572 批改记录（可审计面，时间倒序，
// 联题目摘要）。
func (s *Server) quizAttempts(w http.ResponseWriter, r *http.Request) {
	ontologyID := r.PathValue("id")
	attempts, err := s.Store.ListQuizAttempts(ontologyID, 20)
	if err != nil {
		writeErr(w, err)
		return
	}
	items, _ := s.Store.ListQuizItems(ontologyID, 100)
	qmap := map[string]string{}
	for _, it := range items {
		qmap[it.ID] = it.Question
	}
	out := make([]map[string]any, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, map[string]any{
			"id": a.ID, "quiz_item_id": a.QuizItemID, "question": qmap[a.QuizItemID],
			"question_type": a.QuestionType, "answer": a.Answer, "score": a.Score,
			"verdict": a.Verdict, "feedback": a.Feedback, "concept_names": a.ConceptNames,
			"created_at": a.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempts": out})
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

// fetchOntologySpec 经构建平面读本体 spec（concepts+relations 面）——反代同源地址直调（服务间，非透传）。
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
