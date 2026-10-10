package main

import "testing"

const (
	coreFederationProjectionDirectory = "ucupa-core/internal/query/federation/projection"
	coreFederationTranscriptDirectory = "ucupa-core/internal/query/federation/transcript"
)

var coreFederationProjectionSourceFiles = []string{
	"projection.go",
	"path.go",
	"record.go",
	"query.go",
	"traversal.go",
	"relation.go",
	"failure.go",
	"traversal_test.go",
	"failure_test.go",
	"projection_test.go",
	"query_test.go",
}

var coreFederationTranscriptSourceFiles = []string{"transcript.go", "transcript_test.go"}

func TestReviewedCoreFederationProjectionBoundary(t *testing.T) {
	if !reviewedCoreFederationProjectionPackage("ucupa-core", coreFederationProjectionDirectory) ||
		reviewedCoreFederationProjectionPackage("ucupa-facet", coreFederationProjectionDirectory) ||
		reviewedCoreFederationProjectionPackage("ucupa-core", coreFederationProjectionDirectory+"/child") {
		t.Fatal("Core Federation projection registration must identify exactly one package")
	}
	for _, name := range coreFederationProjectionSourceFiles {
		file := coreFederationProjectionDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Federation projection source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		coreFederationProjectionDirectory + "/extra.go",
		coreFederationProjectionDirectory + "/extra_test.go",
		coreFederationProjectionDirectory + "/child/projection.go",
		"ucupa-core/internal/query/federation/projection/nested.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Federation projection source accepted: %s", file)
		}
	}

	stdlib := federationProjectionStdlib()
	for _, dependency := range []string{
		"ucupa/shared/values",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/internal/element/fact",
		"ucupa/ucupa-core/internal/element/read",
		"ucupa/ucupa-core/internal/facet/fact",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency(coreFederationProjectionDirectory+"/projection.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Federation projection dependency %s: %s", dependency, got)
		}
	}
	for _, tc := range []struct {
		source, dependency string
	}{
		{coreFederationProjectionDirectory + "/path.go", "ucupa/ucupa-core/internal/query/federation/value"},
		{coreFederationProjectionDirectory + "/query.go", "ucupa/ucupa-core/internal/query/federation/value"},
		{coreFederationProjectionDirectory + "/record.go", "ucupa/ucupa-core/internal/query/federation/value"},
		{coreFederationProjectionDirectory + "/relation.go", "ucupa/ucupa-core/internal/query/federation/value"},
		{coreFederationProjectionDirectory + "/traversal.go", "ucupa/ucupa-core/internal/query/federation/transcript"},
	} {
		if got := checkDependency(tc.source, tc.dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Federation projection dependency %s -> %s: %s", tc.source, tc.dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/contract/federation",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/request",
		"ucupa/ucupa-core/internal/query/connected",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/mount",
		"ucupa/ucupa-core/internal/facet",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreFederationProjectionDirectory+"/projection.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Federation projection dependency accepted: %s", dependency)
		}
	}
	for _, tc := range []struct {
		source, dependency string
	}{
		{coreFederationProjectionDirectory + "/projection.go", "ucupa/ucupa-core/internal/query/federation/value"},
		{coreFederationProjectionDirectory + "/projection.go", "ucupa/ucupa-core/internal/query/federation/transcript"},
		{coreFederationProjectionDirectory + "/traversal.go", "ucupa/ucupa-core/internal/query/federation/value"},
		{coreFederationProjectionDirectory + "/path.go", "ucupa/ucupa-core/internal/query/federation/transcript"},
	} {
		if got := checkDependency(tc.source, tc.dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("unapproved Federation projection dependency accepted: %s -> %s", tc.source, tc.dependency)
		}
	}
	if got := checkDependency(coreFederationProjectionDirectory+"/projection_test.go", "ucupa/"+coreFederationProjectionDirectory, "ucupa", true, stdlib); got != "" {
		t.Fatalf("Federation projection white-box self dependency rejected: %s", got)
	}
}

