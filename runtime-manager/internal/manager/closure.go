package manager

// REQ-480/M280：本体组装载闭包与装载门禁——Start 装载链沿仓内 external_refs
//（to_onto_id 非空的 import/reuse）+owl:imports 登记（哨兵 import，to_concept=urn:o:{id}:）
// 递归补全挂载清单：visited set 收敛，互引环按并集装载（=RDF/OWL imports 环语义）并在
// 装载报告呈现；悬空引用（to_onto_id 非空而目标本体已删/概念缺失，含 imports 登记指向
// 已不存在的仓内本体）阻断启动——阻断在装载链不在质量门禁（§2.67 拍板口径）；
// 远端 IRI（非 urn:o 形态）不拉取维持（D-O27 边界）。载体=运行方案 OntologyIDs
// 多对多（方案即部署视图），不新建「本体组」资产实体。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// extRef 构建平面引用登记投影（GET /api/ontologies/{id}/external-refs 的 refs 项；
// dangling 为 REQ-480 结构化悬空标注：""|missing_ontology|missing_concept）。
type extRef struct {
	FromConcept string `json:"from_concept"`
	ToOntoID    string `json:"to_onto_id"`
	ToConcept   string `json:"to_concept"`
	RefKind     string `json:"ref_kind"`
	PinVersion  int    `json:"pin_version"` // REQ-540：>0=钉住目标本体该 spec 版本（0=浮动头版本）
	Dangling    string `json:"dangling"`
}

// closureReport 闭包装载报告（装载日志呈现 + 环标注进 loadedStatus）。
type closureReport struct {
	Declared      []string   `json:"declared"`
	Effective     []string   `json:"effective"` // 声明 ∪ 闭包补全（BFS 序）
	Added         []string   `json:"added"`     // 闭包补全（不在声明集内的部分）
	Cycles        [][]string `json:"cycles,omitempty"`
	RemoteSkipped []string   `json:"remote_skipped,omitempty"`
	// REQ-540/M347：pin 解析结果（目标本体 → 有效 pin 版本）——Start 装载链按此取
	// 对应版本导出（FetchTTLWithCountAt）；同目标多登记 pin 不一致取最低（最保守解析）。
	// 来源=BFS 访问集内各本体登记的 pin_version>0 引用（不限 ref_kind——登记即 pin 载体）。
	Pins map[string]int `json:"pins,omitempty"`
}

