package main

import (
	"strings"
	"testing"
)

func TestReviewedCoreStoreLocationAndDependencyEdges(t *testing.T) {
	if !reviewedStorePackage("ucupa-core", "ucupa-core/internal/store") {
		t.Fatal("Core Store package is not registered")
	}
	if reviewedStorePackage("ucupa-facet", "ucupa-facet/internal/store") {
		t.Fatal("Store registration leaked to another owner")
	}

	for _, file := range []string{
		"ucupa-core/internal/store/json.go",
		"ucupa-core/internal/store/scalar.go",
		"ucupa-core/internal/store/format.go",
		"ucupa-core/internal/store/transaction.go",
		"ucupa-core/internal/store/json_test.go",
		"ucupa-core/internal/store/scalar_test.go",
		"ucupa-core/internal/store/transaction_test.go",
	} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("registered Store source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/store/codec/helper.go",
		"ucupa-core/internal/commit/commit.go",
		"ucupa-core/internal/unreviewed/file.go",
		"ucupa-facet/internal/store/file.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unregistered owner source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"bytes", "encoding/binary", "encoding/json", "errors", "fmt", "io", "math", "strconv", "strings",
	} {
		stdlib[dependency] = true
	}
	for _, dependency := range []string{
		"bytes",
		"encoding/binary",
		"encoding/json",
		"errors",
		"fmt",
		"io",
		"math",
		"strconv",
		"strings",
		"ucupa/shared/values",
	} {
		if got := checkDependency("ucupa-core/internal/store/scalar.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("Store production dependency %s rejected: %s", dependency, got)
		}
	}
	for _, tc := range []struct {
		dependency string
		allowed    bool
	}{
		{"ucupa/ucupa-core/internal/store", true},
		{"ucupa/ucupa-core/internal/store/codec", false},
		{"testing", true},
		{"go.etcd.io/bbolt", false},
		{"go.etcd.io/bbolt/internal/common", false},
		{"github.com/dgraph-io/badger/v4", false},
		{"ucupa/ucupa-core", false},
		{"ucupa/ucupa-core/contract", false},
		{"ucupa/ucupa-core/internal/token", false},
		{"ucupa/ucupa-core/internal/commit", false},
		{"ucupa/ucupa-core/transport/grpc", false},
		{"ucupa/ucupa-core/sdk/go", false},
		{"ucupa/ucupa-facet/contract", false},
		{"ucupa/composition", false},
	} {
		got := checkDependency("ucupa-core/internal/store/json_test.go", tc.dependency, "ucupa", true, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("Store test dependency %s: allowed=%v got=%q", tc.dependency, tc.allowed, got)
		}
	}
	for _, tc := range []struct {
		source  string
		allowed bool
	}{
		{"transaction.go", true},
		{"transaction_test.go", true},
		{"json.go", false},
		{"scalar.go", false},
		{"format.go", false},
		{"json_test.go", false},
		{"scalar_test.go", false},
		{"unreviewed.go", false},
	} {
		source := "ucupa-core/internal/store/" + tc.source
		got := checkDependency(source, "go.etcd.io/bbolt", "ucupa", strings.HasSuffix(source, "_test.go"), stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s database access: allowed=%v got=%q", source, tc.allowed, got)
		}
	}
}

func TestCoreStoreConsumersCannotBypassPeripheralOrOwnerBoundaries(t *testing.T) {
	// Dependency direction alone does not approve a source package. In
	// particular commit -> store is a valid direction, but commit's source
	// location remains unregistered and is rejected by the test above.
	stdlib := testStdlib()
	for _, tc := range []struct {
		source, dependency string
		allowed            bool
	}{
		{"ucupa-core/internal/commit/store.go", "ucupa/ucupa-core/internal/store", true},
		{"ucupa-core/internal/token/cut.go", "go.etcd.io/bbolt", false},
		{"ucupa-core/contract/cut.go", "go.etcd.io/bbolt", false},
		{"ucupa-core/internal/commit/commit.go", "ucupa/ucupa-core/internal/store", true},
		{"ucupa-core/transport/grpc/wire.go", "ucupa/ucupa-core/internal/store", false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/store", false},
		{"ucupa-facet/facet.go", "ucupa/ucupa-core/internal/store", false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/store", false},
		{"acceptance/lifecycle_test.go", "ucupa/ucupa-core/internal/store", false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", strings.HasSuffix(tc.source, "_test.go"), stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}
