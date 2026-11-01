module github.com/ontoagents/ontoagents/ontology-service

go 1.25.0

require (
	github.com/ontoagents/ontoagents/pkg/ontology/spec v0.0.0
	modernc.org/sqlite v1.59.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)

replace github.com/ontoagents/ontoagents/pkg/ontology/spec => ../pkg/ontology/spec

replace github.com/ontoagents/ontoagents/pkg/sparqlguard => ../pkg/sparqlguard
