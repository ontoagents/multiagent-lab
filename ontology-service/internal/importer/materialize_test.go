package importer

// REQ-481/M281：sidecar materialize 实测（python+owlrl 在场时真跑——owlrl 缺失自动 skip，
// 与对拍安全网同口径）。验证数据级闭包推断在输出、axiomatic 词汇闭包不入输出。

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSidecarMaterialize_Req481(t *testing.T) {
	sc := testSidecar(t)
	input := "<urn:x:A> <http://www.w3.org/2000/01/rdf-schema#subClassOf> <urn:x:B> .\n" +
		"<urn:x:a> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <urn:x:A> .\n"
	ttl, n, err := Materialize(sc, "turtle", input)
	if err != nil {
		if strings.Contains(err.Error(), "owlrl 未安装") {
			t.Skip("owlrl 未安装（物化档为环境增强项）")
		}
		t.Fatalf("materialize: %v", err)
	}
	// 数据推断在：实例分类 a type B（subClassOf 上行）
	if !strings.Contains(ttl, "urn:x:a") || !strings.Contains(ttl, "urn:x:B") {
		t.Fatalf("物化输出应含 a type B 推断:\n%s", ttl)
	}
	// axiomatic 词汇闭包不入输出（rdf:Alt 为 OWL/RDF 公理集标志实体）
	if strings.Contains(ttl, "rdf:Alt") || strings.Contains(ttl, "<http://www.w3.org/2000/01/rdf-schema#Container>") {
		t.Fatalf("物化输出不应含 axiomatic 词汇闭包:\n%.400s", ttl)
	}
	if n <= 0 {
		t.Fatalf("计数应>0，实际 %d", n)
	}
}

func testSidecar(t *testing.T) *Sidecar {
	t.Helper()
	sc := &Sidecar{Python: "python", Script: "../../../tools/rdf-sidecar/sidecar.py"}
	if _, err := os.Stat(sc.Script); err != nil {
		t.Skipf("sidecar.py 不可达: %v", err)
	}
	if _, err := exec.LookPath(sc.Python); err != nil {
		sc.Python = "python3"
		if _, err := exec.LookPath(sc.Python); err != nil {
			t.Skip("系统无 python")
		}
	}
	return sc
}
