package main

// Exactly these products are reproduced by cmd/protogen in the full gate.
// Neither a path prefix nor a generated comment grants a general exemption.
func generatedSource(path string) bool {
	switch path {
	case "shared/bindings/go/protobuf/network.pb.go", "shared/bindings/go/protobuf/network_grpc.pb.go",
		"ucupa-core/bindings/go/protobuf/core.pb.go", "ucupa-core/bindings/go/protobuf/core_grpc.pb.go",
		"ucupa-facet/bindings/go/protobuf/facet.pb.go", "ucupa-facet/bindings/go/protobuf/facet_grpc.pb.go",
		"ucupa-realm/bindings/go/protobuf/realm.pb.go", "ucupa-realm/bindings/go/protobuf/realm_grpc.pb.go":
		return true
	}
	return false
}
