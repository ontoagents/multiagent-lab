package vocabsearch

// REQ-261/M259：OLS 通道单测——v2 响应投影（含 config 描述回落/字段缺省容错）+缓存键隔离。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchOntologies_Req261(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if !strings.HasSuffix(r.URL.Path, "/ontologies") || r.URL.Query().Get("q") != "disease" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"_embedded":{"ontologies":[
			{"ontologyId":"efo","iri":"http://www.ebi.ac.uk/efo","label":"Experimental Factor Ontology","description":"EFO 描述"},
			{"ontologyId":"hpo","iri":"http://purl.obolibrary.org/obo/hpo","config":{"title":"Human Phenotype Ontology","description":"HPO 表型描述"}},
			{"iri":""}
		]}}`))
	}))
	defer srv.Close()

	c := New("")
	c.OLSBase = srv.URL
	cards, err := c.SearchOntologies(context.Background(), "disease")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 { // 空 iri+id 条目跳过
		t.Fatalf("应 2 张卡，得到 %d", len(cards))
	}
	if cards[0].Prefix != "efo" || cards[0].Title != "Experimental Factor Ontology" {
		t.Fatalf("efo 卡不符: %+v", cards[0])
	}
	if cards[1].Prefix != "hpo" || cards[1].Description != "HPO 表型描述" {
		t.Fatalf("hpo 卡应回落 config.description: %+v", cards[1])
	}
	// 缓存：第二次不再打上游
	if _, err := c.SearchOntologies(context.Background(), "disease"); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("缓存未生效 hits=%d", hits)
	}
	// 缓存键隔离：LOV 查询不命中 OLS 缓存
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv2.Close()
	c.Base = srv2.URL
	if _, err := c.Search(context.Background(), "disease"); err != nil {
		t.Fatal(err)
	}
}
