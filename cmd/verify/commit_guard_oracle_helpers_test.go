package main

import (
	"go/ast"
	"os"
	"testing"
)

func guardOracleStdlib() map[string]bool {
	stdlib := testStdlib()
	for _, dependency := range []string{
		"bytes", "context", "crypto/rand", "encoding/binary", "encoding/json", "errors", "fmt", "math",
		"io/fs", "os", "path/filepath", "reflect", "strings", "sync", "testing",
		"time",
	} {
		stdlib[dependency] = true
	}
	return stdlib
}

func repositoryRootForGuardOracle(t *testing.T) (string, error) {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return repositoryRoot(working)
}

func sameGuardStructFields(fields *ast.FieldList, want [][]string) bool {
	if fields == nil || len(fields.List) != len(want) {
		return false
	}
	for index, field := range fields.List {
		if len(field.Names) != len(want[index])-1 || guardASTType(field.Type) != want[index][len(want[index])-1] {
			return false
		}
		for nameIndex, name := range field.Names {
			if name.Name != want[index][nameIndex] {
				return false
			}
		}
	}
	return true
}

func guardFieldTypes(fields *ast.FieldList) []string {
	if fields == nil {
		return nil
	}
	types := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) > 1 {
			return nil
		}
		types = append(types, guardASTType(field.Type))
	}
	return types
}

func guardASTType(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return "*" + guardASTType(expression.X)
	case *ast.SelectorExpr:
		return guardASTType(expression.X) + "." + expression.Sel.Name
	default:
		return ""
	}
}

func sameStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
