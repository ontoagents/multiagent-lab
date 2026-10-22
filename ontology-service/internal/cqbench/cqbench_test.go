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
	// REQ-367 r2 schema：revision 非空 + 双维标注（kind/difficulty）齐备
	write("a.json", `{"ontology":"a","revision":"r2","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R","kind":"relation","difficulty":"easy"}],"note":"n"}`)
	write("b.json", `{"ontology":"b","revision":"r2","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R","kind":"relation","difficulty":"easy"},{"id":"2","cq":"Q2?","gold":["Y"],"type":"E","kind":"inference","difficulty":"hard"}],"note":"n"}`)
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
	// REQ-367：revision 与分层分布透出
	if list[1].Revision != "r2" {
		t.Fatalf("revision 应透出: %+v", list[1])
	}
	if list[1].Kinds["relation"] != 1 || list[1].Kinds["inference"] != 1 {
		t.Fatalf("kinds 分布不符: %+v", list[1].Kinds)
	}
	if list[1].Difficulties["easy"] != 1 || list[1].Difficulties["hard"] != 1 {
		t.Fatalf("difficulties 分布不符: %+v", list[1].Difficulties)
	}
}

// TestLoadDatasetValidation REQ-367 严格 schema 校验：缺 revision/非法枚举/空集拒绝（基准集是
// 产品资产，漂移即缺陷；坏文件经 ListDatasets 跳过=诚实消失不炸清单）。
func TestLoadDatasetValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ok := `{"ontology":"a","revision":"r2","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R","kind":"relation","difficulty":"easy"}],"note":"n"}`
	missingRev := `{"ontology":"a","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R","kind":"relation","difficulty":"easy"}],"note":"n"}`
	badKind := `{"ontology":"a","revision":"r2","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R","kind":"quiz","difficulty":"easy"}],"note":"n"}`
	badDiff := `{"ontology":"a","revision":"r2","cqs":[{"id":"1","cq":"Q?","gold":["X"],"type":"R","kind":"relation","difficulty":"简单"}],"note":"n"}`
	empty := `{"ontology":"a","revision":"r2","cqs":[],"note":"n"}`

	if _, err := LoadDataset(filepath.Join(dir, "x.json")); err == nil || !os.IsNotExist(err) {
		t.Fatalf("文件缺失应报错: %v", err)
	}
	for name, body := range map[string]string{"ok.json": ok, "missing_rev.json": missingRev, "bad_kind.json": badKind, "bad_diff.json": badDiff, "empty.json": empty} {
		write(name, body)
	}
	if _, err := LoadDataset(filepath.Join(dir, "ok.json")); err != nil {
		t.Fatalf("合法 r2 数据集应通过: %v", err)
	}
	for _, name := range []string{"missing_rev.json", "bad_kind.json", "bad_diff.json", "empty.json"} {
		if _, err := LoadDataset(filepath.Join(dir, name)); err == nil {
			t.Fatalf("%s 应被 schema 校验拒绝", name)
		}
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
