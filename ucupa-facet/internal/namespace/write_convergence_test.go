package namespace

import (
	"context"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func TestOperationReplayAndBinding(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		first := x.put(t, "put", "one", contract.ExpectAbsent())
		x.source.offline = true
		sourceCalls := x.source.calls.Load()
		replay := x.put(t, "put", "one", contract.ExpectAbsent())
		if replay != first || x.source.calls.Load() != sourceCalls {
			t.Fatal("replay changed or called sources")
		}
		_, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("different"), contract.ExpectAbsent(), "put")
		mustError(t, err, contract.ErrOperationConflict)
		x.auth.deny.Store(true)
		_, err = x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("one"), contract.ExpectAbsent(), "put")
		mustError(t, err, contract.ErrDenied)
		x.auth.deny.Store(false)
		x.source.offline = false
		_, err = x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("two"), contract.ExpectAbsent(), "rejected")
		mustError(t, err, contract.ErrRevisionConflict)
		if _, err = x.f.Delete(context.Background(), testActor, x.key, x.typ, first.Revision, "delete"); err != nil {
			t.Fatal(err)
		}
		_, err = x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("two"), contract.ExpectAbsent(), "rejected")
		mustError(t, err, contract.ErrRevisionConflict)
		x.put(t, "new", "two", contract.ExpectAbsent())
	})
}
func TestAcceptanceAndContactConvergence(t *testing.T) {
	profiles(t, func(t *testing.T, x *fixture) {
		ctx := context.Background()
		changed, err := x.f.UpdateCoreNextHop(testActor, "address", x.key, x.accepted.NextHopRevision, "local:repaired")
		if err != nil {
			t.Fatal(err)
		}
		if changed.Acceptance != x.accepted.Acceptance || changed.Cut.Equal(x.accepted.Cut) {
			t.Fatal("contact changed acceptance")
		}
		x.source.offline = true
		duplicate, err := x.f.AcceptAttachment(ctx, testActor, x.key, "local:ignored", "duplicate")
		if err != nil || duplicate.Status != contract.Unchanged || duplicate.CoreNextHop != "local:repaired" || duplicate.Acceptance != x.accepted.Acceptance {
			t.Fatalf("duplicate %+v %v", duplicate, err)
		}
		replay, err := x.f.AcceptAttachment(ctx, testActor, x.key, "local:core", "accept")
		if err != nil || replay != x.accepted {
			t.Fatal("original accept replay")
		}
		_, err = x.f.AcceptAttachment(ctx, testActor, x.key, "local:other", "accept")
		mustError(t, err, contract.ErrOperationConflict)
		_, err = x.f.UpdateCoreNextHop(testActor, "stale", x.key, x.accepted.NextHopRevision, "local:old")
		mustError(t, err, contract.ErrContactRevisionConflict)
		same, err := x.f.UpdateCoreNextHop(testActor, "same", x.key, changed.NextHopRevision, "local:repaired")
		if err != nil || same.Status != contract.Unchanged || !same.Cut.Equal(changed.Cut) {
			t.Fatal("same address advanced")
		}
		current, err := x.f.ReadAcceptance(testActor, x.key)
		if err != nil || current.CoreNextHop != "local:repaired" || current.Acceptance != x.accepted.Acceptance {
			t.Fatal("replay rewrote address")
		}
	})
}
func TestAcceptProposalAndLineageRejections(t *testing.T) {
	x := newFixture(t, false)
	ctx := context.Background()
	other := testKey(x.key.Facet, x.key.Core, x.key.Element, 5)
	x.source.current = testProof(other, contract.CoreNone, 5)
	_, err := x.f.AcceptAttachment(ctx, testActor, other, "local:core", "cancelled")
	mustError(t, err, contract.ErrProposalNotCurrent)
	x.source.current.Pending = other
	accepted, err := x.f.AcceptAttachment(ctx, testActor, other, "local:core", "new-proposal")
	if err != nil || accepted.Acceptance.Key != other {
		t.Fatal(err)
	}
	another := testKey(x.key.Facet, values.FactAuthorityRef("00000000-0000-4000-8000-00000000ca02"), x.key.Element, 6)
	x.source.current = testProof(another, contract.CoreNone, 6)
	x.source.current.Pending = another
	_, err = x.f.AcceptAttachment(ctx, testActor, another, "local:core", "lineage")
	mustError(t, err, contract.ErrSubjectAuthorityConflict)
	absent := testKey(x.key.Facet, x.key.Core, x.key.Element, 7)
	read, err := x.f.ReadAcceptance(testActor, absent)
	if err != nil || read.Found || !read.Cut.Valid() {
		t.Fatal("absence")
	}
	_, err = x.f.UpdateCoreNextHop(testActor, "missing", absent, x.accepted.NextHopRevision, "local:other")
	mustError(t, err, contract.ErrAcceptanceNotFound)
}