func TestReviewedCoreFederationTranscriptBoundary(t *testing.T) {
	if !reviewedCoreFederationTranscriptPackage("ucupa-core", coreFederationTranscriptDirectory) ||
		reviewedCoreFederationTranscriptPackage("ucupa-facet", coreFederationTranscriptDirectory) ||
		reviewedCoreFederationTranscriptPackage("ucupa-core", coreFederationTranscriptDirectory+"/child") {
		t.Fatal("Core Federation transcript registration must identify exactly one package")
	}
	for _, name := range coreFederationTranscriptSourceFiles {
		file := coreFederationTranscriptDirectory + "/" + name
		if got := checkSourceLocation(file); got != "" {
			t.Errorf("reviewed Federation transcript source %s: %s", file, got)
		}
	}
	for _, file := range []string{
		coreFederationTranscriptDirectory + "/extra.go",
		coreFederationTranscriptDirectory + "/transcript_extra_test.go",
		coreFederationTranscriptDirectory + "/child/transcript.go",
		"ucupa-core/internal/query/federation/transcript/nested.go",
	} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed Federation transcript source accepted: %s", file)
		}
	}

	stdlib := federationTranscriptStdlib()
	for _, dependency := range []string{
		"ucupa/shared/values",
		"ucupa/ucupa-core/internal/query/federation/value",
		"ucupa/ucupa-core/internal/token",
	} {
		if got := checkDependency(coreFederationTranscriptDirectory+"/transcript.go", dependency, "ucupa", false, stdlib); got != "" {
			t.Errorf("allowed Federation transcript dependency %s: %s", dependency, got)
		}
	}
	for _, dependency := range []string{
		"ucupa/ucupa-core",
		"ucupa/ucupa-core/contract",
		"ucupa/ucupa-core/contract/federation",
		"ucupa/ucupa-core/internal/query",
		"ucupa/ucupa-core/internal/query/scan",
		"ucupa/ucupa-core/internal/query/federation/projection",
		"ucupa/ucupa-core/internal/commit",
		"ucupa/ucupa-core/internal/store",
		"ucupa/ucupa-core/internal/access",
		"ucupa/ucupa-core/internal/geometry",
		"ucupa/ucupa-core/internal/mount/fact",
		"ucupa/ucupa-core/transport/grpc",
		"ucupa/ucupa-core/sdk/go",
		"go.etcd.io/bbolt",
	} {
		if got := checkDependency(coreFederationTranscriptDirectory+"/transcript.go", dependency, "ucupa", false, stdlib); got == "" {
			t.Errorf("forbidden Federation transcript dependency accepted: %s", dependency)
		}
	}
	for _, dependency := range []string{"ucupa/ucupa-core/contract", "ucupa/ucupa-core/internal/query/federation/value", "ucupa/ucupa-core/internal/token"} {
		if got := checkDependency(coreFederationTranscriptDirectory+"/transcript_test.go", dependency, "ucupa", true, stdlib); got != "" {
			t.Errorf("allowed Federation transcript test dependency %s rejected: %s", dependency, got)
		}
	}
}

