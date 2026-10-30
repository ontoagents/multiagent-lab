module github.com/ontoagents/ontoagents/ontology-service

go 1.25.0

require (
	github.com/ontoagents/ontoagents/pkg/ontology/spec v0.0.0
	modernc.org/sqlite v1.59.0
)

replace github.com/ontoagents/ontoagents/pkg/ontology/spec => ../pkg/ontology/spec

replace github.com/ontoagents/ontoagents/pkg/sparqlguard => ../pkg/sparqlguard
