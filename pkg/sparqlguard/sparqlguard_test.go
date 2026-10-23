package sparqlguard

// REQ-380/M176：单一实现测试——四消费方既有用例全集（facade 22 例+cqbench/nl2sparql/toolchain 用例）。

import "testing"

func TestVet(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"合法 SELECT", "SELECT ?s ?p ?o WHERE { ?s ?p ?o }", false},
		{"PREFIX 前导", "PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\nSELECT ?c WHERE { ?c rdfs:label ?l }", false},
		{"BASE+PREFIX 前导", "BASE <http://x.example/>\nPREFIX a: <urn:a:>\nSELECT ?s WHERE { ?s a:t ?o }", false},
		{"BASE+PREFIX 混序", "BASE <http://b.org/>\nPREFIX ex: <http://e.org#>\nSELECT * WHERE { ?s ?p ?o }", false},
		{"小写 select", "select ?s where { ?s ?p ?o }", false},
		{"注释含变更关键字", "# delete all data\nSELECT ?s WHERE { ?s ?p ?o }", false},
		{"注释内关键字不误伤2", "SELECT ?s # DROP noise\nWHERE { ?s ?p ?o }", false},
		{"字符串字面量含关键字", `SELECT ?s WHERE { ?s ?p ?o . FILTER(CONTAINS(?o, "delete everything")) }`, false},
		{"字面量含 DELETE 不误伤", "SELECT ?l WHERE { ?s ?p ?l FILTER(?l = 'please DELETE me') }", false},
		{"IRI 片段含关键字", "SELECT ?s WHERE { <http://example.org#deleteRule> ?p ?o }", false},
		{"IRI 内关键字不误伤", "SELECT ?s WHERE { ?s ?p <http://e/delete> }", false},
		{"LIMIT/OFFSET 合法", "SELECT ?s WHERE { ?s ?p ?o } ORDER BY ?s LIMIT 10 OFFSET 5", false},
		{"聚合 SELECT", "SELECT ?c (COUNT(?i) AS ?n) WHERE { ?i a ?c } GROUP BY ?c", false},
		{"UPDATE 拒绝", "UPDATE { ?s ?p ?o } WHERE { ?s ?p ?o }", true},
		{"INSERT DATA 拒绝", "INSERT DATA { <urn:x:a> <urn:x:b> <urn:x:c> }", true},
		{"前缀后 INSERT 拒绝", "PREFIX ex: <http://e.org#>\nINSERT DATA { ex:a ex:b ex:c }", true},
		{"DELETE 拒绝", "DELETE WHERE { ?s ?p ?o }", true},
		{"裸词 DELETE 拒绝", "SELECT ?s WHERE { ?s ?p ?o } DELETE { ?s ?p ?o }", true},
		{"LOAD 拒绝", "LOAD <http://example.org/data.ttl>", true},
		{"CLEAR 拒绝", "CLEAR GRAPH <urn:g>", true},
		{"CREATE 拒绝", "CREATE GRAPH <urn:g>", true},
		{"DROP ALL 拒绝", "DROP ALL", true},
		{"ASK 合法（REQ-366/T1 统一口径）", "ASK { ?s ?p ?o }", false},
		{"PREFIX+ASK 合法", "PREFIX e: <urn:e:>\nask { ?s e:p ?o }", false},
		{"CONSTRUCT 拒绝", "CONSTRUCT { ?s ?p ?o } WHERE { ?s ?p ?o }", true},
		{"无前导声明裸词拒绝", "?s ?p ?o", true},
		{"空查询拒绝", "   ", true},
		{"仅 PREFIX 无 SELECT 拒绝", "PREFIX a: <urn:a:>", true},
		{"大小写混合 INSERT 拒绝", "select ?s where { ?s ?p ?o } ; insert { ?s ?p 'x' } where { ?s ?p ?o }", true},
		{"SELECT 夹带 DROP 拒绝", "SELECT ?s WHERE { ?s ?p ?o } ; DROP DEFAULT", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Vet(tc.query)
			if tc.wantErr && err == nil {
				t.Fatalf("期望拒绝，实际通过: %s", tc.query)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望通过，实际拒绝: %s，错误: %v", tc.query, err)
			}
		})
	}
}

func TestFirstQueryKind(t *testing.T) {
	if got := FirstQueryKind("PREFIX a: <urn:a:>\nASK { ?s ?p ?o }"); got != "ASK" {
		t.Fatalf("FirstQueryKind = %q, want ASK", got)
	}
	if got := FirstQueryKind("   "); got != "" {
		t.Fatalf("空查询应空串: %q", got)
	}
}
