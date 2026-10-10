package main

import (
	"slices"
	"strings"
	"testing"
)

func TestTransferPreflightDependenciesAreExact(t *testing.T) {
	std := testStdlib()
	for file, dependencies := range transferDependencies {
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("%s: %s", file, got)
		}
		for _, dependency := range dependencies {
			imported := "ucupa/" + dependency
			if dependency == "go.etcd.io/bbolt" || strings.HasPrefix(dependency, "google.golang.org/") {
				imported = dependency
			}
			if got := checkDependency(file, imported, "ucupa", false, std); got != "" {
				t.Errorf("%s -> %s: %s", file, dependency, got)
			}
		}
		for _, forbidden := range []string{"ucupa-core/instance", "ucupa-core/internal/element", "ucupa-core/internal/query", "ucupa-core/internal/mount", "ucupa-core/internal/store", "ucupa-core/internal/commit", "ucupa-facet/contract", "ucupa-realm", "composition"} {
			if slices.Contains(dependencies, forbidden) {
				continue
			}
			if got := checkDependency(file, "ucupa/"+forbidden, "ucupa", false, std); got == "" {
				t.Errorf("%s allowed %s", file, forbidden)
			}
		}
	}
	for _, file := range []string{"ucupa-core/internal/transfer/extra.go", "ucupa-core/internal/transfer/fact/extra.go", "ucupa-core/internal/transfer/fact/nested/extra.go", "ucupa-core/transport/grpc/internal/transition/extra.go"} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unregistered %s", file)
		}
	}
	for _, file := range []string{"ucupa-core/instance/instance.go", "ucupa-core/contract/element.go", "ucupa-core/sdk/go/client.go", "ucupa-core/transport/grpc/server.go", "ucupa-core/internal/element/fact/state.go", "ucupa-facet/internal/namespace/namespace.go"} {
		if got := checkDependency(file, "ucupa/ucupa-core/internal/transfer/fact", "ucupa", false, std); got == "" {
			t.Errorf("reverse/cross-boundary %s", file)
		}
	}
	if got := checkDependency("ucupa-core/instance/instance.go", "ucupa/ucupa-core/internal/transfer", "ucupa", false, std); got != "" {
		t.Fatal(got)
	}
	for _, file := range []string{"ucupa-core/contract/transition.go", "ucupa-core/sdk/go/client.go", "ucupa-core/transport/grpc/server.go", "ucupa-core/internal/commit/public_transition.go", "ucupa-facet/instance/instance.go"} {
		if got := checkDependency(file, "ucupa/ucupa-core/internal/transfer", "ucupa", false, std); got == "" {
			t.Errorf("workflow reverse/direct import: %s", file)
		}
	}
	for _, file := range []string{"ucupa-core/internal/transfer/operation.go", "ucupa-core/sdk/go/client.go", "ucupa-core/transport/grpc/internal/mount/control.go", "ucupa-facet/transport/grpc/server.go"} {
		if got := checkDependency(file, "ucupa/ucupa-core/transport/grpc/internal/transition", "ucupa", false, std); got == "" {
			t.Errorf("RPC implementation escape: %s", file)
		}
	}
}

func TestPersistedCodecsCannotBecomeAnotherOwner(t *testing.T) {
	std := testStdlib()
	for _, name := range []string{"sync", "os", "net", "context", "time", "unsafe"} {
		std[name] = true
	}
	for _, file := range []string{"ucupa-core/internal/store/record/element.go", "ucupa-core/internal/transfer/encoding/manifest_decode.go", "ucupa-core/transport/grpc/internal/transitionwire/evidence.go"} {
		for _, imported := range []string{"sync", "os", "net", "context", "time", "unsafe", "go.etcd.io/bbolt", "ucupa/ucupa-core/internal/commit", "ucupa/ucupa-core/internal/transfer", "ucupa/ucupa-core/internal/access", "ucupa/ucupa-core/instance"} {
			if got := checkDependency(file, imported, "ucupa", false, std); got == "" {
				t.Errorf("pure codec %s allowed %s", file, imported)
			}
		}
	}
	for _, file := range []string{"ucupa-core/internal/store/record/extra.go", "ucupa-core/internal/transfer/encoding/extra.go", "ucupa-core/transport/grpc/internal/transitionwire/extra.go", "ucupa-core/transport/grpc/transitionconn/extra.go"} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed codec source %s", file)
		}
	}
	for _, file := range []string{"ucupa-core/contract/transition.go", "ucupa-core/sdk/go/client.go", "ucupa-core/transport/grpc/server.go", "ucupa-core/internal/element/fact/state.go"} {
		for _, imported := range []string{"ucupa/ucupa-core/internal/store/record", "ucupa/ucupa-core/internal/transfer/encoding", "ucupa/ucupa-core/transport/grpc/internal/transitionwire"} {
			if got := checkDependency(file, imported, "ucupa", false, std); got == "" {
				t.Errorf("private encoding escape %s -> %s", file, imported)
			}
		}
	}
}
