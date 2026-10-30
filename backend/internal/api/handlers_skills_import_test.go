package api

// REQ-371/M169：技能导入 handler 单测——zip 上传落库、同名跳过不覆盖、非 zip/无 SKILL.md 守卫。

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func newSkillImportTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	box, err := secrets.LoadKeyFile(filepath.Join(dir, ".secret"))
	if err != nil {
		t.Fatalf("打开 secrets box 失败: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	srv := &Server{Store: st, Box: box, Mux: http.NewServeMux()}
	srv.Mux.HandleFunc("POST /api/skills/import", srv.importSkills)
	srv.Mux.HandleFunc("GET /api/skills", srv.listSkills)
	ts := httptest.NewServer(srv.Mux)
	t.Cleanup(ts.Close)
	return ts
}

// skillImportZipBytes 构造 zip：skills/<name>/SKILL.md 两技能（附一个超 64KB 资源→触发诚实标注）。
func skillImportZipBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	big := bytes.Repeat([]byte("a"), 65*1024)
	for name, md := range map[string]string{
		"alpha/SKILL.md": "---\nname: 导入甲\ndescription: 甲描述\n---\n\n甲正文\n",
		"beta/SKILL.md":  "---\nname: 导入乙\n---\n乙正文\n",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(md)); err != nil {
			t.Fatal(err)
		}
	}
	w, err := zw.Create("alpha/big-ref.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(big); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func postImport(t *testing.T, ts *httptest.Server, fileField string, content []byte, filename string) (*http.Response, skillImportReport) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	resp, err := http.Post(ts.URL+"/api/skills/import", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var rep skillImportReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("响应非 JSON: %s", raw)
	}
	return resp, rep
}

func TestSkillImportCreatedAndDedup(t *testing.T) {
	ts := newSkillImportTestServer(t)
	zipBytes := skillImportZipBytes(t)

	resp, rep := postImport(t, ts, "file", zipBytes, "skills.zip")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("应 201: %d", resp.StatusCode)
	}
	if rep.Created != 2 || len(rep.Names) != 2 {
		t.Fatalf("应新建 2: %+v", rep)
	}
	// 诚实标注透传：导入甲附超 64KB 资源→notes 应有记录
	if len(rep.Notes) != 1 || rep.Notes[0].Skill != "导入甲" || len(rep.Notes[0].Notes) == 0 {
		t.Fatalf("超限资源应产生诚实标注: %+v", rep.Notes)
	}
	// 落库核验：指令与启用态
	list := listSkillsForTest(t, ts)
	byName := map[string]*store.Skill{}
	for _, sk := range list {
		byName[sk.Name] = sk
	}
	a := byName["导入甲"]
	if a == nil || a.Instruction != "甲正文" || a.Description != "甲描述" || !a.Enabled || a.Builtin {
		t.Fatalf("导入甲落库不符: %+v", a)
	}
	if b := byName["导入乙"]; b == nil || b.Instruction != "乙正文" {
		t.Fatalf("导入乙落库不符: %+v", b)
	}

	// 同包重传：同名跳过不覆盖（沿 M23 语义）
	resp2, rep2 := postImport(t, ts, "file", zipBytes, "skills.zip")
	if resp2.StatusCode != http.StatusCreated || rep2.Created != 0 || len(rep2.Skipped) != 2 {
		t.Fatalf("重传应 0 新建 2 跳过: %d %+v", resp2.StatusCode, rep2)
	}
	if !bytes.Contains([]byte(rep2.Skipped[0].Reason), []byte("同名")) {
		t.Fatalf("跳过原因应含「同名」: %+v", rep2.Skipped)
	}
}

func TestSkillImportGuards(t *testing.T) {
	ts := newSkillImportTestServer(t)

	// 非 zip 内容 → 400
	resp, _ := postImport(t, ts, "file", []byte("plain text"), "x.zip")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("非 zip 应 400: %d", resp.StatusCode)
	}

	// zip 内无 SKILL.md → 400
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("readme.txt")
	w.Write([]byte("no skill here"))
	zw.Close()
	resp2, _ := postImport(t, ts, "file", buf.Bytes(), "empty.zip")
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("无 SKILL.md 应 400: %d", resp2.StatusCode)
	}

	// 缺 file 字段 → 400
	resp3, err := http.Post(ts.URL+"/api/skills/import", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺 file 字段应 400: %d", resp3.StatusCode)
	}
}

func listSkillsForTest(t *testing.T, ts *httptest.Server) []*store.Skill {
	t.Helper()
	resp, err := http.Get(ts.URL + "/api/skills")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out []*store.Skill
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("列表非 JSON: %s", raw)
	}
	return out
}
