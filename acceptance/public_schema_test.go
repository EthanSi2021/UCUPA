package acceptance_test

import (
	"encoding/json"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var publicSchemas sync.Map

// This independent Draft 2020-12 consumer has no access to owner internals.
// The dependency is test-only; it checks public representations only.
func assertPublicSchema(t *testing.T, owner, name string, value any) {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", owner, "api", name+".schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema *jsonschema.Schema
	if cached, ok := publicSchemas.Load(path); ok {
		schema = cached.(*jsonschema.Schema)
	} else {
		compiler := jsonschema.NewCompiler()
		schema, err = compiler.Compile(path)
		if err != nil {
			t.Fatal(err)
		}
		publicSchemas.Store(path, schema)
	}
	if err = schema.Validate(value); err != nil {
		t.Fatalf("%s %s schema: %v", owner, name, err)
	}
}

func TestP5PublicSchemasCompileAndRejectPrivateOrMalformedFields(t *testing.T) {
	for _, owner := range []string{"ucupa-core", "ucupa-facet"} {
		for _, name := range []string{"request", "response", "types"} {
			path, _ := filepath.Abs(filepath.Join("..", owner, "api", name+".schema.json"))
			schema, err := jsonschema.NewCompiler().Compile(path)
			if err != nil {
				t.Fatal(err)
			}
			if name == "request" {
				for _, raw := range []string{`{"op":"enter","Principal":"self"}`, `{"op":"poll","cursor":{},"limit":0}`, `{"op":"get","access":{}}`} {
					var value any
					if err = json.Unmarshal([]byte(raw), &value); err != nil {
						t.Fatal(err)
					}
					if err = schema.Validate(value); err == nil {
						t.Fatal("schema accepted", raw)
					}
				}
			}
		}
	}
}

func TestP5LanguageNeutralConformanceCorpus(t *testing.T) {
	for _, owner := range []string{"ucupa-core", "ucupa-facet"} {
		raw, err := os.ReadFile(filepath.Join("..", owner, "api", "conformance.json"))
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Name   string
			Schema string
			Value  any
			Valid  bool
		}
		if err = json.Unmarshal(raw, &cases); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			t.Run(owner+"/"+tc.Name, func(t *testing.T) {
				path, _ := filepath.Abs(filepath.Join("..", owner, "api", tc.Schema+".schema.json"))
				schema, err := jsonschema.NewCompiler().Compile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err = schema.Validate(tc.Value); (err == nil) != tc.Valid {
					t.Fatalf("expected valid=%v: %v", tc.Valid, err)
				}
			})
		}
	}
}
