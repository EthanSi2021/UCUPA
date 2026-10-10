package main

import (
	"strings"
	"testing"
)

func TestReviewedOpaqueContractDependencyEdges(t *testing.T) {
	for _, owner := range []string{"ucupa-core", "ucupa-facet"} {
		for _, tc := range []struct {
			source, dependency string
			allowed            bool
		}{
			{owner + "/contract/cut.go", "ucupa/" + owner + "/internal/token", true},
			{owner + "/contract/cut_test.go", "ucupa/" + owner + "/contract", true},
			{owner + "/contract/cut.go", "ucupa/" + owner + "/contract", false},
			{owner + "/contract/cut.go", "ucupa/shared/values", true},
			{owner + "/contract/cut.go", "ucupa/" + owner, false},
			{owner + "/contract/cut.go", "ucupa/" + owner + "/instance", false},
			{owner + "/contract/cut.go", "ucupa/" + owner + "/internal/commit", false},
			{owner + "/contract/cut.go", "go.etcd.io/bbolt", false},
			{owner + "/internal/token/cut.go", "ucupa/" + owner + "/contract", false},
			{owner + "/internal/token/cut.go", "ucupa/" + owner, false},
			{owner + "/internal/token/cut_test.go", "ucupa/" + owner, false},
			{owner + "/internal/token/cut_test.go", "ucupa/" + owner + "/internal/token", true},
			{owner + "/internal/token/cut.go", "ucupa/" + owner + "/internal/token", false},
			{owner + "/internal/token/cut.go", "ucupa/shared/values", true},
			{owner + "/internal/token/cut.go", "fmt", true},
			{owner + "/internal/token/cut.go", "go.etcd.io/bbolt", false},
			{owner + "/transport/grpc/wire.go", "ucupa/" + owner + "/contract", true},
			{owner + "/transport/grpc/wire.go", "ucupa/" + owner + "/internal/token", false},
			{owner + "/sdk/go/client.go", "ucupa/" + owner + "/contract", true},
			{owner + "/sdk/go/client.go", "ucupa/" + owner + "/instance", false},
			{owner + "/sdk/go/client.go", "ucupa/" + owner + "/internal/token", false},
			{"composition/ports.go", "ucupa/" + owner + "/contract", true},
			{"composition/ports.go", "ucupa/" + owner + "/internal/token", false},
			{"acceptance/opaque_test.go", "ucupa/" + owner + "/internal/token", false},
			{owner + "/root.go", "ucupa/" + owner + "/internal/unreviewed", false},
		} {
			got := checkDependency(tc.source, tc.dependency, "ucupa", strings.HasSuffix(tc.source, "_test.go"), testStdlib())
			if (got == "") != tc.allowed {
				t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
			}
		}
		for _, location := range []string{owner + "/contract/cut.go", owner + "/internal/token/cut.go"} {
			if got := checkSourceLocation(location); got != "" {
				t.Errorf("reviewed source %s: %s", location, got)
			}
		}
		for _, location := range []string{owner + "/internal/unreviewed/file.go", owner + "/internal/token/hidden/file.go", owner + "/transport/grpc/internal/token/file.go", owner + "/sdk/go/internal/token/file.go"} {
			if checkSourceLocation(location) == "" {
				t.Errorf("unreviewed internal source accepted: %s", location)
			}
		}
	}
}
