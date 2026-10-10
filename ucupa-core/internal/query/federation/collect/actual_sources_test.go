package collect_test

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

// Migrated from the root TestFederationSourceVectorNestedSealAndRelease.
func TestFederationSourceVectorNestedSealAndRelease(t *testing.T) {
	fixture := newActualFixture(t, true, nil)
	request := federationcontract.FederationReadRequest{Boundary: fixture.root, Intent: "query", Purpose: [32]byte{8}, PrepareObserve: true}
	attempt, err := fixture.openQuery(t, request, 3, actualReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := collectActualPage(attempt.read, false)
	if err != nil || attempt.connections.openCount() != 2 || len(page.SourceGraph.Sources) != 3 {
		t.Fatal("did not retain all actual contributors", err)
	}
	for index, source := range page.SourceGraph.Sources {
		if source.Owner != fixture.owners[index].AuthorityRef() {
			t.Fatalf("source %d owner=%s, want %s", index, source.Owner, fixture.owners[index].AuthorityRef())
		}
	}
	// C contributed no matching Element to the initial page but remains an
	// explicit source whose receipt is checked on every later page.
	fixture.write(t, 2, 9750, actualID(9720), "after-vector")
	if _, err := collectActualPage(attempt.read, false); err != nil {
		t.Fatalf("retained ordinary write: %v", err)
	}
	if attempt.connections.openCount() != 2 {
		t.Fatal("sealed source set was reopened")
	}
	if err := attempt.read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := attempt.read.Retire(context.Background()); err != nil {
		t.Fatal("release not idempotent", err)
	}
	attempt.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)
}

// Migrated from the root TestFederationSourceVectorRejectsMismatchedAdmissionAndLimits.
func TestFederationSourceVectorRejectsMismatchedAdmissionAndLimits(t *testing.T) {
	fixture := newActualFixture(t, true, nil)
	request := federationcontract.FederationReadRequest{Boundary: fixture.root, Intent: "query", Purpose: [32]byte{9}}
	limited, err := fixture.openQuery(t, request, 1, actualReadOptions{})
	if !errors.Is(err, values.PublicResourceExhausted) || limited.connections.openCount() != 0 {
		t.Fatal("capacity checked after dialing", err)
	}
	limited.tracker.requireReleased(t, fixture.owners[0].AuthorityRef())

	var originalRange corecontract.BoundedSphere
	mismatched, err := fixture.openQuery(t, request, 3, actualReadOptions{alter: func(info *federationcontract.FederationReadPart) {
		originalRange = info.Incoming.Range
		info.Incoming.Range, _ = corecontract.NewBoundedSphere(1)
	}})
	if err != nil {
		t.Fatal(err)
	}
	page, err := collectActualPage(mismatched.read, false)
	if err != nil || !page.Partial || page.Gap != corecontract.QueryUnresolvedCoverage || len(page.SourceGraph.Sources) != 1 || page.SourceGraph.Sources[0].Owner != fixture.owners[0].AuthorityRef() {
		t.Fatalf("mismatched optional source was not isolated as unresolved coverage: %+v %v", page, err)
	}
	if originalRange.Radius() != 20 || mismatched.connections.openCount() != 1 {
		t.Fatalf("mismatched edge input/open count changed: radius=%v opens=%d", originalRange.Radius(), mismatched.connections.openCount())
	}
	mismatched.tracker.requireReleaseState(t, fixture.owners[0].AuthorityRef(), 0, nil)
	mismatched.tracker.requireReleaseState(t, fixture.owners[1].AuthorityRef(), 1, values.PublicInvalidContext)
	if err := mismatched.read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	mismatched.tracker.requireReleased(t, fixture.owners[0].AuthorityRef(), fixture.owners[1].AuthorityRef())
}

// Migrated from the root TestFederationSourceVectorRequiresCurrentInitialCut.
func TestFederationSourceVectorRequiresCurrentInitialCut(t *testing.T) {
	fixture := newActualFixture(t, true, nil)
	request := federationcontract.FederationReadRequest{Boundary: fixture.root, Intent: "query", Purpose: [32]byte{9}}
	attempt, err := fixture.openQuery(t, request, 3, actualReadOptions{afterOpen: func(opened int) {
		if opened == 1 {
			fixture.write(t, 0, 9751, fixture.root, "before-seal")
		}
	}})
	if !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("sealed stale captures", err)
	}
	if attempt.connections.openCount() != 2 {
		t.Fatalf("stale seal opened %d remote sources, want 2", attempt.connections.openCount())
	}
	attempt.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)
}

// Migrated from the root TestFederationSourceVectorMixedPolicyRequiresRecollection.
func TestFederationSourceVectorMixedPolicyRequiresRecollection(t *testing.T) {
	fixture := newActualFixture(t, true, nil)
	request := federationcontract.FederationReadRequest{Boundary: fixture.root, Intent: "query", Purpose: [32]byte{9}}
	mixed, err := fixture.openQuery(t, request, 3, actualReadOptions{alter: func(info *federationcontract.FederationReadPart) {
		info.ReadMode = corecontract.QueryCurrentVerified
	}})
	if !errors.Is(err, federationcollect.ErrMixedRetention) {
		t.Fatal("mixed vector published", err)
	}
	mixed.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)

	request.Mode = corecontract.QueryCurrentVerified
	current, err := fixture.openQuery(t, request, 3, actualReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := collectActualPage(current.read, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range page.SourceGraph.Sources {
		if source.ReadMode != corecontract.QueryCurrentVerified {
			t.Fatal("retained source reused")
		}
	}
	if err := current.read.Retire(context.Background()); err != nil {
		t.Fatal(err)
	}
	current.tracker.requireReleased(t, actualOwnerRefs(fixture, 3)...)
}
