// handlers_settings.go 配置面导入/导出（M23①，REQ-113 互补——配置面 vs 记录面）。
//
// GET  /api/settings/export → 四域配置 JSON（模型连接/智能体/项目/技能；key 脱敏不导出）
// POST /api/settings/import ← 同形态文件 → 按名去重保守导入（已存在跳过不覆盖；连接不带 key，
//
//	导入后需手工重录——脱敏导出的安全边界）
//
// 诚实边界：导出为换机/备份便利面非审计快照（不含对话/事件等记录面——REQ-113① 已有对话导出）；
// 连接密钥经 secrets.Box 加密落库，明文不出导出文件；内置助手/内置技能不入导出（平台自带随版本走）。
package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

const settingsExportFormat = "ontoagents-settings"

// settingsExportFormatLegacy REQ-539 平台更名前的导出格式值：导入侧继续接受旧导出文件（读旧写新）。
const settingsExportFormatLegacy = "multiagent-lab-settings"

type settingsExport struct {
	Format           string                   `json:"format"`
	Version          int                      `json:"version"`
	ExportedAt       string                   `json:"exported_at"`
	Counts           map[string]int           `json:"counts"`
	ModelConnections []*store.ModelConnection `json:"model_connections"`
	Agents           []*store.Agent           `json:"agents"`
	Projects         []*store.Project         `json:"projects"`
	Skills           []*store.Skill           `json:"skills"`
}

// exportSettings GET /api/settings/export：四域配置导出（key 脱敏）。
func (s *Server) exportSettings(w http.ResponseWriter, _ *http.Request) {
	out := &settingsExport{
		Format:     settingsExportFormat,
		Version:    1,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Counts:     map[string]int{},
	}
	if conns, err := s.Store.ListConnections(); err == nil {
		for _, c := range conns {
			// key 脱敏：明文/密文均不导出；has_key 标注原实例存有 key（导入后需重录）
			cc := *c
			cc.APIKey = ""
			cc.APIKeyHint = ""
			out.ModelConnections = append(out.ModelConnections, &cc)
		}
	}
	if agents, err := s.Store.ListAgents(); err == nil {
		for _, a := range agents {
			if a.IsBuiltin {
				continue // 内置助手随平台版本走，不入配置导出
			}
			out.Agents = append(out.Agents, a)
		}
	}
	if projects, err := s.Store.ListProjects(); err == nil {
		out.Projects = projects
	}
	if skills, err := s.Store.ListSkills(); err == nil {
		for _, sk := range skills {
			if sk.Builtin {
				continue
			}
			out.Skills = append(out.Skills, sk)
		}
	}
	out.Counts["model_connections"] = len(out.ModelConnections)
	out.Counts["agents"] = len(out.Agents)
	out.Counts["projects"] = len(out.Projects)
	out.Counts["skills"] = len(out.Skills)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=ontoagents-settings-%s.json", time.Now().UTC().Format("20060102-150405")))
	writeJSON(w, http.StatusOK, out)
}

