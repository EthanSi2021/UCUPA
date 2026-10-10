package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func corePredecessorDeclarations() map[string]struct{} {
	return map[string]struct{}{
		"AcceptMount":           {},
		"AdmitMount":            {},
		"Anchor":                {},
		"ConfirmElement":        {},
		"DelegationBinding":     {},
		"Discover":              {},
		"Entity":                {},
		"ErrUnsupportedProfile": {},
		// P2 adds its exact local mount-control surface, not predecessor
		// routing, delegation, migration, or alternate mount entry points.
		"ElementScope":                  {},
		"ElementTarget":                 {},
		"MatchesDelegation":             {},
		"MatchesMountAcceptance":        {},
		"MountAcceptance":               {},
		"MountContext":                  {},
		"PermissionAcceptMount":         {},
		"PermissionDelegateMount":       {},
		"PermissionReanchorDestination": {},
		"PermissionReanchorSubject":     {},
		"PermissionServeMount":          {},
		"ReadAnchor":                    {},
		"ReadEntity":                    {},
		"ReanchorAnchor":                {},
		"ReanchorEntity":                {},
		"RouteRegistry":                 {},
		"SpatialIndex":                  {},
		"WorldPose":                     {},
	}
}

// The former root-only test must inspect the actual public definitions after
// migration. Private fact types named Anchor/Entity are not predecessor APIs.
func TestCorePredecessorSurfaceFollowsMovedPublicCode(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := corePredecessorDeclarations()
	for _, relative := range []string{"ucupa-core/instance", "ucupa-core/contract"} {
		for _, problem := range predecessorSourceIssues(filepath.Join(root, relative), true, forbidden,
			[]string{"ucupa.core.reanchor.", "ucupa.core.mount."}) {
			t.Error(problem)
		}
	}
	for _, problem := range predecessorSourceIssues(filepath.Join(root, "composition"), false,
		map[string]struct{}{"RouteRegistry": {}}, nil) {
		t.Error(problem)
	}
	for _, check := range []struct{ directory, prefix string }{
		{"ucupa-core", "federation"}, {"composition", "routes"},
	} {
		files, err := os.ReadDir(filepath.Join(root, check.directory))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range files {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), check.prefix) && strings.HasSuffix(entry.Name(), ".go") {
				t.Errorf("predecessor file remains: %s", filepath.Join(check.directory, entry.Name()))
			}
		}
	}
	obsolete := filepath.Join(root, "acceptance", "concept_v0_test.go")
	if _, err := os.Stat(obsolete); err == nil {
		t.Fatalf("predecessor file remains: %s", obsolete)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func predecessorSourceIssues(directory string, recursive bool, forbidden map[string]struct{}, prefixes []string) []string {
	var problems []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != directory && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		report := func(kind, name string) {
			problems = append(problems, fmt.Sprintf("predecessor %s %s remains in %s", kind, name, path))
		}
		checkDeclaration := func(name string) {
			if _, found := forbidden[name]; found {
				report("declaration", name)
			}
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				checkDeclaration(declaration.Name.Name)
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						checkDeclaration(specification.Name.Name)
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							checkDeclaration(name.Name)
						}
					}
				}
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.Ident:
				if node.Name != "Anchor" && node.Name != "Entity" {
					if _, found := forbidden[node.Name]; found {
						report("identifier", node.Name)
					}
				}
			case *ast.BasicLit:
				if node.Kind != token.STRING {
					break
				}
				value, err := strconv.Unquote(node.Value)
				if err != nil {
					problems = append(problems, fmt.Sprintf("decode string in %s: %v", path, err))
					break
				}
				for _, prefix := range prefixes {
					if strings.HasPrefix(value, prefix) {
						report("source value", value)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	return problems
}

func TestPredecessorSourceScanFindsMovedDefinitionsAndSkipsTestFixtures(t *testing.T) {
	for _, test := range []struct {
		name, file, source   string
		recursive, wantIssue bool
	}{
		{"nested declaration", "query/value.go", "package p; func ReanchorAnchor() {}", true, true},
		{"nested reference", "query/value.go", "package p; var x = legacy.RouteRegistry{}", true, true},
		{"old literal", "permission.go", "package p; const id = \"ucupa.core.mount.old\"", true, true},
		{"old type", "value.go", "package p; type Anchor struct{}", true, true},
		{"legal record arms", "value.go", "package p; type Record struct { Anchor *int; Entity *int }", true, false},
		{"test-only negative fixture", "fixture_test.go", "package p; func ReanchorAnchor() {}", true, false},
		{"explicit direct scope", "child/value.go", "package p; func ReanchorAnchor() {}", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, filepath.FromSlash(test.file))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(test.source), 0600); err != nil {
				t.Fatal(err)
			}
			problems := predecessorSourceIssues(directory, test.recursive, corePredecessorDeclarations(),
				[]string{"ucupa.core.reanchor.", "ucupa.core.mount."})
			if (len(problems) != 0) != test.wantIssue {
				t.Fatalf("issues: %v; want issue %t", problems, test.wantIssue)
			}
		})
	}
}
