//go:build !linux

package acceptance_test

import (
	"testing"

	netpb "ucupa/shared/bindings/go/protobuf"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetsdk "ucupa/ucupa-facet/sdk/go"
)

func verifyP8IsolatedDiskFull(t *testing.T, _ string, _ string, _ *facetsdk.Client, _ *facetpb.Access, _ []byte, _ *netpb.Token, _ func()) {
	t.Helper()
	t.Fatal("P8 isolated disk-full exercise requires Linux and a separately mounted bounded filesystem")
}

func verifyP8InFlightCrash(t *testing.T, _ int, _ *facetsdk.Client, _ *facetpb.Access, _ []byte, _ *netpb.Token, _ func()) {
	t.Helper()
	t.Fatal("P8 in-flight crash exercise requires Linux and an independently traced product process")
}
