module github.com/ontoagents/ontoagents/runtime-manager

go 1.25.0

require (
	github.com/mark3labs/mcp-go v0.43.0
	github.com/ontoagents/ontoagents/pkg/ontology/spec v0.0.0
	github.com/ontoagents/ontoagents/pkg/sparqlguard v0.0.0
)

replace github.com/ontoagents/ontoagents/pkg/ontology/spec => ../pkg/ontology/spec

replace github.com/ontoagents/ontoagents/pkg/sparqlguard => ../pkg/sparqlguard
