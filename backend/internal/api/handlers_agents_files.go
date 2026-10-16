// REQ-218④/M49：智能体文件视图——以 agent.work_dir 为安全根浏览（沿项目 dir-files 先例，
// fsutil.SafeJoin 防越界；单层列表 + 1MB 文本预览）。「设为工作目录」走既有 PUT /api/agents/{id}
// 全量载荷（agentFullPayload），不另设端点；装配期 resolveWorkRoot 随新值生效=新 Run 文件工具安全根切换。
package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/fsutil"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// agentWorkRoot REQ-336：智能体文件根——显式 work_dir 优先；未设置时回落默认目录
// <DB 同级>/agent-files/{agent_id}（web=仓库 data/agent-files/；客户端=data.location 数据目录内，
// 天然满足「客户端默认文件路径在客户端数据目录中」），按需创建。侧板文件视图/对话附加文件/
// run_command 执行根共用此解析。
func (s *Server) agentWorkRoot(a *store.Agent) (string, error) {
	if strings.TrimSpace(a.WorkDir) != "" {
		return fsutil.NormalizeDir(a.WorkDir), nil
	}
	root := filepath.Join(filepath.Dir(s.DBPath), "agent-files", a.ID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

// listAgentDirFiles GET /api/agents/{id}/dir-files?path=<sub>：以 work_dir 为根单层列出。
func (s *Server) listAgentDirFiles(w http.ResponseWriter, r *http.Request) {
	a, err := s.Store.GetAgent(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	root, err := s.agentWorkRoot(a)
	if err != nil {
		writeErr(w, err)
		return
	}
	sub := strings.TrimSpace(r.URL.Query().Get("path"))
	full, err := fsutil.SafeJoin(root, sub)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径越界"})
		return
	}
	des, err := os.ReadDir(full)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "目录不存在或不可读（检查工作目录是否有效）"})
		return
	}
	entries := make([]dirFileEntry, 0, len(des))
	for _, de := range des {
		info, ierr := de.Info()
		if ierr != nil {
			continue
		}
		e := dirFileEntry{Name: de.Name(), IsDir: de.IsDir(), Size: info.Size(), ModTime: info.ModTime().UTC().Format(time.RFC3339)}
		if de.IsDir() {
			e.Size = 0
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // 目录优先
		}
		return entries[i].Name < entries[j].Name
	})
	relOut := filepath.ToSlash(filepath.Clean(sub))
	if sub == "" {
		relOut = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": relOut, "root": root, "entries": entries})
}

// getAgentDirFile GET /api/agents/{id}/dir-file?path=<rel>：读取 work_dir 内文件文本预览（1MB 上限）。
func (s *Server) getAgentDirFile(w http.ResponseWriter, r *http.Request) {
	a, err := s.Store.GetAgent(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	root, err := s.agentWorkRoot(a)
	if err != nil {
		writeErr(w, err)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path 必填"})
		return
	}
	full, err := fsutil.SafeJoin(root, rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径越界"})
		return
	}
	fi, err := os.Stat(full)
	if err != nil || fi.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "文件不存在"})
		return
	}
	if fi.Size() > 1<<20 { // 1MB 预览上限（沿项目口径）
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件过大，请用本地编辑器打开"})
		return
	}
	b, err := os.ReadFile(full)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", contentTypeOf(full))
	_, _ = w.Write(b)
}

// uploadAgentDirFile POST /api/agents/{id}/files（multipart，字段名 file）：上传文件入智能体文件根
// （REQ-336：对话窗口附加文件落位——文件进工作目录、消息文本附路径引用；同名覆盖）。
func (s *Server) uploadAgentDirFile(w http.ResponseWriter, r *http.Request) {
	a, err := s.Store.GetAgent(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	root, err := s.agentWorkRoot(a)
	if err != nil {
		writeErr(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20) // 单文件 64MB 上限（沿导入体积防线口径）
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的 multipart 上传（或超过 64MB 上限）"})
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart file 字段必填"})
		return
	}
	defer f.Close()
	name := filepath.Base(strings.ReplaceAll(hdr.Filename, "\\", "/"))
	if name == "" || name == "." || name == "/" || strings.HasPrefix(name, ".") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效文件名"})
		return
	}
	full, err := fsutil.SafeJoin(root, name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "路径越界"})
		return
	}
	out, err := os.Create(full)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer out.Close()
	n, err := io.Copy(out, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"path": name, "root": root, "size": n})
}
