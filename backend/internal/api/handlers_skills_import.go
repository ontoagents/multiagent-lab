package api

// REQ-371/M169：目录式技能包批量导入——POST /api/skills/import（multipart 字段 file，zip）。
// 语义沿 M23 配置导入=同名跳过不覆盖（含 builtin 行，冲突即跳过）；单技能解析失败不中断整批；
// 工具面不映射（allowed-tools 等置空，由使用者在平台工具注册表内自行勾选）。

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ontoagents/ontoagents/backend/internal/skill"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// 报告列表上限：单次导入可能上百技能，超限截断并诚实标注（计数仍完整）。
const skillImportListCap = 300

type skillImportReport struct {
	Created     int              `json:"created"`
	CreatedIDs  []string         `json:"-"` // 新建成功件 id（按包归集用，不外露）
	Names       []string         `json:"names,omitempty"`
	Skipped     []skill.SkipItem `json:"skipped,omitempty"`
	Failed      []skill.SkipItem `json:"failed,omitempty"`
	Notes       []skillNoteItem  `json:"notes,omitempty"` // 资源超限/正文截断等诚实标注（仅列有标注的技能）
	ListsShrunk bool             `json:"lists_shrunk,omitempty"`
	Collection  *importCollect   `json:"collection,omitempty"` // REQ-373：按包归集结果
}

// importCollect 按包归集结果（REQ-373）：collection_name 非空时，本次新建成功的技能
// 归集为一卡集合；跳过/失败件不入集（诚实计数）。
type importCollect struct {
	Name    string `json:"name"`
	Members int    `json:"members"`
}

// skillNoteItem 单技能的导入标注（截断/资源跳过不静默）。
type skillNoteItem struct {
	Skill string   `json:"skill"`
	Notes []string `json:"notes"`
}

func (s *Server) importSkills(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 沿上传体积防线口径
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的 multipart 上传（或超过 64MB 上限）"})
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart file 字段必填（zip 格式技能包）"})
		return
	}
	defer f.Close()
	imp, err := skill.ParseSkillZip(f, hdr.Size)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	report := &skillImportReport{Names: []string{}}
	appendSkip := func(list *[]skill.SkipItem, it skill.SkipItem) {
		if len(*list) < skillImportListCap {
			*list = append(*list, it)
		} else {
			report.ListsShrunk = true
		}
	}
	for _, p := range imp.Skills {
		sk := &store.Skill{
			Name:        p.Name,
			Description: p.Description,
			Instruction: p.Instruction,
			Tools:       []string{},
			Resources:   p.Resources,
			Enabled:     true, // 新建即启用，沿 createSkill 口径
		}
		created, err := s.Store.CreateSkill(sk)
		if err != nil {
			var he *store.HTTPError
			if errors.As(err, &he) && he.Status == http.StatusConflict {
				appendSkip(&report.Skipped, skill.SkipItem{Name: p.Name, Reason: "同名技能已存在，跳过不覆盖（沿配置导入口径）"})
			} else {
				appendSkip(&report.Failed, skill.SkipItem{Name: p.Name, Reason: err.Error()})
			}
			continue
		}
		report.Created++
		report.CreatedIDs = append(report.CreatedIDs, created.ID)
		report.Names = append(report.Names, created.Name)
		if len(p.Notes) > 0 {
			if len(report.Notes) < skillImportListCap {
				report.Notes = append(report.Notes, skillNoteItem{Skill: created.Name, Notes: p.Notes})
			} else {
				report.ListsShrunk = true
			}
		}
	}
	report.Skipped = append(report.Skipped, imp.Skipped...)
	report.Failed = append(report.Failed, imp.Failed...)
	// REQ-373 按包归集：collection_name 非空且本次有新建成功件时，归集为一卡集合
	//（同名集合已存在=不归集如实标注，沿同名跳过不覆盖口径；跳过/失败件不入集）。
	if cname := strings.TrimSpace(r.FormValue("collection_name")); cname != "" {
		if report.Created == 0 {
			report.Notes = append(report.Notes, skillNoteItem{Skill: cname, Notes: []string{"无新建技能（全部跳过/失败），未创建集合"}})
		} else if len(report.CreatedIDs) > store.MaxCollectionChildren {
			report.Notes = append(report.Notes, skillNoteItem{Skill: cname, Notes: []string{fmt.Sprintf("新建 %d 件超过集合成员上限 %d，未创建集合", report.Created, store.MaxCollectionChildren)}})
		} else if _, err := s.Store.CreateSkill(&store.Skill{
			Name:        cname,
			Description: fmt.Sprintf("技能包导入归集（新建 %d 件）", report.Created),
			Kind:        "collection",
			Children:    report.CreatedIDs,
			Enabled:     true,
		}); err != nil {
			var heC *store.HTTPError
			if errors.As(err, &heC) && heC.Status == http.StatusConflict {
				report.Notes = append(report.Notes, skillNoteItem{Skill: cname, Notes: []string{"同名技能已存在，未创建集合（沿同名跳过口径）"}})
			} else {
				report.Notes = append(report.Notes, skillNoteItem{Skill: cname, Notes: []string{"创建集合失败：" + err.Error()}})
			}
		} else {
			report.Collection = &importCollect{Name: cname, Members: report.Created}
		}
	}
	writeJSON(w, http.StatusCreated, report)
}
