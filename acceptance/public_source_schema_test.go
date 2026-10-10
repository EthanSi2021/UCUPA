package acceptance_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestPublicSourceEvidenceSchema(t *testing.T) {
	path, err := filepath.Abs("../ucupa-core/api/read-source-evidence.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.NewCompiler().Compile(path)
	if err != nil {
		t.Fatal(err)
	}
	const stamp = "AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	const id = "00000000-0000-4000-8000-000000000001"
	base := map[string]any{"purpose": stamp, "operation": "query", "sources": []any{
		map[string]any{"owner": id, "boundary": id, "principal": map[string]any{"realm": "realm", "principal": "caller"},
			"cut": stamp, "authorization": stamp, "readMode": "RETAINED", "queryScope": id, "coverage": stamp,
			"parents": []any{map[string]any{"parent": id, "adjacency": stamp}}},
	}}
	if err := schema.Validate(base); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"nextHop", "credential", "basis", "hidden", "coverage", "authorization"} {
		t.Run(field, func(t *testing.T) {
			raw, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			var candidate map[string]any
			if err := json.Unmarshal(raw, &candidate); err != nil {
				t.Fatal(err)
			}
			node := candidate["sources"].([]any)[0].(map[string]any)
			if field == "coverage" {
				delete(node, field)
			} else if field == "authorization" {
				node[field] = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
			} else {
				node[field] = "private"
			}
			if err := schema.Validate(candidate); err == nil {
				t.Fatal("accepted private or incomplete evidence")
			}
		})
	}
}
