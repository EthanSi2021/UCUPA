package instance_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

func TestCorePublicMethodSurfaceIsClosed(t *testing.T) {
	want := []string{
		"Access",
		"AuthorityRef",
		"Close",
		"Connected",
		"CurrentRealmContact",
		"Elements",
		"Facets",
		"Federation",
		"Mounts",
		"Queries",
		"RealmContact",
		"Transfers",
		"Writer",
	}
	coreType := reflect.TypeOf((*core.Core)(nil))
	got := make([]string, 0, coreType.NumMethod())
	for index := 0; index < coreType.NumMethod(); index++ {
		got = append(got, coreType.Method(index).Name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Core public method surface: got %v, want %v", got, want)
	}
}

func TestCoreInstanceHasNoExportedState(t *testing.T) {
	typ := reflect.TypeOf(core.Core{})
	if typ.Kind() != reflect.Struct {
		t.Fatalf("opaque Core is not a struct: %v", typ.Kind())
	}
	for index := 0; index < typ.NumField(); index++ {
		if field := typ.Field(index); field.IsExported() {
			t.Fatalf("opaque Core exports field %s", field.Name)
		}
	}
}

func TestReadConfigIsExplicitOnEveryConstructor(t *testing.T) {
	connections := reflect.TypeOf((*corecontract.RealmConnections)(nil)).Elem()
	initial := reflect.TypeOf(corecontract.RealmInitialization{})
	config := reflect.TypeOf(corecontract.Config{})
	results := []reflect.Type{reflect.TypeOf((*core.Core)(nil)), reflect.TypeOf((*error)(nil)).Elem()}
	tests := []struct {
		name        string
		constructor any
		arguments   []reflect.Type
	}{
		{"New", core.New, []reflect.Type{connections, initial, config}},
		{"CreateDurable", core.CreateDurable, []reflect.Type{connections, initial, reflect.TypeOf(""), config}},
		{"OpenDurable", core.OpenDurable, []reflect.Type{connections, reflect.TypeOf(""), config}},
	}
	for _, test := range tests {
		got := reflect.TypeOf(test.constructor)
		want := reflect.FuncOf(test.arguments, results, false)
		if got != want || got.IsVariadic() {
			t.Errorf("%s: got %v variadic=%v, want exact non-variadic %v", test.name, got, got.IsVariadic(), want)
		}
	}
}

func TestGreenfieldCoreEntireExportedDeclarationSurface(t *testing.T) {
	want := []string{
		"func CreateDurable",
		"func New",
		"func OpenDurable",
		"method Core.AuthorityRef",
		"method Core.Connected",
		"method Core.CurrentRealmContact",
		"method Core.Elements",
		"method Core.Close",
		"method Core.Access",
		"method Core.Queries",
		"method Core.RealmContact",
		"type Core",
	}
	want = append(want, "method Core.Mounts", "method Core.Facets", "method Core.Federation", "method Core.Writer", "method Core.Transfers")
	want = append(want, "func OpenRealmMaintenance", "type RealmMaintenance",
		"method RealmMaintenance.Inspect", "method RealmMaintenance.Update", "method RealmMaintenance.Recover", "method RealmMaintenance.Close")
	sort.Strings(want)

	coreDir, err := filepath.Abs(filepath.Join("..", "..", "instance"))
	if err != nil {
		t.Fatalf("locate Core source directory: %v", err)
	}
	got := exportedDeclarations(t, coreDir)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Core exported declaration surface:\n got %v\nwant %v", got, want)
	}
}

func exportedDeclarations(t *testing.T, directory string, selectedFiles ...string) []string {
	t.Helper()
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var declarations []string
	selected := map[string]bool{}
	for _, file := range selectedFiles {
		selected[file] = true
	}
	for _, entry := range files {
		if len(selected) != 0 && !selected[entry.Name()] {
			continue
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range parsed.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if !value.Name.IsExported() {
					continue
				}
				if value.Recv == nil {
					declarations = append(declarations, "func "+value.Name.Name)
					continue
				}
				receiver, ok := exportedReceiverName(value.Recv.List[0].Type)
				if !ok {
					if receiver != "" {
						// Exported methods on an unexported adapter type are not
						// independently exported package declarations. Its public
						// interface is checked separately.
						continue
					}
					t.Fatalf("exported method %s has unsupported receiver in %s", value.Name.Name, path)
				}
				declarations = append(declarations, "method "+receiver+"."+value.Name.Name)
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					switch item := specification.(type) {
					case *ast.TypeSpec:
						if item.Name.IsExported() {
							declarations = append(declarations, "type "+item.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range item.Names {
							if name.IsExported() {
								declarations = append(declarations, strings.ToLower(value.Tok.String())+" "+name.Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(declarations)
	return declarations
}

func exportedReceiverName(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name, value.IsExported()
	case *ast.StarExpr:
		return exportedReceiverName(value.X)
	default:
		return "", false
	}
}
