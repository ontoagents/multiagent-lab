package manager

// REQ-377/M173：运行面数据/代码分离——伴生图持久化。
// 本体图（可重建的「代码」：Stop/Reload 后从构建平面重拉重装）与伴生子图（不可重建的
// 对话沉淀「数据」：随引擎数据目录全删而丢失）生命周期分离——Stop/Reload 前自动快照
// 伴生子图（named graph ont-{oid}，SPARQL SELECT 全量三元组带 term 类型落 JSON），
// Start 健康检查通过后回放恢复（INSERT DATA）。治「REQ-87 显式重载语义 × 伴生数据全删」
// 的结构冲突；数据目录全删重建语义仅保留给纯本体图（快照为空=无伴生数据，行为不变）。
//
// 诚实边界：①仅覆盖 named graph ont-{oid} 形态的伴生子图（companion host 默认图形态的
// 沉淀与基底本体混装于默认图，不可区分——backend REQ-368 伴生快照底盘为准）；②空图/
// 无快照=行为不变（零写放大）；③快照失败仅日志告警不阻断 Stop（Fail-safe：伴生数据
// 缺失可由对话再沉淀，不撒谎不阻断主链路）。

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

)

// companionGraphURI 伴生子图 URI（与 backend/internal/companion GraphURI 同值：ont-{oid}——
// REQ-368 双时序底盘同源；engine.GraphIRI〔urn:onto:{oid}〕是本体装载图，非伴生图）。
func companionGraphURI(ontologyID string) string { return "http://eino-lab/graph/ont-" + ontologyID }

// companionTerm 一个 RDF term（快照 JSON 载体——type: uri|literal|bnode，值与语言/数据类型随存）。
type companionTerm struct {
	Type    string `json:"type"`
	Value   string `json:"value"`
	Lang    string `json:"xml:lang,omitempty"`
	Datatype string `json:"datatype,omitempty"`
}

// companionGraphSnapshot 单本体伴生子图快照文件形态。
// snapshotDir 伴生子图快照目录（Manager.SnapshotDir，默认 data/companion-graph-snapshots）。
func (m *Manager) snapshotDir() string {
	if m.SnapshotDir != "" {
		return m.SnapshotDir
	}
	return filepath.Join("data", "companion-graph-snapshots")
}

// snapshotCompanionGraphs Stop/Reload 前调用：逐本体拉取伴生子图 named graph 全量三元组落盘。
func (m *Manager) snapshotCompanionGraphs(id string, oids []string, endpoint string) {
	if m.HTTP == nil {
		m.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	for _, oid := range oids {
		// 伴生子图 URI 与 backend companion.GraphURI / facade companionGraphURI 同值
		graph := companionGraphURI(oid)
		q := fmt.Sprintf("SELECT ?s ?p ?o WHERE { GRAPH <%s> { ?s ?p ?o } }", graph)
		bindings, err := sparqlSelect(m.HTTP, endpoint, q)
		if err != nil {
			log.Printf("[companion-persist] 方案 %s 本体 %s 伴生图快照查询失败（继续 Stop）： %v", id, oid, err)
			continue
		}
		if len(bindings) == 0 {
			_ = os.Remove(m.snapshotFile(oid)) // 空图：清除旧快照（重载后不回放已消失的伴生数据）
			continue
		}
		rows := make([][]companionTerm, 0, len(bindings))
		for _, b := range bindings {
			rows = append(rows, []companionTerm{toTerm(b["s"]), toTerm(b["p"]), toTerm(b["o"])})
		}
		snap := map[string]any{
			"ontology_id": oid, "graph": graph, "triples": rows,
			"saved_at": time.Now().UTC().Format(time.RFC3339), "count": len(rows),
		}
		b, jerr := json.Marshal(snap)
		if jerr != nil {
			log.Printf("[companion-persist] 快照序列化失败 oid=%s: %v", oid, jerr)
			continue
		}
		f := m.snapshotFile(oid)
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			log.Printf("[companion-persist] 快照目录创建失败: %v", err)
			continue
		}
		if err := os.WriteFile(f, b, 0o644); err != nil {
			log.Printf("[companion-persist] 快照写盘失败 oid=%s: %v", oid, err)
			continue
		}
		log.Printf("[companion-persist] 方案 %s 本体 %s 伴生图已快照（%d 三元组）——Stop/Reload 后自动回放", id, oid, len(rows))
	}
}

