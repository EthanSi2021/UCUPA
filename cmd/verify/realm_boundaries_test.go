package main

import "testing"

func TestRealmResponsibilityEdgesAndOldRootRefusal(t *testing.T) {
	stdlib := map[string]bool{"context": true, "errors": true, "net/http": true, "os": true, "sync": true, "time": true, "testing": true}
	positive := [][2]string{
		{"ucupa-realm/contract/binding.go", "ucupa/ucupa-realm/internal/token"},
		{"ucupa-realm/instance/instance.go", "ucupa/ucupa-realm/internal/policy"},
		{"ucupa-realm/internal/policy/store.go", "go.etcd.io/bbolt"},
		{"ucupa-realm/internal/identity/identity_assertion.go", "ucupa/ucupa-realm/contract"},
		{"ucupa-realm/transport/grpc/server.go", "ucupa/ucupa-realm/transport/grpc/internal/entry"},
		{"ucupa-realm/transport/grpc/internal/entry/service.go", "ucupa/ucupa-realm/transport/grpc/internal/admission"},
		{"ucupa-realm/transport/grpc/internal/admission/service.go", "ucupa/ucupa-realm/transport/grpc/internal/wire"},
		{"ucupa-realm/transport/grpc/internal/wire/representation.go", "ucupa/internal/protovalue"},
	}
	for _, pair := range positive {
		if reason := checkDependency(pair[0], pair[1], "ucupa", false, stdlib); reason != "" {
			t.Errorf("allowed %v: %s", pair, reason)
		}
	}
	negative := [][2]string{
		{"composition/realm.go", "ucupa/ucupa-realm"},
		{"composition/realm.go", "ucupa/ucupa-realm/internal/policy"},
		{"ucupa-realm/contract/binding.go", "ucupa/ucupa-realm/instance"},
		{"ucupa-realm/contract/binding.go", "os"},
		{"ucupa-realm/internal/token/revision.go", "sync"},
		{"ucupa-realm/internal/identity/identity_assertion.go", "ucupa/ucupa-realm/internal/policy"},
		{"ucupa-realm/internal/policy/owner.go", "ucupa/ucupa-realm/instance"},
		{"ucupa-realm/internal/policy/owner.go", "net/http"},
		{"ucupa-realm/transport/grpc/server.go", "ucupa/ucupa-realm/instance"},
		{"ucupa-realm/transport/grpc/internal/entry/service.go", "ucupa/ucupa-realm/internal/policy"},
		{"ucupa-realm/transport/grpc/internal/wire/representation.go", "ucupa/ucupa-realm/transport/grpc/internal/admission"},
		{"ucupa-realm/sdk/go/client.go", "ucupa/ucupa-realm/transport/grpc/internal/entry"},
		{"ucupa-core/transport/grpc/server.go", "ucupa/ucupa-realm/transport/grpc/internal/entry"},
	}
	for _, pair := range negative {
		if reason := checkDependency(pair[0], pair[1], "ucupa", false, stdlib); reason == "" {
			t.Errorf("forbidden dependency accepted: %v", pair)
		}
	}
}

func TestRealmLocationsRejectUnreviewedFlattening(t *testing.T) {
	for dir, names := range realmSourceFiles {
		_ = names
		if reason := checkSourceLocation(dir + "/extra.go"); reason == "" {
			t.Errorf("unreviewed file accepted: %s", dir)
		}
	}
	for _, file := range []string{"ucupa-realm/realm.go", "ucupa-realm/realm_test.go", "ucupa-realm/internal/misc/helper.go", "ucupa-realm/transport/grpc/service.go"} {
		if reason := checkSourceLocation(file); reason == "" {
			t.Errorf("retired/unknown location accepted: %s", file)
		}
	}
	for _, file := range []string{"ucupa-realm/instance/instance.go", "ucupa-realm/tests/instance/instance_test.go", "ucupa-realm/internal/policy/store.go", "ucupa-realm/transport/grpc/internal/entry/service.go"} {
		if reason := checkSourceLocation(file); reason != "" {
			t.Errorf("reviewed location refused: %s: %s", file, reason)
		}
	}
}
