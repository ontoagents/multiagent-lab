package main

// REQ-438/M385 ontocli 单测：httptest 桩直调——命令真执行断言（HTTP 方法/路径/体+人读输出）。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// stubServer 覆盖 CLI 全部触达端点的最小桩（记录最近请求供断言）。
type stubServer struct {
	t          *testing.T
	lastMethod string
	lastPath   string
	lastBody   map[string]any
}

func (s *stubServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/ontologies", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"onto_x","name":"冒烟"}`))
	})
	mux.HandleFunc("GET /api/ontologies", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		_, _ = w.Write([]byte(`[{"id":"onto_x","name":"冒烟","version":2,"n_concepts":3}]`))
	})
	mux.HandleFunc("PUT /api/ontologies/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		_, _ = w.Write([]byte(`{"id":"` + r.PathValue("id") + `"}`))
	})
	mux.HandleFunc("POST /api/ontologies/{id}/elements/{kind}", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("POST /api/ontologies/{id}/elements/{kind}/{name}/rename", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("GET /api/ontologies/{id}/spec", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		_, _ = w.Write([]byte(`{"concepts":[{"name":"订单","parents":["单据"]}],"relations":[]}`))
	})
	mux.HandleFunc("GET /api/ontologies/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		_, _ = w.Write([]byte("# 概念词典\n"))
	})
	mux.HandleFunc("POST /api/ontology-sparql", func(w http.ResponseWriter, r *http.Request) {
		s.rec(r)
		_, _ = w.Write([]byte(`{"bindings":[{"s":"a"}],"count":1}`))
	})
	return mux
}

func (s *stubServer) rec(r *http.Request) {
	s.lastMethod = r.Method
	// Path=解码形态（中文断言用）；query 保留 RequestURI 形态
	s.lastPath = r.URL.Path
	if r.URL.RawQuery != "" {
		s.lastPath += "?" + r.URL.RawQuery
	}
	s.lastBody = nil
	if r.Body != nil {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		s.lastBody = m
	}
}

// runCmd 执行命令并捕获 stdout（emit/printOK 走 fmt 标准流，非 cobra out）。
func runCmd(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	root.SetArgs(args)
	err := root.Execute()
	w.Close()
	os.Stdout = old
	bts, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("执行 %v 失败: %v", args, err)
	}
	return string(bts)
}

func TestAssetCreate(t *testing.T) {
	st := &stubServer{t: t}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()
	ontoURL = srv.URL
	out := runCmd(t, assetCmd(), "create", "冒烟", "--desc", "测试")
	if !strings.Contains(out, "已创建 冒烟") || !strings.Contains(out, "onto_x") {
		t.Fatalf("create 输出不符: %s", out)
	}
	if st.lastMethod != "POST" || st.lastPath != "/api/ontologies" || st.lastBody["name"] != "冒烟" || st.lastBody["description"] != "测试" {
		t.Fatalf("create 请求不符: %s %s %v", st.lastMethod, st.lastPath, st.lastBody)
	}
}

func TestConceptUpsertAndRename(t *testing.T) {
	st := &stubServer{t: t}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()
	ontoURL = srv.URL
	up := conceptCmd()
	up.Flags().String("label", "", "")
	up.Flags().String("def", "", "")
	up.Flags().String("parents", "", "")
	up.Flags().String("synonyms", "", "")
	runCmd(t, up, "upsert", "onto_x", "订单", "--label", "Order", "--parents", "单据,凭证", "--synonyms", "采购单")
	if st.lastPath != "/api/ontologies/onto_x/elements/concept" {
		t.Fatalf("upsert 路径不符: %s", st.lastPath)
	}
	if ps, ok := st.lastBody["parents"].([]any); !ok || len(ps) != 2 || ps[0] != "单据" {
		t.Fatalf("parents 解析不符: %v", st.lastBody["parents"])
	}
	if syn, ok := st.lastBody["synonyms"].([]any); !ok || len(syn) != 1 {
		t.Fatalf("synonyms 解析不符: %v", st.lastBody["synonyms"])
	}
	runCmd(t, up, "rename", "onto_x", "订单", "采购订单")
	if st.lastMethod != "POST" || !strings.HasSuffix(st.lastPath, "/elements/concept/订单/rename") || st.lastBody["to"] != "采购订单" {
		t.Fatalf("rename 请求不符: %s %s %v", st.lastMethod, st.lastPath, st.lastBody)
	}
}

func TestSparqlAndExport(t *testing.T) {
	st := &stubServer{t: t}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()
	rtURL = srv.URL
	ontoURL = srv.URL
	out := runCmd(t, sparqlCmd(), "onto_x", "SELECT ?s WHERE { ?s ?p ?o }")
	if !strings.Contains(out, "1 行") {
		t.Fatalf("sparql 输出不符: %s", out)
	}
	if st.lastPath != "/api/ontology-sparql" || st.lastBody["ontology_id"] != "onto_x" {
		t.Fatalf("sparql 请求不符: %s %v", st.lastPath, st.lastBody)
	}
	out = runCmd(t, exportCmd(), "onto_x", "--format", "dict_md")
	if !strings.Contains(out, "# 概念词典") {
		t.Fatalf("export 输出不符: %s", out)
	}
	if !strings.Contains(st.lastPath, "format=dict_md") {
		t.Fatalf("export 路径不符: %s", st.lastPath)
	}
	if st.lastPath[0] != '/' {
		t.Fatalf("path 应以 / 起: %s", st.lastPath)
	}
	// 非法 format 拒绝
	bad := exportCmd()
	bad.SetOut(&strings.Builder{})
	bad.SetErr(&strings.Builder{})
	bad.SetArgs([]string{"onto_x", "--format", "exe"})
	if err := bad.Execute(); err == nil {
		t.Fatal("非法 format 应报错")
	}
}

func TestJSONOutput(t *testing.T) {
	st := &stubServer{t: t}
	srv := httptest.NewServer(st.handler())
	defer srv.Close()
	ontoURL = srv.URL
	jsonOut = true
	defer func() { jsonOut = false }()
	root := &cobra.Command{Use: "ontocli"}
	root.PersistentFlags().StringVar(&ontoURL, "onto", ontoURL, "")
	root.PersistentFlags().BoolVar(&jsonOut, "json", true, "")
	root.AddCommand(assetCmd())
	out := runCmd(t, root, "--json", "asset", "list")
	if !strings.Contains(out, `"n_concepts": 3`) {
		t.Fatalf("--json 输出应缩进机读: %s", out)
	}
}