// restoreCompanionGraphs Start 健康检查通过后调用：存在快照的本体回放 INSERT DATA。
func (m *Manager) restoreCompanionGraphs(id string, oids []string, endpoint string) {
	if m.HTTP == nil {
		m.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	for _, oid := range oids {
		f := m.snapshotFile(oid)
		b, err := os.ReadFile(f)
		if err != nil {
			continue // 无快照=无伴生沉淀，行为不变
		}
		var snap struct {
			Triples [][]companionTerm `json:"triples"`
			Count   int               `json:"count"`
		}
		if json.Unmarshal(b, &snap) != nil || len(snap.Triples) == 0 {
			continue
		}
		graph := companionGraphURI(oid)
		var sb strings.Builder
		sb.WriteString("INSERT DATA { GRAPH <" + graph + "> {\n")
		for _, row := range snap.Triples {
			if len(row) != 3 {
				continue
			}
			sb.WriteString(termNT(row[0]) + " " + termNT(row[1]) + " " + termNT(row[2]) + " .\n")
		}
		sb.WriteString("} }")
		if err := sparqlUpdate(m.HTTP, endpoint, sb.String()); err != nil {
			log.Printf("[companion-persist] 方案 %s 本体 %s 伴生图回放失败（诚实边界：伴生数据可由对话再沉淀）：%v", id, oid, err)
			continue
		}
		log.Printf("[companion-persist] 方案 %s 本体 %s 伴生图已回放（%d 三元组，REQ-377）", id, oid, len(snap.Triples))
	}
}

// snapshotFile 快照落盘路径（按本体 id 单文件——伴生子图本体级共享，与方案解耦）。
func (m *Manager) snapshotFile(oid string) string {
	return filepath.Join(m.snapshotDir(), oid+".json")
}

// toTerm SPARQL JSON binding → term。
func toTerm(v any) companionTerm {
	mm, _ := v.(map[string]any)
	t := companionTerm{}
	if mm == nil {
		return t
	}
	t.Type, _ = mm["type"].(string)
	t.Value, _ = mm["value"].(string)
	t.Lang, _ = mm["xml:lang"].(string)
	t.Datatype, _ = mm["datatype"].(string)
	return t
}

// termNT term → SPARQL 注入形态（URI <>；字面量引号+lang/datatype；bnode [] 占位）。
func termNT(t companionTerm) string {
	switch t.Type {
	case "uri":
		return "<" + t.Value + ">"
	case "bnode":
		return "_:" + strings.ReplaceAll(t.Value, ":", "_")
	default: // literal
		v := strings.ReplaceAll(t.Value, "\\", "\\\\")
		v = strings.ReplaceAll(v, "\"", "\\\"")
		v = strings.ReplaceAll(v, "\n", "\\n")
		s := "\"" + v + "\""
		if t.Lang != "" {
			return s + "@" + t.Lang
		}
		if t.Datatype != "" {
			return s + "^^<" + t.Datatype + ">"
		}
		return s
	}
}

// sparqlSelect 执行 SELECT 返回 bindings。
func sparqlSelect(client *http.Client, endpoint, query string) ([]map[string]any, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/sparql-query")
	req.Header.Set("Accept", "application/sparql-results+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SPARQL 查询失败: %s", resp.Status)
	}
	var res struct {
		Results struct {
			Bindings []map[string]any `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Results.Bindings, nil
}

// sparqlUpdate 执行 UPDATE（回放 INSERT DATA）。
func sparqlUpdate(client *http.Client, endpoint, query string) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(query))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/sparql-update")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("SPARQL 更新失败: %s", resp.Status)
	}
	return nil
}
