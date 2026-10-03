package importer

// REQ-235/M62 导入保真度测试：
// ①Sniff 扩 JSON-LD（.jsonld 扩展名 / .json + @context 内容特征 / 默认分支）；
// ②sidecar 真机解析（SKOS 词表→概念+broader/narrower→parents；JSON-LD；datatype 固定策略降级）——
//   依赖 python3+rdflib，缺失自动 skip（CI 安全；真机验证在开发机全量跑过）。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSniffJSONLD(t *testing.T) {
	cases := []struct {
		name, filename, content, want string
	}{
		{"扩展名", "vocab.jsonld", `{"@context":{},"@graph":[]}`, FormatJSONLD},
		{"json扩展+context", "data.json", `{"@context":{"skos":"http://www.w3.org/2004/02/skos/core#"},"@graph":[{"@id":"ex:a","@type":"skos:Concept"}]}`, FormatJSONLD},
		{"json扩展+spec优先", "spec.json", `{"concepts":[],"relations":[],"instances":[]}`, FormatSpecJSON},
		{"内容兜底", "unknown.txt", `{"@context":"https://www.w3.org/2018/credentials/v1"}`, FormatJSONLD},
	}
	for _, c := range cases {
		got, err := Sniff(c.filename, c.content)
		if err != nil {
			t.Fatalf("%s: Sniff 失败: %v", c.name, err)
		}
		if got != c.want {
			t.Fatalf("%s: 期望 %s，实际 %s", c.name, c.want, got)
		}
	}
}

// sidecar 可用性探测（无 python3/rdflib 环境 skip）。
func sidecarAvailable(t *testing.T) *Sidecar {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 不可用")
	}
	script, err := filepath.Abs("../../../tools/rdf-sidecar/sidecar.py")
	if err != nil || !fileExists(script) {
		t.Skip("sidecar.py 不可达")
	}
	if out, err := exec.Command(py, "-c", "import rdflib").CombinedOutput(); err != nil {
		t.Skipf("rdflib 未安装: %s", strings.TrimSpace(string(out))[:min(80, len(out))])
	}
	return &Sidecar{Python: py, Script: script}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

const skosFixture = `@prefix skos: <http://www.w3.org/2004/02/skos/core#> .
@prefix ex: <http://example.org/vocab#> .
ex:ops a skos:Concept ; skos:prefLabel "运维领域" ; skos:definition "集群运维知识域" .
ex:rollout a skos:Concept ; skos:prefLabel "滚动更新" ; skos:broader ex:ops .
ex:ops skos:narrower ex:hpa .
ex:hpa a skos:Concept ; skos:prefLabel "水平扩缩" .
`

func TestSidecarParsesSKOSVocabulary(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, rep, err := Import(sc, "vocab.ttl", skosFixture)
	if err != nil {
		t.Fatalf("SKOS 导入失败: %v", err)
	}
	if len(sp.Concepts) != 3 {
		t.Fatalf("期望 3 概念，实际 %d", len(sp.Concepts))
	}
	byName := map[string]bool{}
	for _, c := range sp.Concepts {
		byName[c.Name] = true
		if c.Name == "滚动更新" && (len(c.Parents) != 1 || c.Parents[0] != "运维领域") {
			t.Fatalf("skos:broader 应映射 parents=[运维领域]，实际 %v", c.Parents)
		}
		if c.Name == "水平扩缩" && (len(c.Parents) != 1 || c.Parents[0] != "运维领域") {
			t.Fatalf("skos:narrower 反向应映射 parents，实际 %v", c.Parents)
		}
		if c.Name == "运维领域" && c.Definition != "集群运维知识域" {
			t.Fatalf("skos:definition 应映射 definition，实际 %q", c.Definition)
		}
	}
	if !byName["运维领域"] {
		t.Fatalf("prefLabel 未映射为概念名: %v", sp.Concepts)
	}
	if rep.Lossy {
		t.Fatalf("纯 SKOS 词表导入不应标记 lossy: %v", rep.Warnings)
	}
}

const jsonldFixture = `{
  "@context": {"skos": "http://www.w3.org/2004/02/skos/core#", "ex": "http://example.org/j#"},
  "@graph": [
    {"@id": "ex:svc", "@type": "skos:Concept", "skos:prefLabel": "服务网格"},
    {"@id": "ex:mesh", "@type": "skos:Concept", "skos:prefLabel": "Mesh 控制面", "skos:broader": {"@id": "ex:svc"}}
  ]
}`

func TestSidecarParsesJSONLD(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, _, err := Import(sc, "vocab.jsonld", jsonldFixture)
	if err != nil {
		t.Fatalf("JSON-LD 导入失败: %v", err)
	}
	if len(sp.Concepts) != 2 {
		t.Fatalf("期望 2 概念，实际 %d", len(sp.Concepts))
	}
	found := false
	for _, c := range sp.Concepts {
		if c.Name == "Mesh 控制面" {
			found = true
			if len(c.Parents) != 1 || c.Parents[0] != "服务网格" {
				t.Fatalf("JSON-LD broader 应映射 parents，实际 %v", c.Parents)
			}
		}
	}
	if !found {
		t.Fatalf("JSON-LD 概念缺失: %v", sp.Concepts)
	}
}

const datatypeFixture = `@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix ex: <http://example.org/d#> .
ex:Pod a owl:Class ; rdfs:label "Pod" .
ex:replicas a owl:DatatypeProperty ; rdfs:domain ex:Pod ; rdfs:label "副本数" .
ex:pod1 a owl:NamedIndividual, ex:Pod ; rdfs:label "frontend" ; ex:replicas "3" .
`

func TestSidecarDatatypeDowngradeFixedPolicy(t *testing.T) {
	sc := sidecarAvailable(t)
	sp, rep, err := Import(sc, "dt.ttl", datatypeFixture)
	if err != nil {
		t.Fatalf("datatype 导入失败: %v", err)
	}
	if len(sp.Relations) != 0 {
		t.Fatalf("datatype 声明不得入 relations，实际 %v", sp.Relations)
	}
	// REQ-235：datatype 断言落实例 attributes；实例类型已注册（a ex:Pod）故正常保留
	if len(sp.Instances) != 1 || sp.Instances[0].Attributes["replicas"] != "3" {
		t.Fatalf("datatype 断言应落实例 attributes，实际 %+v", sp.Instances)
	}
	// 无类型实例丢弃分支由真机 API 验证覆盖（merge/apply 400 缺口修复）
	_ = rep
	hit := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "固定策略降级") && strings.Contains(w, "副本数") {
			hit = true
		}
	}
	if !hit || !rep.Lossy {
		t.Fatalf("datatype 降级应有固定策略 warning 且 lossy=true: %v", rep.Warnings)
	}
}
