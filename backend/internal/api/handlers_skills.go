package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// skillFirstLine 取正文首行截断（集合预览中无描述成员的回退描述）。
func skillFirstLine(s string, n int) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// GET /api/skills：技能列表（P1，方案 §6.12）。
func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.Store.ListSkills()
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skills)
}

// normalizeSkillKind 校验与归一 kind/children（REQ-373）：kind 仅 ''|collection；
// 集合 children 经存在性与非嵌套校验并去重；普通技能 children 恒空。
func (s *Server) normalizeSkillKind(sk *store.Skill) error {
	switch sk.Kind {
	case "", "collection":
	default:
		return &store.HTTPError{Status: 400, Msg: "kind 仅支持空（普通技能）或 collection（技能集合）"}
	}
	if sk.Kind != "collection" {
		sk.Children = nil
		return nil
	}
	sk.Instruction = "" // 集合无正文（成员正文经 load_skill 懒加载）
	sk.Tools = nil      // 工具白名单装配期取成员并集
	normalized, err := s.Store.ValidateCollectionChildren(sk.ID, sk.Children)
	if err != nil {
		return err
	}
	sk.Children = normalized
	return nil
}

// POST /api/skills：新建技能。
func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	var sk store.Skill
	if err := decodeJSON(r, &sk); err != nil {
		writeErr(w, err)
		return
	}
	sk.Enabled = true // 新建即启用；停用走 PUT（请求体 enabled 字段忽略）
	if err := s.normalizeSkillKind(&sk); err != nil {
		writeErr(w, err)
		return
	}
	created, err := s.Store.CreateSkill(&sk)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GET /api/skills/{id}：技能详情。
func (s *Server) getSkill(w http.ResponseWriter, r *http.Request) {
	sk, err := s.Store.GetSkill(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sk)
}

// PUT /api/skills/{id}：更新技能（builtin 只可停用与改内容，不可改名隐藏？——
// 保持简单：builtin 允许编辑与停用，不允许删除；name 唯一性由 store 保证）。
func (s *Server) updateSkill(w http.ResponseWriter, r *http.Request) {
	var sk store.Skill
	if err := decodeJSON(r, &sk); err != nil {
		writeErr(w, err)
		return
	}
	id := r.PathValue("id")
	old, err := s.Store.GetSkill(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	sk.ID = id
	sk.Builtin = old.Builtin // builtin 标记不可篡改
	sk.Kind = old.Kind       // kind 创建后不可变（集合/普通互转不做）
	if err := s.normalizeSkillKind(&sk); err != nil {
		writeErr(w, err)
		return
	}
	updated, err := s.Store.UpdateSkill(&sk)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/skills/{id}：删除技能（内置技能禁止删除，返回 409 提示停用）。
func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	old, err := s.Store.GetSkill(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if old.Builtin {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "内置技能不可删除，可停用"})
		return
	}
	if err := s.Store.DeleteSkill(id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/skills/{id}/preview：注入预览——该技能注入后的指令块与工具白名单（§6.12 LG-16）。
// REQ-373：集合预览=按启用成员展开的目录（与装配期 ComposeInstruction 目录形态同构），
// 工具白名单为启用成员并集；停用集合/无启用成员诚实空块。
func (s *Server) previewSkill(w http.ResponseWriter, r *http.Request) {
	sk, err := s.Store.GetSkill(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	block := ""
	tools := sk.Tools
	if sk.Kind == "collection" {
		var b strings.Builder
		toolSeen := map[string]bool{}
		tools = nil
		if sk.Enabled {
			enabled := 0
			for _, cid := range sk.Children {
				m, err := s.Store.GetSkill(cid)
				if err != nil || m == nil || !m.Enabled {
					continue
				}
				desc := strings.TrimSpace(m.Description)
				if desc == "" {
					desc = skillFirstLine(m.Instruction, 60)
				}
				b.WriteString("- 「" + m.Name + "」：" + desc + "\n")
				for _, t := range m.Tools {
					if t != "" && !toolSeen[t] {
						toolSeen[t] = true
						tools = append(tools, t)
					}
				}
				enabled++
			}
			block = "# 技能集合「" + sk.Name + "」展开（启用 " + strconv.Itoa(enabled) + " 件；正文经 load_skill 懒加载）\n" + b.String()
		}
	} else if sk.Enabled {
		block = "<skill name=\"" + sk.Name + "\">" + sk.Instruction + "</skill>"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                sk.ID,
		"name":              sk.Name,
		"kind":              sk.Kind,
		"enabled":           sk.Enabled,
		"instruction_block": block,
		"tools":             tools,
	})
}
