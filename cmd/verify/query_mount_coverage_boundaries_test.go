package main

import "testing"

func TestReviewedCoreQueryMountCoverageDependencies(t *testing.T) {
	stdlib := guardOracleStdlib()
	cases := []struct {
		file    string
		allowed []string
	}{
		{
			file: "ucupa-core/tests/query/mount_coverage_fixture_test.go",
			allowed: []string{
				"context", "testing", "time", "ucupa/composition", "ucupa/shared/values",
				"ucupa/ucupa-core/instance", "ucupa/ucupa-core/contract",
			},
		},
		{
			file: "ucupa-core/tests/query/mount_coverage_test.go",
			allowed: []string{
				"context", "encoding/json", "errors", "testing", "ucupa/shared/values", "ucupa/ucupa-core/instance",
			},
		},
	}
	for _, tc := range cases {
		if got := checkSourceLocation(tc.file); got != "" {
			t.Errorf("reviewed mount-coverage test %s rejected: %s", tc.file, got)
		}
		for _, dependency := range tc.allowed {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got != "" {
				t.Errorf("allowed mount-coverage dependency %s -> %s: %s", tc.file, dependency, got)
			}
		}
	}
	for _, tc := range []struct {
		file         string
		dependencies []string
	}{
		{
			file: "ucupa-core/tests/query/mount_coverage_fixture_test.go",
			dependencies: []string{
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/element",
				"ucupa/ucupa-core/internal/query", "ucupa/ucupa-core/internal/mount",
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/transport/grpc",
				"ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
		{
			file: "ucupa-core/tests/query/mount_coverage_test.go",
			dependencies: []string{
				"ucupa/composition", "ucupa/ucupa-core/contract",
				"ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/query",
				"ucupa/ucupa-core/internal/store", "ucupa/ucupa-core/transport/grpc",
				"ucupa/ucupa-core/sdk/go", "go.etcd.io/bbolt",
			},
		},
	} {
		for _, dependency := range tc.dependencies {
			if got := checkDependency(tc.file, dependency, "ucupa", true, stdlib); got == "" {
				t.Errorf("forbidden mount-coverage dependency accepted: %s -> %s", tc.file, dependency)
			}
		}
	}
	for _, file := range []string{
		"ucupa-core/tests/query/mount_coverage_extra_test.go",
		"ucupa-core/tests/query/mount_coverage/fixture_test.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed mount-coverage source accepted: %s", file)
		}
	}
	if got := checkDependency("ucupa-core/tests/query/mount_coverage_extra_test.go", "ucupa/composition", "ucupa", true, stdlib); got == "" {
		t.Fatal("unreviewed mount-coverage dependency accepted")
	}
	if got := checkDependency("ucupa-core/tests/query/mount_coverage_fixture_test.go", "ucupa/composition", "ucupa", false, stdlib); got == "" {
		t.Fatal("production mount-coverage fixture dependency accepted")
	}
}
