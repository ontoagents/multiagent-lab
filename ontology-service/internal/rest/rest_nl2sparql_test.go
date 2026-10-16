package rest

// REQ-333/M133：NL2SPARQL 只读白名单（facade ValidateReadonlySelect 同口径本地实现）+
// 回复提取器。与 runtime-manager facade 闸门双闸同语义：此处生成预检、执行侧代理权威兜底。

import "testing"

func TestVetReadonlySelect(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"普通 SELECT", "PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\nSELECT ?s WHERE { ?s ?p ?o } LIMIT 10", false},
		{"小写 select", "select ?s where { ?s ?p ?o }", false},
		{"DELETE 拒绝", "DELETE WHERE { ?s ?p ?o }", true},
		{"INSERT 拒绝", "PREFIX ex: <http://e/>\nINSERT DATA { ex:a ex:b ex:c }", true},
		{"SELECT 夹带 UPDATE 关键字拒绝", "SELECT ?s WHERE { ?s ?p ?o } ; DROP DEFAULT", true},
		{"首词非 SELECT 拒绝", "CONSTRUCT { ?s ?p ?o } WHERE { ?s ?p ?o }", true},
		{"字符串字面量关键字不误伤", "SELECT ?s WHERE { ?s rdfs:label \"delete me\" }", false},
		{"IRI 内关键字不误伤", "SELECT ?s WHERE { ?s ?p <http://e/delete> }", false},
		{"注释内关键字不误伤", "SELECT ?s # DROP noise\nWHERE { ?s ?p ?o }", false},
		{"空查询拒绝", "   ", true},
		{"仅 PREFIX 拒绝", "PREFIX ex: <http://e/>", true},
	}
	for _, c := range cases {
		err := vetReadonlySelect(c.query)
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: 期望错误=%v 得到 %v", c.name, c.wantErr, err)
		}
	}
}

func TestExtractSparqlReply(t *testing.T) {
	cases := []struct{ name, reply, want string }{
		{"纯查询", "SELECT ?s WHERE { ?s ?p ?o }", "SELECT ?s WHERE { ?s ?p ?o }"},
		{"sparql 围栏", "好的：\n```sparql\nSELECT ?s WHERE { ?s ?p ?o }\n```\n以上。", "SELECT ?s WHERE { ?s ?p ?o }"},
		{"裸围栏", "```\nSELECT ?s WHERE { ?s ?p ?o }\n```", "SELECT ?s WHERE { ?s ?p ?o }"},
	}
	for _, c := range cases {
		if got := extractSparqlReply(c.reply); got != c.want {
			t.Fatalf("%s: 得到 %q 期望 %q", c.name, got, c.want)
		}
	}
}
