package main

import (
	"path"
	"strings"
)

// P7.1 adds lifecycle control, not a general transition framework. Each actual
// source and consumer is registered; adding another file still needs review.
var writerDependencies = map[string][]string{
	"ucupa-core/internal/handoff/operation.go":                   {"context", "shared/values", "ucupa-core/contract", "ucupa-core/internal/access", "ucupa-core/internal/access/registry", "ucupa-core/internal/commit"},
	"ucupa-core/internal/commit/writer.go":                       {"errors", "math", "go.etcd.io/bbolt", "shared/values"},
	"ucupa-core/internal/commit/writer_record.go":                {"bytes", "encoding/json", "math", "unicode/utf8", "shared/values"},
	"ucupa-core/internal/commit/writer_store.go":                 {"bytes", "crypto/sha256", "encoding/binary", "encoding/json", "errors", "go.etcd.io/bbolt", "shared/values"},
	"ucupa-core/internal/commit/writer_test.go":                  {"errors", "path/filepath", "reflect", "testing", "go.etcd.io/bbolt", "shared/values", "ucupa-core/contract", "ucupa-core/internal/element/fact"},
	"ucupa-facet/internal/namespace/writer.go":                   {"context", "crypto/sha256", "errors", "math", "reflect", "go.etcd.io/bbolt", "shared/values", "ucupa-facet/contract"},
	"ucupa-facet/internal/namespace/writer_record.go":            {"bytes", "encoding/json", "math", "unicode/utf8", "shared/values"},
	"ucupa-facet/internal/namespace/writer_store.go":             {"bytes", "crypto/sha256", "encoding/json", "errors", "go.etcd.io/bbolt", "shared/values"},
	"ucupa-core/transport/grpc/internal/writer/service.go":       {"context", "errors", "google.golang.org/grpc/codes", "google.golang.org/grpc/status", "google.golang.org/protobuf/proto", "shared/bindings/go/protobuf", "shared/values", "ucupa-core/bindings/go/protobuf", "ucupa-core/transport/grpc/internal/session", "ucupa-core/transport/grpc/internal/admission"},
	"ucupa-facet/transport/grpc/internal/writer/service.go":      {"context", "errors", "google.golang.org/grpc/codes", "google.golang.org/grpc/status", "google.golang.org/protobuf/proto", "shared/bindings/go/protobuf", "shared/values", "ucupa-facet/bindings/go/protobuf"},
	"ucupa-core/transport/grpc/internal/writer/service_test.go":  {"context", "testing", "google.golang.org/grpc/codes", "google.golang.org/grpc/status", "google.golang.org/protobuf/proto", "shared/bindings/go/protobuf", "shared/values", "ucupa-core/transport/grpc/internal/session", "ucupa-core/transport/grpc/internal/admission"},
	"ucupa-facet/transport/grpc/internal/writer/service_test.go": {"context", "testing", "google.golang.org/grpc/codes", "google.golang.org/grpc/status", "google.golang.org/protobuf/proto", "shared/bindings/go/protobuf", "shared/values"},
	"ucupa-facet/internal/namespace/writer_test.go":              {"context", "crypto/sha256", "encoding/json", "errors", "path/filepath", "reflect", "testing", "time", "go.etcd.io/bbolt", "shared/values", "ucupa-facet/contract"},
}

func checkWriterDependency(file, imported, module string, test bool, stdlib map[string]bool) (bool, string) {
	if deps, ok := writerDependencies[file]; ok {
		if reviewedDependencyAllowed(imported, module, stdlib, deps...) {
			return true, ""
		}
		return true, "writer source may consume only its exact reviewed lifecycle, value or wire dependencies"
	}
	for _, dir := range []string{"ucupa-core/internal/handoff", "ucupa-core/transport/grpc/internal/writer", "ucupa-facet/transport/grpc/internal/writer"} {
		if imported != module+"/"+dir {
			continue
		}
		if dir == "ucupa-core/internal/handoff" && file == "ucupa-core/instance/instance.go" {
			return true, ""
		}
		if dir == "ucupa-core/transport/grpc/internal/writer" && file == "ucupa-core/transport/grpc/server.go" {
			return true, ""
		}
		if dir == "ucupa-facet/transport/grpc/internal/writer" && file == "ucupa-facet/transport/grpc/server.go" {
			return true, ""
		}
		return true, "writer internals may be consumed only by their exact instance or network assembler"
	}
	return false, ""
}
func writerLocation(file string) (bool, string) {
	for _, dir := range []string{"ucupa-core/internal/handoff", "ucupa-core/transport/grpc/internal/writer", "ucupa-facet/transport/grpc/internal/writer"} {
		if path.Dir(file) == dir || strings.HasPrefix(file, dir+"/") {
			if _, ok := writerDependencies[file]; ok {
				return true, ""
			}
			return true, "writer source needs exact responsibility registration"
		}
	}
	return false, ""
}
