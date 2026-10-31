package api

// REQ-495/M304 笔记本 Notes REST（自产成果文档资产层；详设 11 §2 REQ-495 行+三拍板+106 号宿主注）。
// 端点：笔记本 CRUD 五端点+页面 CRUD+两源同步（本地目录/git 本地克隆 pull+walk，mtime 触碰同步）
// +KB 回流登记（引用语义三拍板③——页触碰自动重索引引用 KB）。

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// noteDocMarker KB 回流文档确定性标题（页 ID 唯一→重索引 delete+import 幂等）。
func noteDocMarker(pageID, title string) string {
	return fmt.Sprintf("【笔记本】%s · %s", pageID[:8], title)
}

// touchSyncPage 页触碰→引用 KB 重索引（引用语义+触碰同步，三拍板③）。
func (s *Server) touchSyncPage(page *store.NotePage) {
	if s.KB == nil {
		return
	}
	kbIDs, err := s.Store.ListNotebookSourceKBs(page.NotebookID)
	if err != nil || len(kbIDs) == 0 {
		return
	}
	marker := noteDocMarker(page.ID, page.Title)
	for _, kbID := range kbIDs {
		docs, err := s.KB.Store.ListKnowledgeDocs(kbID)
		if err != nil {
			continue
		}
		for _, d := range docs {
			// 前缀全删（含同标记旧文档）——内容更新即替换（delete+import 幂等重索引），改名同理
			if strings.HasPrefix(d.Title, fmt.Sprintf("【笔记本】%s", page.ID[:8])) {
				_ = s.KB.DeleteDoc(context.Background(), kbID, d.ID)
			}
		}
		_, _ = s.KB.ImportAsync(context.Background(), kbID, marker, page.ContentMD)
	}
}

// touchRemovePage 页删除→引用 KB 移除对应语料文档。
func (s *Server) touchRemovePage(page *store.NotePage) {
	if s.KB == nil {
		return
	}
	kbIDs, err := s.Store.ListNotebookSourceKBs(page.NotebookID)
	if err != nil {
		return
	}
	for _, kbID := range kbIDs {
		docs, err := s.KB.Store.ListKnowledgeDocs(kbID)
		if err != nil {
			continue
		}
		for _, d := range docs {
			if strings.HasPrefix(d.Title, fmt.Sprintf("【笔记本】%s", page.ID[:8])) {
				_ = s.KB.DeleteDoc(context.Background(), kbID, d.ID)
			}
		}
	}
}