type importSummary struct {
	Created int      `json:"created"`
	Skipped []string `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// importSettings POST /api/settings/import：按名去重保守导入（已存在跳过不覆盖）。
func (s *Server) importSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsExport
	if err := decodeJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "导入文件解析失败: " + err.Error()})
		return
	}
	if in.Format != "" && in.Format != settingsExportFormat && in.Format != settingsExportFormatLegacy {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("format=%q 不是 %s 导出文件", in.Format, settingsExportFormat)})
		return
	}
	out := map[string]*importSummary{}
	sum := func(domain string) *importSummary {
		if out[domain] == nil {
			out[domain] = &importSummary{Skipped: []string{}}
		}
		return out[domain]
	}

	// ---- 模型连接（按名去重；不带 key 导入——脱敏边界，导入后需重录）----
	existingByName := map[string]bool{}
	if conns, err := s.Store.ListConnections(); err == nil {
		for _, c := range conns {
			existingByName[c.Name] = true
		}
	}
	for _, c := range in.ModelConnections {
		if c == nil || strings.TrimSpace(c.Name) == "" {
			continue
		}
		if existingByName[c.Name] {
			sum("model_connections").Skipped = append(sum("model_connections").Skipped, c.Name)
			continue
		}
		cc := *c
		cc.ID = "" // 新建重派 id
		cc.APIKey = ""
		cc.APIKeyHint = ""
		cc.HasKey = false
		cc.IsDefault = false // 默认位不迁移（导入侧以本机现状为准）
		if _, err := s.Store.CreateConnection(&cc, nil); err != nil {
			sum("model_connections").Failed++
			sum("model_connections").Errors = append(sum("model_connections").Errors, c.Name+": "+err.Error())
			continue
		}
		existingByName[c.Name] = true
		sum("model_connections").Created++
	}

	// ---- 智能体（按名去重；内置行不导入）----
	agentNames := map[string]bool{}
	if agents, err := s.Store.ListAgents(); err == nil {
		for _, a := range agents {
			agentNames[a.Name] = true
		}
	}
	for _, a := range in.Agents {
		if a == nil || strings.TrimSpace(a.Name) == "" || a.IsBuiltin {
			continue
		}
		if agentNames[a.Name] {
			sum("agents").Skipped = append(sum("agents").Skipped, a.Name)
			continue
		}
		na := *a
		na.ID = ""
		na.IsBuiltin = false
		na.McpServe = store.McpServe{} // 对外服务不随导入迁移（安全面：token 不跨机复制）
		if na.MaxIteration <= 0 {
			na.MaxIteration = 25
		}
		if na.RuntimeBackend == "" {
			na.RuntimeBackend = "inprocess"
		}
		if _, err := s.Store.CreateAgent(&na); err != nil {
			sum("agents").Failed++
			sum("agents").Errors = append(sum("agents").Errors, a.Name+": "+err.Error())
			continue
		}
		agentNames[na.Name] = true
		sum("agents").Created++
	}

	// ---- 项目（按名去重）----
	projectNames := map[string]bool{}
	if projects, err := s.Store.ListProjects(); err == nil {
		for _, pj := range projects {
			projectNames[pj.Name] = true
		}
	}
	for _, pj := range in.Projects {
		if pj == nil || strings.TrimSpace(pj.Name) == "" {
			continue
		}
		if projectNames[pj.Name] {
			sum("projects").Skipped = append(sum("projects").Skipped, pj.Name)
			continue
		}
		np := *pj
		np.ID = ""
		if _, err := s.Store.CreateProject(&np); err != nil {
			sum("projects").Failed++
			sum("projects").Errors = append(sum("projects").Errors, pj.Name+": "+err.Error())
			continue
		}
		projectNames[np.Name] = true
		sum("projects").Created++
	}

	// ---- 技能（按名去重；内置不入）----
	skillNames := map[string]bool{}
	if skills, err := s.Store.ListSkills(); err == nil {
		for _, sk := range skills {
			skillNames[sk.Name] = true
		}
	}
	for _, sk := range in.Skills {
		if sk == nil || strings.TrimSpace(sk.Name) == "" || sk.Builtin {
			continue
		}
		if skillNames[sk.Name] {
			sum("skills").Skipped = append(sum("skills").Skipped, sk.Name)
			continue
		}
		nsk := *sk
		nsk.ID = ""
		nsk.Builtin = false
		if _, err := s.Store.CreateSkill(&nsk); err != nil {
			sum("skills").Failed++
			sum("skills").Errors = append(sum("skills").Errors, sk.Name+": "+err.Error())
			continue
		}
		skillNames[nsk.Name] = true
		sum("skills").Created++
	}

	writeJSON(w, http.StatusOK, map[string]any{"imported": out})
}
