package main

import "testing"

func TestFacetNamespaceAndInstanceDependencies(t *testing.T) {
	const namespace = "ucupa-facet/internal/namespace"
	const instance = "ucupa-facet/instance"
	std := testStdlib()
	std["sync"], std["testing"] = true, true
	for _, tc := range []struct {
		file, imported string
		allowed        bool
	}{
		{namespace + "/owner.go", "sync", true},
		{namespace + "/owner.go", "ucupa/ucupa-facet/internal/access", true},
		{namespace + "/state.go", "ucupa/ucupa-facet/internal/namespace/request", true},
		{namespace + "/state.go", "ucupa/ucupa-facet/internal/token", true},
		{namespace + "/content.go", "ucupa/ucupa-facet/contract", true},
		{namespace + "/store.go", "go.etcd.io/bbolt", true},
		{namespace + "/store_operations.go", "go.etcd.io/bbolt", true},
		{namespace + "/content.go", "go.etcd.io/bbolt", false},
		{namespace + "/owner.go", "ucupa/ucupa-facet/instance", false},
		{namespace + "/owner.go", "ucupa/ucupa-facet", false},
		{namespace + "/owner.go", "ucupa/ucupa-core", false},
		{namespace + "/owner.go", "ucupa/ucupa-facet/internal/commit", false},
		{namespace + "/owner.go", "ucupa/ucupa-facet/internal/store", false},
		{namespace + "/owner.go", "ucupa/ucupa-facet/transport/grpc", false},
		{namespace + "/owner.go", "ucupa/composition", false},
		{instance + "/instance.go", "ucupa/" + namespace, true},
		{instance + "/instance.go", "ucupa/ucupa-facet/contract", true},
		{instance + "/instance.go", "ucupa/shared/values", true},
		{instance + "/instance.go", "ucupa/ucupa-facet/internal/access", false},
		{instance + "/instance.go", "ucupa/ucupa-facet/internal/token", false},
		{instance + "/instance.go", "go.etcd.io/bbolt", false},
		{instance + "/instance.go", "ucupa/ucupa-facet/transport/grpc", false},
		{"composition/cluster.go", "ucupa/" + instance, true},
		{"composition/cluster.go", "ucupa/" + namespace, false},
		{"ucupa-facet/transport/grpc/server.go", "ucupa/" + instance, false},
		{"ucupa-facet/transport/grpc/server.go", "ucupa/" + namespace, false},
		{"ucupa-facet/sdk/go/client.go", "ucupa/" + instance, false},
		{"ucupa-facet/contract/namespace.go", "ucupa/" + namespace, false},
		{"ucupa-facet/internal/access/resources.go", "ucupa/" + namespace, false},
	} {
		got := checkDependency(tc.file, tc.imported, "ucupa", false, std)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.file, tc.imported, tc.allowed, got)
		}
	}
	for _, file := range []string{namespace + "/owner.go", instance + "/instance.go"} {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed source %s: %s", file, got)
		}
	}
	for _, file := range []string{"ucupa-facet/facet.go", "ucupa-facet/alias.go", "ucupa-facet/root_test.go", namespace + "/extra/owner.go", instance + "/extra/config.go", "ucupa-core/instance/extra/config.go"} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unreviewed source %s", file)
		}
	}
	for _, file := range []string{"acceptance/fixture_test.go", "composition/cluster.go", "ucupa-facet/instance/instance.go"} {
		if checkDependency(file, "ucupa/ucupa-facet", "ucupa", true, std) == "" {
			t.Errorf("legacy root import accepted for %s", file)
		}
	}
}