func (s *Server) listNotebooks(w http.ResponseWriter, r *http.Request) {
	ns, err := s.Store.ListNotebooks(r.URL.Query().Get("workspace_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if ns == nil {
		ns = []*store.Notebook{}
	}
	writeJSON(w, http.StatusOK, ns)
}

func (s *Server) createNotebook(w http.ResponseWriter, r *http.Request) {
	var n store.Notebook
	if err := decodeJSON(r, &n); err != nil {
		writeErr(w, err)
		return
	}
	created, err := s.Store.CreateNotebook(&n)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getNotebook(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.GetNotebook(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) updateNotebook(w http.ResponseWriter, r *http.Request) {
	var n store.Notebook
	if err := decodeJSON(r, &n); err != nil {
		writeErr(w, err)
		return
	}
	n.ID = r.PathValue("id")
	updated, err := s.Store.UpdateNotebook(&n)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteNotebook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// 引用 KB 语料清理（引用语义：笔记本删→登记 KB 移除对应页文档；登记关系随 CASCADE）
	pages, _ := s.Store.ListNotePages(id)
	for _, p := range pages {
		s.touchRemovePage(p)
	}
	if err := s.Store.DeleteNotebook(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}

func (s *Server) listNotePages(w http.ResponseWriter, r *http.Request) {
	pages, err := s.Store.ListNotePages(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if pages == nil {
		pages = []*store.NotePage{}
	}
	writeJSON(w, http.StatusOK, pages)
}

func (s *Server) createNotePage(w http.ResponseWriter, r *http.Request) {
	var p store.NotePage
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, err)
		return
	}
	p.NotebookID = r.PathValue("id")
	created, err := s.Store.CreateNotePage(&p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getNotePage(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetNotePage(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) updateNotePage(w http.ResponseWriter, r *http.Request) {
	var p store.NotePage
	if err := decodeJSON(r, &p); err != nil {
		writeErr(w, err)
		return
	}
	p.ID = r.PathValue("id")
	updated, err := s.Store.UpdateNotePage(&p)
	if err != nil {
		writeErr(w, err)
		return
	}
	// 触碰同步（三拍板③）：页更新→引用 KB 重索引（异步队列，ImportAsync 落库即返回）
	s.touchSyncPage(updated)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteNotePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	page, err := s.Store.GetNotePage(id)
	if err == nil {
		s.touchRemovePage(page)
	}
	if err := s.Store.DeleteNotePage(id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}

// syncNotebook 两源同步（MVP：本地目录+git 本地克隆路径；触碰同步=mtime 基线跳过未变页）。
// 请求 {path, kind}：kind=local 直 walk；kind=git 先 git pull --ff-only（10s 超时，失败 400 人话）。
func (s *Server) syncNotebook(w http.ResponseWriter, r *http.Request) {
	nbID := r.PathValue("id")
	if _, err := s.Store.GetNotebook(nbID); err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		Path string `json:"path"`
		Kind string `json:"kind"` // local | git
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path 必填"})
		return
	}
	kind := req.Kind
	if kind == "" {
		kind = "local"
	}
	if kind != "local" && kind != "git" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind must be local|git"})
		return
	}
	root := filepath.Clean(req.Path)
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目录不存在或不可读：请检查路径（本地目录源指向含 .md 的目录；git 源指向本地克隆路径）"})
		return
	}
	if kind == "git" {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "-C", root, "pull", "--ff-only")
		if out, err := cmd.CombinedOutput(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("git pull 失败（文档仓库源要求本地克隆且可快进）：%.200s", out)})
			return
		}
	}
	pages, _ := s.Store.ListNotePages(nbID)
	byRef := map[string]*store.NotePage{}
	for _, p := range pages {
		if p.SourceRef != "" {
			byRef[p.SourceRef] = p
		}
	}
	seen := map[string]bool{}
	added, updated, skipped := 0, 0, 0
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		if strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		mtime := info.ModTime().UTC().Format(time.RFC3339Nano)
		if old, ok := byRef[rel]; ok && old.SourceMtime == mtime {
			skipped++ // 触碰同步：未变更跳过
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if old, ok := byRef[rel]; ok {
			old.Title, old.ContentMD, old.SourceMtime = title, string(b), mtime
			if _, uerr := s.Store.UpdateNotePage(old); uerr == nil {
				updated++
				s.touchSyncPage(old)
			}
			return nil
		}
		np, cerr := s.Store.CreateNotePage(&store.NotePage{NotebookID: nbID, Title: title, ContentMD: string(b), Source: kind, SourceRef: rel, SourceMtime: mtime})
		if cerr == nil {
			added++
			s.touchSyncPage(np)
		}
		return nil
	})
	if walkErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("目录遍历失败：%v", walkErr)})
		return
	}
	// 源内删除的文件→对应源页移除（manual 手写页不触碰）
	removed := 0
	for ref, p := range byRef {
		if !seen[ref] && p.Source != "manual" {
			s.touchRemovePage(p)
			if err := s.Store.DeleteNotePage(p.ID); err == nil {
				removed++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "updated": updated, "skipped": skipped, "removed": removed, "kind": kind})
}

// ---- KB 回流登记（引用语义三拍板③） ----

func (s *Server) listNotebookSources(w http.ResponseWriter, r *http.Request) {
	rs, err := s.Store.ListNotebookSources(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if rs == nil {
		rs = []*store.KBNotebookSource{}
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) createNotebookSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KBID string `json:"kb_id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.KBID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kb_id 必填"})
		return
	}
	src, err := s.Store.CreateNotebookSource(req.KBID, r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	// 登记 即全量回流一次（既有页全部入 KB；此后增量随触碰同步）
	pages, _ := s.Store.ListNotePages(r.PathValue("id"))
	for _, p := range pages {
		s.touchSyncPage(p)
	}
	writeJSON(w, http.StatusCreated, src)
}

func (s *Server) deleteNotebookSource(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := s.Store.DeleteNotebookSource(q.Get("kb_id"), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}
