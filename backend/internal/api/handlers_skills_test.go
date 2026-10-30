package api

// REQ-373/M171：技能集合 handler 单测——create/preview 集合形态、children 校验、
// zip 导入按包归集（新建件入集/同名集合跳过/零新建不建集）。

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func newSkillTestServer(t *testing.T) *httptest.Server {
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
	srv.Mux.HandleFunc("POST /api/skills", srv.createSkill)
	srv.Mux.HandleFunc("GET /api/skills", srv.listSkills)
	srv.Mux.HandleFunc("POST /api/skills/import", srv.importSkills)
	srv.Mux.HandleFunc("GET /api/skills/{id}/preview", srv.previewSkill)
	ts := httptest.NewServer(srv.Mux)
	t.Cleanup(ts.Close)
	return ts
}

func postSkill(t *testing.T, ts *httptest.Server, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+"/api/skills", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	_ = resp.Body.Close()
	return resp, out
}

func TestSkillCollectionHandler(t *testing.T) {
	ts := newSkillTestServer(t)

	// 普通技能两件
	r1, m1 := postSkill(t, ts, map[string]any{"name": "s1", "description": "成员一", "instruction": "正文一"})
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("s1 创建应 201: %d", r1.StatusCode)
	}
	r2, m2 := postSkill(t, ts, map[string]any{"name": "s2", "description": "成员二", "instruction": "正文二"})
	if r2.StatusCode != http.StatusCreated {
		t.Fatalf("s2 创建应 201: %d", r2.StatusCode)
	}
	id1, id2 := m1["id"].(string), m2["id"].(string)

	// 集合创建：免 instruction；children 落库
	rc, mc := postSkill(t, ts, map[string]any{"name": "pack", "kind": "collection", "children": []string{id1, id2}})
	if rc.StatusCode != http.StatusCreated {
		t.Fatalf("集合创建应 201: %d %v", rc.StatusCode, mc)
	}
	collID := mc["id"].(string)
	if mc["kind"] != "collection" || len(mc["children"].([]any)) != 2 {
		t.Fatalf("集合往返应保 kind/children: %v", mc)
	}

	// 校验三分支：未知子件/嵌套集合/非法 kind
	rb, _ := postSkill(t, ts, map[string]any{"name": "bad1", "kind": "collection", "children": []string{"ghost"}})
	if rb.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知子件应 400: %d", rb.StatusCode)
	}
	rb, _ = postSkill(t, ts, map[string]any{"name": "bad2", "kind": "collection", "children": []string{collID}})
	if rb.StatusCode != http.StatusBadRequest {
		t.Fatalf("嵌套集合应 400: %d", rb.StatusCode)
	}
	rb, _ = postSkill(t, ts, map[string]any{"name": "bad3", "kind": "weird"})
	if rb.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 kind 应 400: %d", rb.StatusCode)
	}

	// 集合 preview=启用成员展开目录+工具并集
	resp, err := http.Get(ts.URL + "/api/skills/" + collID + "/preview")
	if err != nil {
		t.Fatal(err)
	}
	var pv map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&pv)
	_ = resp.Body.Close()
	blk, _ := pv["instruction_block"].(string)
	if pv["kind"] != "collection" || !bytes.Contains([]byte(blk), []byte("成员一")) || !bytes.Contains([]byte(blk), []byte("成员二")) {
		t.Fatalf("集合预览应展开成员目录: %v", pv)
	}
}

func TestSkillImportCollect(t *testing.T) {
	ts := newSkillTestServer(t)
	zipB := skillImportZipBytes(t) // 复用 REQ-371 夹具：两技能 zip

	// 按包归集：collection_name 非空 → 新建件入集
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("collection_name", "归集包")
	fw, _ := mw.CreateFormFile("file", "pack.zip")
	_, _ = fw.Write(zipB)
	_ = mw.Close()
	resp, err := http.Post(ts.URL+"/api/skills/import", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Created    int             `json:"created"`
		Collection *map[string]any `json:"collection"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rep)
	_ = resp.Body.Close()
	if rep.Created != 2 || rep.Collection == nil {
		t.Fatalf("导入归集应 2 件入集: %+v", rep)
	}

	// 列表核验：集合行 children 指向新建成员
	lr, _ := http.Get(ts.URL + "/api/skills")
	var skills []store.Skill
	_ = json.NewDecoder(lr.Body).Decode(&skills)
	_ = lr.Body.Close()
	var coll *store.Skill
	for i := range skills {
		if skills[i].Kind == "collection" {
			coll = &skills[i]
		}
	}
	if coll == nil || len(coll.Children) != 2 {
		t.Fatalf("归集集合应落库 2 子件: %+v", coll)
	}

	// 重传同名 → 全部跳过、零新建 → 不建集（notes 诚实标注）
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	_ = mw2.WriteField("collection_name", "另一个集合")
	fw2, _ := mw2.CreateFormFile("file", "pack.zip")
	_, _ = fw2.Write(zipB)
	_ = mw2.Close()
	resp2, err := http.Post(ts.URL+"/api/skills/import", mw2.FormDataContentType(), &buf2)
	if err != nil {
		t.Fatal(err)
	}
	var rep2 struct {
		Created    int             `json:"created"`
		Collection *map[string]any `json:"collection"`
		Notes      []struct {
			Skill string   `json:"skill"`
			Notes []string `json:"notes"`
		} `json:"notes"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&rep2)
	_ = resp2.Body.Close()
	if rep2.Created != 0 || rep2.Collection != nil || len(rep2.Notes) == 0 {
		t.Fatalf("零新建不应建集且应诚实标注: %+v", rep2)
	}
}