func TestFederationProjectionConsumersAreExact(t *testing.T) {
	stdlib := federationProjectionStdlib()
	for _, tc := range []struct {
		source, dependency string
		test               bool
		allowed            bool
	}{
		{"ucupa-core/public_federation_projection.go", "ucupa/" + coreFederationProjectionDirectory, false, true},
		{"ucupa-core/public_federation_observe_binding.go", "ucupa/" + coreFederationProjectionDirectory, false, true},
		{"ucupa-core/public_federation_part.go", "ucupa/" + coreFederationProjectionDirectory, false, true},
		{"ucupa-core/public_federation_hidden.go", "ucupa/" + coreFederationProjectionDirectory, false, true},
		{"ucupa-core/public_federation_parent_test.go", "ucupa/" + coreFederationProjectionDirectory, true, true},
		{coreFederationProjectionDirectory + "/projection_test.go", "ucupa/" + coreFederationProjectionDirectory, true, true},
		{"ucupa-core/public_federated_read.go", "ucupa/" + coreFederationProjectionDirectory, false, false},
		{"ucupa-core/public_federation_query.go", "ucupa/" + coreFederationProjectionDirectory, false, false},
		{"ucupa-core/internal/query/operation.go", "ucupa/" + coreFederationProjectionDirectory, false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/" + coreFederationProjectionDirectory, false, false},
		{"ucupa-core/transport/grpc/federation_read.go", "ucupa/" + coreFederationProjectionDirectory, false, false},
		{"ucupa-core/sdk/go/federation.go", "ucupa/" + coreFederationProjectionDirectory, false, false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}

func TestFederationTranscriptConsumersAreExact(t *testing.T) {
	stdlib := federationTranscriptStdlib()
	for _, tc := range []struct {
		source, dependency string
		test               bool
		allowed            bool
	}{
		{coreFederationProjectionDirectory + "/traversal.go", "ucupa/" + coreFederationTranscriptDirectory, false, true},
		{"ucupa-core/public_federated_closure.go", "ucupa/" + coreFederationTranscriptDirectory, false, true},
		{"ucupa-core/public_federated_disclosure.go", "ucupa/" + coreFederationTranscriptDirectory, false, true},
		{coreFederationTranscriptDirectory + "/transcript_test.go", "ucupa/" + coreFederationTranscriptDirectory, true, true},
		{"ucupa-core/public_federated_read.go", "ucupa/" + coreFederationTranscriptDirectory, false, false},
		{"ucupa-core/internal/query/operation.go", "ucupa/" + coreFederationTranscriptDirectory, false, false},
		{"ucupa-core/internal/commit/state.go", "ucupa/" + coreFederationTranscriptDirectory, false, false},
		{"ucupa-core/transport/grpc/federation_read.go", "ucupa/" + coreFederationTranscriptDirectory, false, false},
		{"ucupa-core/sdk/go/federation.go", "ucupa/" + coreFederationTranscriptDirectory, false, false},
	} {
		got := checkDependency(tc.source, tc.dependency, "ucupa", tc.test, stdlib)
		if (got == "") != tc.allowed {
			t.Errorf("%s -> %s: allowed=%v got=%q", tc.source, tc.dependency, tc.allowed, got)
		}
	}
}

func TestFederationValueProjectionAndTranscriptConsumersAreExact(t *testing.T) {
	stdlib := federationProjectionStdlib()
	for _, source := range []string{
		coreFederationProjectionDirectory + "/path.go",
		coreFederationProjectionDirectory + "/query.go",
		coreFederationProjectionDirectory + "/record.go",
		coreFederationProjectionDirectory + "/relation.go",
		coreFederationTranscriptDirectory + "/transcript.go",
	} {
		if got := checkDependency(source, "ucupa/ucupa-core/internal/query/federation/value", "ucupa", false, stdlib); got != "" {
			t.Errorf("approved value consumer %s rejected: %s", source, got)
		}
	}
	if got := checkDependency(coreFederationTranscriptDirectory+"/transcript_test.go", "ucupa/ucupa-core/internal/query/federation/value", "ucupa", true, stdlib); got != "" {
		t.Fatalf("approved transcript test value dependency rejected: %s", got)
	}
	for _, tc := range []struct {
		source string
		test   bool
	}{
		{"ucupa-core/public_federation_query.go", false},
		{"ucupa-core/public_federated_read.go", false},
		{"ucupa-core/public_federation_projection.go", false},
		{"ucupa-core/public_federation_projection_test.go", true},
		{"ucupa-core/internal/query/operation.go", false},
		{"ucupa-core/transport/grpc/internal/wire/federation_wire.go", false},
		{"ucupa-core/sdk/go/federation.go", false},
	} {
		if got := checkDependency(tc.source, "ucupa/ucupa-core/internal/query/federation/value", "ucupa", tc.test, stdlib); got == "" {
			t.Errorf("unapproved value consumer accepted: %s", tc.source)
		}
	}
}

func TestReviewedMountFactValidationBoundary(t *testing.T) {
	const directory = "ucupa-core/internal/mount/fact"
	for _, name := range []string{"read.go", "record.go", "state.go", "state_test.go", "validation.go", "validation_test.go", "view.go", "view_test.go"} {
		if got := checkSourceLocation(directory + "/" + name); got != "" {
			t.Errorf("reviewed mount fact source %s rejected: %s", name, got)
		}
	}
	for _, file := range []string{directory + "/extra.go", directory + "/validation_extra_test.go", directory + "/child/validation.go"} {
		if got := checkSourceLocation(file); got == "" {
			t.Errorf("unreviewed mount fact source accepted: %s", file)
		}
	}
	stdlib := federationProjectionStdlib()
	if got := checkDependency(directory+"/validation.go", "ucupa/ucupa-core/internal/commit/binding", "ucupa", false, stdlib); got != "" {
		t.Fatalf("validation binding dependency rejected: %s", got)
	}
	if got := checkDependency(directory+"/validation_test.go", "ucupa/ucupa-core/internal/token", "ucupa", true, stdlib); got != "" {
		t.Fatalf("validation test token fixture rejected: %s", got)
	}
	for _, tc := range []struct {
		file, dependency string
		test             bool
	}{
		{directory + "/state.go", "ucupa/ucupa-core/internal/commit/binding", false},
		{directory + "/state_test.go", "ucupa/ucupa-core/internal/commit/binding", true},
		{directory + "/record.go", "ucupa/ucupa-core/internal/token", false},
		{directory + "/view.go", "ucupa/ucupa-core/internal/token", false},
	} {
		if got := checkDependency(tc.file, tc.dependency, "ucupa", tc.test, stdlib); got == "" {
			t.Errorf("unapproved mount fact dependency accepted: %s -> %s", tc.file, tc.dependency)
		}
	}
}

func federationProjectionStdlib() map[string]bool {
	stdlib := testStdlib()
	for _, dependency := range []string{"context", "errors", "math", "reflect", "sort", "strings", "testing"} {
		stdlib[dependency] = true
	}
	return stdlib
}

func federationTranscriptStdlib() map[string]bool {
	stdlib := testStdlib()
	for _, dependency := range []string{"bytes", "crypto/hmac", "crypto/sha256", "encoding/binary", "encoding/hex", "hash", "math", "testing"} {
		stdlib[dependency] = true
	}
	return stdlib
}
