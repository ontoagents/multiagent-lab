package cqbench

// REQ-346/M146：cqbench 单测——清单加载/白名单/URN 对齐（LLM 运行器走 build tag eval 真机）。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDatasets(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.json", `{"ontology":"a","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R"}],"note":"n"}`)
	write("b.json", `{"ontology":"b","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R"},{"id":"2","cq":"Q2?","gold":["Y"],"type":"E"}]}`)
	write("readme.md", "非 json 应跳过")
	write("broken.json", `{invalid`)

	list, err := ListDatasets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("期望 2 份数据集得到 %d: %+v", len(list), list)
	}
	// 字典序
	if list[0].Name != "a" || list[1].Name != "b" {
		t.Fatalf("清单序不符: %+v", list)
	}
	if list[1].Count != 2 {
		t.Fatalf("count 不符: %+v", list[1])
	}
}

func TestListDatasetsEmpty(t *testing.T) {
	list, err := ListDatasets(t.TempDir())
	if err != nil || len(list) != 0 {
		t.Fatalf("空目录应空表无错: %v %+v", err, list)
	}
	// 目录不存在=诚实空表（客户端包可不含 seeds）
	list, err = ListDatasets(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(list) != 0 {
		t.Fatalf("缺失目录应空表无错: %v %+v", err, list)
	}
}

func TestVetReadonlySelect(t *testing.T) {
	if err := VetReadonlySelect("SELECT ?s WHERE { ?s ?p ?o }"); err != nil {
		t.Fatalf("合法 SELECT 应过: %v", err)
	}
	if err := VetReadonlySelect("DELETE WHERE { ?s ?p ?o }"); err == nil {
		t.Fatal("DELETE 应拒")
	}
	if err := VetReadonlySelect("   "); err == nil {
		t.Fatal("空查询应拒")
	}
}

func TestSanitizeURN(t *testing.T) {
	if got := SanitizeURN("故障原因"); got != "故障原因" {
		t.Fatalf("中文应保留: %q", got)
	}
	if got := SanitizeURN("has part"); got != "has_part" {
		t.Fatalf("空格应转下划线: %q", got)
	}
}
