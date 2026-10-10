package main

import "testing"

const coreConnectedIntegrationTestDirectory = "ucupa-core/tests/connected"

func TestReviewedCoreConnectedIntegrationTestBoundary(t *testing.T) {
	if !reviewedCoreConnectedIntegrationTestPackage("ucupa-core", coreConnectedIntegrationTestDirectory) ||
		reviewedCoreConnectedIntegrationTestPackage("ucupa-facet", coreConnectedIntegrationTestDirectory) ||
		reviewedCoreConnectedIntegrationTestPackage("ucupa-core", coreConnectedIntegrationTestDirectory+"/nested") {
		t.Fatal("Core connected integration tests must identify exactly one package")
	}
	for _, name := range []string{"fixture_test.go", "local_test.go", "pair_fixture_test.go", "pair_test.go", "nested_test.go", "attachment_test.go"} {
		file := coreConnectedIntegrationTestDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Core connected integration test %s rejected: %s", file, got)
		}
	}
	for _, file := range []string{
		coreConnectedIntegrationTestDirectory + "/extra_test.go",
		coreConnectedIntegrationTestDirectory + "/connected_test.go",
		coreConnectedIntegrationTestDirectory + "/nested/local_test.go",
		coreConnectedIntegrationTestDirectory + "/production.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Core connected integration source accepted: %s", file)
		}
	}

	stdlib := connectedIntegrationTestStdlib()
	for _, file := range []string{
		coreConnectedIntegrationTestDirectory + "/fixture_test.go",
		coreConnectedIntegrationTestDirectory + "/local_test.go",
		coreConnectedIntegrationTestDirectory + "/pair_fixture_test.go",
		coreConnectedIntegrationTestDirectory + "/pair_test.go",
		coreConnectedIntegrationTestDirectory + "/nested_test.go",
		coreConnectedIntegrationTestDirectory + "/attachment_test.go",
	} {
		for _, dependency := range []string{
			"context",
			"errors",
			"fmt",
			"reflect",
			"sync",
			"testing",
			"ucupa/shared/values",
			"ucupa/ucupa-core/instance",
			"ucupa/ucupa-core/contract",
			"ucupa/ucupa-core/contract/connected",
		} {
			if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed Core connected integration dependency %s -> %s: %s", file, dependency, got)
			}
		}
		for _, dependency := range []string{
			"ucupa/ucupa-core/internal/query/connected",
			"ucupa/ucupa-core/internal/query/exchange",
			"ucupa/ucupa-core/internal/element/fact",
			"ucupa/ucupa-core/internal/facet/fact",
			"ucupa/ucupa-core/internal/mount/fact",
			"ucupa/ucupa-core/internal/store",
			"ucupa/ucupa-core/internal/token",
			"ucupa/ucupa-core/transport/grpc",
			"ucupa/ucupa-core/sdk/go",
			"ucupa/ucupa-core/tests/query",
			"go.etcd.io/bbolt",
		} {
			if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden Core connected integration dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
	if got := checkDependency(coreConnectedIntegrationTestDirectory+"/fixture_test.go", "ucupa/ucupa-core/tests/connected", "ucupa", true, stdlib); got == "" {
		t.Fatal("connected integration tests must not become a reusable test fixture package")
	}
	if got := checkDependency(coreConnectedIntegrationTestDirectory+"/fixture_test.go", "ucupa/ucupa-core", "ucupa", false, stdlib); got == "" {
		t.Fatal("production source cannot be placed in Core connected integration test package")
	}
}

func TestReviewedCoreConnectedWorldTestBoundary(t *testing.T) {
	const file = "ucupa-core/internal/query/connected/world_test.go"
	if got := checkSourceLocation(file); got != "" {
		t.Fatalf("reviewed connected world test rejected: %s", got)
	}
	stdlib := connectedIntegrationTestStdlib()
	for _, dependency := range []string{
		"errors",
		"testing",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/contract/connected",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query/exchange",
		"ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed connected world test dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/facet",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-core/tests/connected",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden connected world test dependency accepted: %s", dependency)
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/query/connected/world.go",
		"ucupa-core/internal/query/connected/operation_test.go",
	} {
		if got := checkDependency(file, "ucupa/ucupa-core", "ucupa", false, stdlib); got == "" {
			t.Errorf("connected production source or non-test file may not import Core root: %s", file)
		}
	}
}

func TestReviewedCoreConnectedFilterTestBoundary(t *testing.T) {
	const file = "ucupa-core/internal/query/connected/filter_test.go"
	if got := checkSourceLocation(file); got != "" {
		t.Fatalf("reviewed connected filter test rejected: %s", got)
	}
	stdlib := connectedIntegrationTestStdlib()
	for _, dependency := range []string{
		"errors",
		"testing",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/contract/connected",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query/exchange",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/query",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed connected filter test dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/facet",
		"ucupa/ucupa-core/internal/token/extra",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
	} {
		if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("forbidden connected filter test dependency accepted: %s", dependency)
		}
	}
}

func TestReviewedCoreConnectedReviewTestBoundaries(t *testing.T) {
	stdlib := connectedIntegrationTestStdlib()
	stdlib["sync/atomic"] = true
	stdlib["time"] = true
	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/internal/query/connected/review_fixture_test.go",
			allowed: []string{
				"context", "fmt", "sync", "testing", "time",
				"ucupa/shared/values",
				"ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/contract/connected",
				"ucupa/ucupa-core/internal/commit",
				"ucupa/ucupa-core/internal/element",
				"ucupa/ucupa-core/internal/mount",
				"ucupa/ucupa-core/internal/query",
				"ucupa/ucupa-core/internal/query/exchange",
			},
		},
		{
			file: "ucupa-core/internal/query/connected/review_test.go",
			allowed: []string{
				"context", "errors", "reflect", "sync", "sync/atomic", "testing", "time",
				"ucupa/shared/values",
				"ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/query/exchange",
			},
		},
	}
	for _, tc := range cases {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed connected test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed connected review dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, file := range []string{
		"ucupa-core/internal/query/connected/review_fixture_test.go",
		"ucupa-core/internal/query/connected/review_test.go",
	} {
		for _, dependency := range []string{
			"ucupa/ucupa-core",
			"ucupa/ucupa-core/internal/access",
			"ucupa/ucupa-core/internal/store",
			"ucupa/ucupa-core/internal/facet",
			"ucupa/ucupa-core/internal/query/scan",
			"ucupa/ucupa-core/internal/query/request",
			"ucupa/ucupa-core/internal/token",
			"ucupa/ucupa-core/transport/grpc",
			"ucupa/ucupa-core/sdk/go",
			"go.etcd.io/bbolt",
		} {
			if got := checkDependency(file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden connected review dependency accepted: %s -> %s", file, dependency)
			}
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core/internal/query/connected",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/element",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/query",
	} {
		if got := checkDependency("ucupa-core/internal/query/connected/review_test.go", dependency, "ucupa", true, stdlib); got == "" {
			t.Errorf("review assertion test acquired unreviewed operation dependency: %s", dependency)
		}
	}
	if got := checkSourceLocation("ucupa-core/internal/query/connected/review_extra_test.go"); got == "" {
		t.Fatal("unreviewed connected review test source accepted")
	}
}

func connectedIntegrationTestStdlib() map[string]bool {
	stdlib := testStdlib()
	for _, dependency := range []string{"context", "encoding/binary", "errors", "fmt", "reflect", "sync", "testing"} {
		stdlib[dependency] = true
	}
	return stdlib
}
