package main

import "testing"

func TestWriterBoundariesAreExact(t *testing.T) {
	std := testStdlib()
	for source, deps := range writerDependencies {
		if got := checkSourceLocation(source); got != "" {
			t.Errorf("%s: %s", source, got)
		}
		for _, dep := range deps {
			if !std[dep] && dep != "go.etcd.io/bbolt" && len(dep) > 0 && dep[0] != 'g' {
				dep = "ucupa/" + dep
			}
			if got := checkDependency(source, dep, "ucupa", false, std); got != "" {
				t.Errorf("%s -> %s: %s", source, dep, got)
			}
		}
		for _, bad := range []string{"ucupa/ucupa-core/instance", "ucupa/ucupa-facet/instance", "ucupa/ucupa-realm", "ucupa/composition"} {
			if got := checkDependency(source, bad, "ucupa", false, std); got == "" {
				t.Errorf("%s allowed %s", source, bad)
			}
		}
	}
	for _, file := range []string{"ucupa-core/internal/handoff/extra.go", "ucupa-core/internal/handoff/extra/extra.go", "ucupa-facet/transport/grpc/internal/writer/extra.go"} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("allowed unreviewed %s", file)
		}
	}
	for _, source := range []string{"ucupa-core/contract/writer.go", "ucupa-facet/contract/writer.go", "ucupa-core/internal/commit/state.go", "ucupa-core/sdk/go/client.go"} {
		if got := checkDependency(source, "ucupa/ucupa-core/internal/handoff", "ucupa", false, std); got == "" {
			t.Errorf("reverse dependency %s", source)
		}
	}
}
