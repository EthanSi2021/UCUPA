package main

import "testing"

func TestNetworkDependencyBoundaries(t *testing.T) {
	stdlib := testStdlib()
	for _, item := range []struct{ source, dependency string }{
		{"ucupa-core/core.go", "google.golang.org/grpc"},
		{"ucupa-core/core.go", "ucupa/shared/bindings/go/protobuf"},
		{"ucupa-core/sdk/go/client.go", "ucupa/ucupa-core"},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-realm"},
		{"shared/bindings/go/protobuf/network.pb.go", "ucupa/ucupa-core"},
		{"shared/values/anything.go", "google.golang.org/protobuf/proto"},
		{"ucupa-facet/facet.go", "ucupa/internal/protovalue"},
		{"ucupa-facet/sdk/go/client.go", "ucupa/internal/protovalue"},
		{"composition/wire.go", "ucupa/internal/protovalue"},
		{"internal/protovalue/value.go", "ucupa/ucupa-facet"},
		{"internal/protovalue/value.go", "ucupa/composition"},
		{"ucupa-facet/facet.go", "ucupa/internal/grpcbound"},
		{"ucupa-facet/sdk/go/client.go", "ucupa/internal/grpcbound"},
		{"ucupa-core/core.go", "ucupa/internal/grpcdial"},
		{"ucupa-core/transport/grpc/server.go", "ucupa/internal/grpcdial"},
		{"internal/grpcbound/limits.go", "ucupa/ucupa-core"},
		{"internal/grpcdial/dial.go", "ucupa/composition"},
	} {
		if checkDependency(item.source, item.dependency, "ucupa", false, stdlib) == "" {
			t.Errorf("accepted %s -> %s", item.source, item.dependency)
		}
	}
	for _, file := range []string{"shared/bindings/go/protobuf/handwritten.go", "ucupa-core/bindings/go/protobuf/extra.pb.go", "ucupa-facet/bindings/go/protobuf/extra.pb.go", "ucupa-core/transport/http/server.go"} {
		if checkSourceLocation(file) == "" {
			t.Errorf("unapproved source %s", file)
		}
	}
	if checkDependency("shared/bindings/go/protobuf/network.pb.go", "google.golang.org/protobuf/runtime/protoimpl", "ucupa", false, stdlib) != "" {
		t.Fatal("registered runtime rejected")
	}
}
