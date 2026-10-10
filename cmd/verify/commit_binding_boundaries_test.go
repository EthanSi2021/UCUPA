package main

import "testing"

func TestReviewedCoreCommitBindingBoundary(t *testing.T) {
	const dir = "ucupa-core/internal/commit/binding"
	if !reviewedCommitBindingPackage("ucupa-core", dir) || reviewedCommitBindingPackage("ucupa-core", dir+"/child") || reviewedCommitBindingPackage("ucupa-facet", dir) {
		t.Fatal("commit binding registration must identify exactly one Core package")
	}
	if got := checkSourceLocation(dir + "/binding.go"); got != "" {
		t.Fatal(got)
	}
	for _, file := range []string{
		dir + "/child/binding.go",
		"ucupa-core/internal/commit/commit.go",
		"ucupa-facet/internal/commit/binding/binding.go",
	} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed commit binding source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"fmt",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/access/value",
	} {
		if got := checkDependency(dir+"/binding.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed production edge %s: %s", dependency, got)
		}
	}
	if got := checkDependency(dir+"/binding_test.go", "ucupa/"+dir, "ucupa", true, stdlib); got != "" {
		t.Fatalf("test self-dependency rejected: %s", got)
	}
	if got := checkDependency(dir+"/binding.go", "ucupa/"+dir, "ucupa", false, stdlib); got == "" {
		t.Fatal("production self-dependency accepted")
	}

	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/facet",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-realm",
		"ucupa/composition",
		"ucupa/acceptance",
		"go.etcd.io/bbolt",
	} {
		for _, test := range []bool{false, true} {
			if got := checkDependency(dir+"/binding.go", dependency, "ucupa", test, stdlib); got == "" {
				t.Errorf("forbidden binding edge accepted: %s test=%v", dependency, test)
			}
		}
	}

	for _, tc := range []struct {
		file, dependency string
		allowed          bool
		test             bool
	}{
		{"ucupa-core/convergence.go", "ucupa/" + dir, true, false},
		{"ucupa-core/public_mutation.go", "ucupa/" + dir, true, false},
		{"ucupa-core/contract/record.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/access/value/object.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/element/fact/state.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/token/revision.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/query/request/canonical.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/query/scan/candidate.go", "ucupa/" + dir, false, false},
		{dir + "/binding_test.go", "ucupa/" + dir, true, true},
	} {
		got := checkDependency(tc.file, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v test=%v got=%q", tc.file, tc.dependency, tc.allowed, tc.test, got)
		}
	}
}

func TestReviewedCoreAccessValueBoundary(t *testing.T) {
	const dir = "ucupa-core/internal/access/value"
	if !reviewedAccessValuePackage("ucupa-core", dir) || reviewedAccessValuePackage("ucupa-core", dir+"/child") || reviewedAccessValuePackage("ucupa-facet", dir) {
		t.Fatal("access value registration must identify exactly one Core package")
	}
	for _, file := range []string{"object.go", "spatial.go", "record.go", "value_test.go", "record_test.go"} {
		if got := checkSourceLocation(dir + "/" + file); got != "" {
			t.Fatalf("reviewed value file %s: %s", file, got)
		}
	}
	for _, file := range []string{
		dir + "/registry.go",
		dir + "/extra_test.go",
		dir + "/Record.go",
		dir + "/child/object.go",
		"ucupa-core/internal/access/access.go",
		"ucupa-facet/internal/access/value/object.go",
	} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed access value source accepted: %s", file)
		}
	}

	stdlib := testStdlib()
	for _, dependency := range []string{
		"fmt",
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/geometry",
	} {
		if got := checkDependency(dir+"/object.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed access value edge %s: %s", dependency, got)
		}
	}
	if got := checkDependency(dir+"/value_test.go", "ucupa/"+dir, "ucupa", true, stdlib); got != "" {
		t.Fatalf("access value test self-dependency rejected: %s", got)
	}
	if got := checkDependency(dir+"/object.go", "ucupa/"+dir, "ucupa", false, stdlib); got == "" {
		t.Fatal("access value production self-dependency accepted")
	}

	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/instance",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/commit/binding",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/token",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"ucupa/ucupa-realm",
		"ucupa/composition",
		"go.etcd.io/bbolt",
	} {
		for _, test := range []bool{false, true} {
			if got := checkDependency(dir+"/object.go", dependency, "ucupa", test, stdlib); got == "" {
				t.Errorf("forbidden access value edge accepted: %s test=%v", dependency, test)
			}
		}
	}

	for _, tc := range []struct {
		file, dependency string
		allowed          bool
		test             bool
	}{
		{"ucupa-core/public_entry.go", "ucupa/" + dir, true, false},
		{"ucupa-core/public_observe.go", "ucupa/" + dir, true, false},
		{"ucupa-core/contract/record.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/element/fact/state.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/token/revision.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/query/request/canonical.go", "ucupa/" + dir, false, false},
		{"ucupa-core/internal/query/scan/candidate.go", "ucupa/" + dir, false, false},
		{dir + "/value_test.go", "ucupa/" + dir, true, true},
		{dir + "/record.go", "ucupa/ucupa-core/internal/token", true, false},
		{dir + "/record_test.go", "ucupa/ucupa-core/internal/token", true, true},
		{dir + "/record_test.go", "ucupa/ucupa-core/internal/token", false, false},
		{dir + "/spatial.go", "ucupa/ucupa-core/internal/token", false, false},
		{dir + "/value_test.go", "ucupa/ucupa-core/internal/token", false, true},
		{dir + "/record.go", "ucupa/ucupa-core/internal/commit", false, false},
		{dir + "/record_test.go", "ucupa/ucupa-core/internal/commit", false, true},
		{dir + "/record.go", "ucupa/ucupa-core/internal/store", false, false},
		{dir + "/record_test.go", "ucupa/ucupa-core/internal/access", false, true},
	} {
		got := checkDependency(tc.file, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v test=%v got=%q", tc.file, tc.dependency, tc.allowed, tc.test, got)
		}
	}
}

func TestReviewedCommitBindingsAreNotReachableFromPeripherals(t *testing.T) {
	stdlib := testStdlib()
	for _, tc := range []struct {
		source, target string
		test           bool
	}{
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-core/internal/commit/binding", false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/commit/binding", false},
		{"ucupa-facet/transport/grpc/server.go", "ucupa/ucupa-core/internal/commit/binding", false},
		{"ucupa-facet/facet.go", "ucupa/ucupa-core/internal/commit/binding", false},
		{"ucupa-realm/realm.go", "ucupa/ucupa-core/internal/commit/binding", false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/commit/binding", false},
		{"acceptance/lifecycle_test.go", "ucupa/ucupa-core/internal/commit/binding", true},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-core/internal/access/value", false},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core/internal/access/value", false},
		{"ucupa-facet/transport/grpc/server.go", "ucupa/ucupa-core/internal/access/value", false},
		{"ucupa-realm/realm.go", "ucupa/ucupa-core/internal/access/value", false},
		{"composition/ports.go", "ucupa/ucupa-core/internal/access/value", false},
	} {
		if got := checkDependency(tc.source, tc.target, "ucupa", tc.test, stdlib); got == "" {
			t.Errorf("peripheral or external edge accepted: %s -> %s", tc.source, tc.target)
		}
	}
}