// FetchExternalRefs 构建平面本体引用登记清单。404=本体不存在（闭包目标悬空/已删除，
// 由调用方阻断）；网络失败=构建平面不可达（阻断——引用清单已是装载关键数据）。
func (m *Manager) FetchExternalRefs(ontologyID string) ([]extRef, error) {
	resp, err := m.HTTP.Get(fmt.Sprintf("%s/api/ontologies/%s/external-refs", m.BuildURL, ontologyID))
	if err != nil {
		return nil, fmt.Errorf("构建平面不可达: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("本体 %s 不存在（owl:imports 闭包目标悬空或已删除——REQ-480② 装载门禁）", ontologyID)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("引用清单获取失败（HTTP %d: %.120s）", resp.StatusCode, string(b))
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var out struct {
		Refs []extRef `json:"refs"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("引用清单解析失败: %w", err)
	}
	return out.Refs, nil
}

// urnOntoOf owl:imports 登记的 urn:o:{id}: 形态 IRI → 仓内本体 ID；非该形态=远端（空串，
// 不拉取维持）。ID 含 sanitize 变形字符时解析失配按远端跳过并如实注记（诚实边界）。
func urnOntoOf(iri string) string {
	iri = strings.TrimSpace(iri)
	if !strings.HasPrefix(iri, "urn:o:") || !strings.HasSuffix(iri, ":") {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(iri, "urn:o:"), ":")
	if id == "" || strings.ContainsAny(id, " <>\"{}|\\^`") {
		return ""
	}
	return id
}

// expandClosure 闭包 BFS：从声明清单沿引用边递归补全，返回有效装载清单与报告。
// 悬空阻断条件（REQ-480②）：①仓内登记（to_onto_id 非空）dangling 非空；
// ②owl:imports 哨兵登记指向 urn:o:{x}: 而本体 x 不存在（BFS 取清单 404）。
func (m *Manager) expandClosure(declared []string) (*closureReport, error) {
	rep := &closureReport{Declared: append([]string(nil), declared...), Pins: map[string]int{}}
	visited := map[string]bool{}
	queue := append([]string(nil), declared...)
	edges := map[string][]string{}
	remoteSeen := map[string]bool{}
	for len(queue) > 0 {
		oid := queue[0]
		queue = queue[1:]
		if visited[oid] {
			continue
		}
		visited[oid] = true
		rep.Effective = append(rep.Effective, oid)
		refs, err := m.FetchExternalRefs(oid)
		if err != nil {
			return nil, fmt.Errorf("闭包扩展失败（本体 %s）: %w", oid, err)
		}
		for _, ref := range refs {
			// REQ-540：pin 收集——to_onto_id 非空且 pin_version>0 的登记（不限 ref_kind，
			// 登记即 pin 载体）；同目标多登记取最低 pin（最保守解析，与 lifecycle ListPinEdges 一致）。
			if ref.ToOntoID != "" && ref.PinVersion > 0 {
				if prev, ok := rep.Pins[ref.ToOntoID]; !ok || ref.PinVersion < prev {
					rep.Pins[ref.ToOntoID] = ref.PinVersion
				}
			}
			switch {
			case ref.ToOntoID != "" && (ref.RefKind == "import" || ref.RefKind == "reuse"):
				if ref.Dangling != "" {
					return nil, fmt.Errorf("装载门禁阻断（REQ-480② 悬空引用）：本体 %s 的 %s 登记 %s → %s:%s 目标%s——悬空引用须先修复或摘除（资产页「关联本体」）",
						oid, ref.RefKind, ref.FromConcept, ref.ToOntoID, ref.ToConcept, danglingText(ref.Dangling))
				}
				edges[oid] = append(edges[oid], ref.ToOntoID)
				if !visited[ref.ToOntoID] {
					queue = append(queue, ref.ToOntoID)
				}
			case ref.ToOntoID == "" && ref.RefKind == "import" && ref.FromConcept == "(ontology)":
				// owl:imports 装载侧登记（REQ-262 哨兵）：to_concept=目标 IRI
				if tgt := urnOntoOf(ref.ToConcept); tgt != "" {
					edges[oid] = append(edges[oid], tgt)
					if !visited[tgt] {
						queue = append(queue, tgt)
					}
				} else if !remoteSeen[ref.ToConcept] {
					remoteSeen[ref.ToConcept] = true
					rep.RemoteSkipped = append(rep.RemoteSkipped, ref.ToConcept)
				}
			}
		}
	}
	if len(rep.Pins) == 0 {
		rep.Pins = nil
	}
	declaredSet := map[string]bool{}
	for _, d := range declared {
		declaredSet[d] = true
	}
	for _, oid := range rep.Effective {
		if !declaredSet[oid] {
			rep.Added = append(rep.Added, oid)
		}
	}
	rep.Cycles = findImportCycles(edges, declared)
	return rep, nil
}

func danglingText(code string) string {
	if code == "missing_ontology" {
		return "本体已删除"
	}
	if code == "missing_concept" {
		return "概念不存在"
	}
	return "悬空（" + code + "）"
}

// findImportCycles 彩色 DFS 找回边环（至多 10 条去重路径呈现；环语义=并集装载不受影响，
// 报告仅为可观测性）。
func findImportCycles(edges map[string][]string, roots []string) [][]string {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var cycles [][]string
	seen := map[string]bool{}
	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		if len(cycles) >= 10 {
			return
		}
		color[node] = gray
		path = append(path, node)
		for _, tgt := range edges[node] {
			switch color[tgt] {
			case gray:
				start := 0
				for j, p := range path {
					if p == tgt {
						start = j
						break
					}
				}
				cyc := append(append([]string(nil), path[start:]...), tgt)
				sig := fmt.Sprint(cyc)
				if !seen[sig] {
					seen[sig] = true
					cycles = append(cycles, cyc)
				}
			case white:
				dfs(tgt, path)
			}
		}
		color[node] = black
	}
	for _, r := range roots {
		if color[r] == white {
			dfs(r, nil)
		}
	}
	return cycles
}

// logLines 装载报告行（与引擎日志同文件，运行栏日志面板可见）。
func (r *closureReport) logLines() []string {
	var lines []string
	addedNote := "无"
	if len(r.Added) > 0 {
		addedNote = strings.Join(r.Added, ", ")
	}
	lines = append(lines, fmt.Sprintf("[REQ-480 闭包装载] 声明 [%s] → 有效清单 [%s]（闭包补全: %s）",
		strings.Join(r.Declared, ", "), strings.Join(r.Effective, ", "), addedNote))
	for _, cyc := range r.Cycles {
		lines = append(lines, fmt.Sprintf("[REQ-480 闭包装载] 互引环: %s（按并集装载=RDF/OWL imports 环语义）", strings.Join(cyc, " → ")))
	}
	for _, iri := range r.RemoteSkipped {
		lines = append(lines, fmt.Sprintf("[REQ-480 闭包装载] 远端 IRI 跳过（不拉取，D-O27）: %s", iri))
	}
	// REQ-540：pin 装载呈现（确定性序——目标本体字典序）
	targets := make([]string, 0, len(r.Pins))
	for t := range r.Pins {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, t := range targets {
		lines = append(lines, fmt.Sprintf("[REQ-540 pin 装载] %s → v%d（登记 pin 版本；被引方头版本演进不自动跟随，显式刷新 pin 才取新版）", t, r.Pins[t]))
	}
	return lines
}

// cycleTags 环标注（oid → 环路径串，进 loadedStatus 逐本体呈现）。
func (r *closureReport) cycleTags() map[string]string {
	out := map[string]string{}
	for _, cyc := range r.Cycles {
		tag := strings.Join(cyc, "⇄")
		for _, oid := range cyc {
			if _, ok := out[oid]; !ok {
				out[oid] = tag
			}
		}
	}
	return out
}

// appendProcLog 管理平面装载报告行（REQ-480：闭包/环/一致性检查呈现）。
// 引擎日志同文件追加——引擎以截断方式打开时本行可能在引擎启动前写入后被截断，
// 故调用时机在 startEngine 成功之后。
func (m *Manager) appendProcLog(id string, lines []string) {
	if len(lines) == 0 {
		return
	}
	f, err := os.OpenFile(filepath.Join(m.LogDir, id+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	for _, l := range lines {
		_, _ = fmt.Fprintf(f, "[%s] [manager] %s\n", ts, l)
	}
}

// preloadReasonCheck REQ-480③：装载前 owlrl 并置一致性检查（闭包全集 TTL 合并 → 构建平面
// /reason-check 端点）。策略落档=强告警不阻断（存量不一致不 brick 启动；结果留装载日志，
// 质量卡复核为修复动线）。任何失败降级为日志注记跳过（sidecar 未配置/构建平面不可达）。
func (m *Manager) preloadReasonCheck(ttls map[string]string) string {
	if len(ttls) == 0 {
		return ""
	}
	keys := make([]string, 0, len(ttls))
	for k := range ttls {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(ttls[k])
		b.WriteString("\n\n")
	}
	payload, _ := json.Marshal(map[string]string{"content": b.String(), "format": "turtle"})
	resp, err := m.HTTP.Post(m.BuildURL+"/api/ontologies/reason-check", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Sprintf("[REQ-480③ 装载前一致性检查] 跳过（构建平面不可达: %v）", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("[REQ-480③ 装载前一致性检查] 跳过（HTTP %d: %.160s）", resp.StatusCode, string(body))
	}
	var res struct {
		Consistent *bool    `json:"consistent"`
		Violations []string `json:"violations"`
		Error      string   `json:"error"`
	}
	if json.Unmarshal(body, &res) != nil {
		return "[REQ-480③ 装载前一致性检查] 跳过（结果解析失败）"
	}
	if res.Consistent == nil {
		return fmt.Sprintf("[REQ-480③ 装载前一致性检查] 不可判定（%s）", res.Error)
	}
	if *res.Consistent {
		return fmt.Sprintf("[REQ-480③ 装载前一致性检查] 一致（owlrl OWL 2 RL 并置 %d 本体）", len(ttls))
	}
	v := res.Violations
	if len(v) > 3 {
		v = v[:3]
	}
	return fmt.Sprintf("[REQ-480③ 装载前一致性检查] 不一致（强告警不阻断）：%d 违例 %v——建议质量卡复核", len(res.Violations), v)
}

// materializeInferred REQ-481/M281：组装载全集 TTL 并置 → 构建平面 /materialize 端点
// （owlrl OWL 2 RL 数据级闭包，axiomatic 词汇不入输出）。返回（推断图 IRI, 推断 TTL, 计数, 日志行）；
// 任何失败降级为日志注记（不阻断装载）。图命名：单本体 urn:inferred:{oid}；多本体
// urn:inferred:assembly——跨本体推断不可归因单本体，合并图=诚实粒度（§2.67「如
// urn:inferred:{oid}」示例性命名落此变体）；raw 图引擎侧恒 urn:onto:{oid} 双图隔离成立。
func (m *Manager) materializeInferred(ttls map[string]string) (string, string, int, string) {
	if len(ttls) == 0 {
		return "", "", 0, ""
	}
	keys := make([]string, 0, len(ttls))
	for k := range ttls {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(ttls[k])
		b.WriteString("\n\n")
	}
	payload, _ := json.Marshal(map[string]string{"content": b.String(), "format": "turtle"})
	resp, err := m.HTTP.Post(m.BuildURL+"/api/ontologies/materialize", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", "", 0, fmt.Sprintf("[REQ-481 装载期物化] 跳过（构建平面不可达: %v）", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", "", 0, fmt.Sprintf("[REQ-481 装载期物化] 跳过（HTTP %d: %.160s）", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	cnt := -1
	if h := resp.Header.Get("X-Onto-Inferred-Triples"); h != "" {
		if n, perr := strconv.Atoi(h); perr == nil {
			cnt = n
		}
	}
	gr := "urn:inferred:assembly"
	if len(keys) == 1 {
		gr = "urn:inferred:" + keys[0]
	}
	return gr, string(body), cnt, fmt.Sprintf("[REQ-481 装载期物化] owlrl OWL 2 RL 并置 %d 本体 → 推断 %s 三元组 → %s（raw/inferred 双图隔离，N²~N³ 膨胀风险在案观察）",
		len(keys), triplesText(cnt), gr)
}

func triplesText(n int) string {
	if n < 0 {
		return "未知计数"
	}
	return strconv.Itoa(n)
}
