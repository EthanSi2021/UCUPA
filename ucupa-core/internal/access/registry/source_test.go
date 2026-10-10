package registry

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	federationdenial "ucupa/ucupa-core/internal/query/federation/denial"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestSourceBindingsKeepFirstViewAndPartialCharges(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	reservation, err := registry.ReserveLocked(4096)
	if err != nil {
		t.Fatal(err)
	}
	mounts := mountfact.New()
	config := SourceConfig{
		Source: registry.authority,
		Request: federationcontract.FederationReadRequest{
			Boundary: boundary, Intent: "query", Purpose: [32]byte{1},
		},
		Mode: contract.QueryCurrentVerified,
	}
	if err := CheckSourceConfig(config, mounts.View); err != nil {
		t.Fatal(err)
	}
	read, err := registry.IssueSourceLocked(reservation, caller, boundary, contract.CoreReadCut{}, values.AuthorizationView{}, nil, config)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.active {
		t.Fatal("source reservation was not transferred")
	}
	if err := registry.CheckSourceRelationsLocked(read.Ref, mounts.View, transferfact.Visibility{}); err != nil {
		t.Fatal(err)
	}
	element := values.ElementID("10000000-0000-4000-8000-000000000002")
	first := values.AuthorizationView{1}
	bound, err := registry.BindSourceReadViewLocked(read.Ref, element, first)
	if err != nil || bound != first {
		t.Fatalf("first bind = %x, %v", bound, err)
	}
	bound, err = registry.BindSourceReadViewLocked(read.Ref, element, values.AuthorizationView{2})
	if err != nil || bound != first {
		t.Fatalf("replacement changed first view = %x, %v", bound, err)
	}
	firstEdge := values.ElementID("10000000-0000-4000-8000-000000000003")
	if err := registry.BindSourceDependenciesLocked(read.Ref, mounts.View, map[values.ElementID]mountfact.Record{firstEdge: {}}); err != nil {
		t.Fatal(err)
	}
	if len(read.Ref.entry.read.source.outgoing) != 1 {
		t.Fatalf("outgoing dependencies = %d", len(read.Ref.entry.read.source.outgoing))
	}
	used := registry.bytes
	registry.bytes = registry.limits.Bytes
	secondEdge := values.ElementID("10000000-0000-4000-8000-000000000004")
	err = registry.BindSourceDependenciesLocked(read.Ref, mounts.View, map[values.ElementID]mountfact.Record{secondEdge: {}})
	if !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatalf("capacity error = %v", err)
	}
	if len(read.Ref.entry.read.source.outgoing) != 1 || read.Ref.entry.read.source.outgoing[firstEdge] != (mountfact.Record{}) {
		t.Fatal("earlier source binding was rolled back or replaced")
	}
	registry.bytes = used
	mismatch := mountfact.IncomingOnly(mountfact.Incoming{Detached: true})
	if err := CheckSourceConfig(config, mismatch.View); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("incoming mismatch error = %v", err)
	}
	registry.RemoveLocked(read.Ref)
	if registry.bytes != 0 || len(registry.entries) != 0 {
		t.Fatalf("source resources remain: entries=%d bytes=%d", len(registry.entries), registry.bytes)
	}
	locker.Unlock()
}

func TestHiddenPreparationUsesReceiptTTLAndOneFixedCursor(t *testing.T) {
	registry, locker, _, caller, boundary := registryFixture(t, 1<<20)
	locker.Lock()
	config := SourceConfig{
		Source: registry.authority,
		Request: federationcontract.FederationReadRequest{
			Boundary: boundary, Intent: "query", PrepareObserve: true, Purpose: [32]byte{1},
		},
		Mode: contract.QueryCurrentVerified,
	}
	// Same-package construction is sufficient here: denial contents are never
	// interpreted by Registry, only retained as an opaque non-nil capability.
	config.Denial = new(federationdenial.Evidence)
	read, err := registry.IssueHiddenSourceLocked(caller, boundary, contract.CoreReadCut{}, config)
	if err != nil {
		t.Fatal(err)
	}
	prep, cursor, err := registry.PrepareHiddenLocked(read.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if prep.Ref.entry.expires != read.Ref.entry.expires {
		t.Fatalf("hidden expiry=%v receipt=%v", prep.Ref.entry.expires, read.Ref.entry.expires)
	}
	if prep.Initial != cursor.Token || cursor.Position != 0 || cursor.Preparation != prep.Ref {
		t.Fatalf("hidden cursor not fixed to preparation: prep=%#v cursor=%#v", prep, cursor)
	}
	registry.RemoveLocked(read.Ref)
	if registry.bytes != 0 || len(registry.entries) != 0 {
		t.Fatalf("hidden resources remain: entries=%d bytes=%d", len(registry.entries), registry.bytes)
	}
	locker.Unlock()
}
