package commit_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

var writeActor = values.PrincipalRef{Realm: "write-test", Principal: "operator"}

type writeAllowed struct{}

func authorizationViewDigest(subject values.PrincipalRef, requirements []values.Requirement) values.AuthorizationView {
	return values.AuthorizationView(sha256.Sum256([]byte(fmt.Sprintf("view:%v:%v", subject, requirements))))
}

func authorizationDenialDigest(subject values.PrincipalRef, requirement values.Requirement) values.AuthorizationDenialView {
	return values.AuthorizationDenialView(sha256.Sum256([]byte(fmt.Sprintf("denial:%v:%v", subject, requirement))))
}

func (writeAllowed) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (writeAllowed) AuthorizeView(ctx context.Context, caller values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationValid, authorizationViewDigest(caller.Subject, requirements)
}

func (writeAllowed) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	if authorizationViewDigest(caller.Subject, requirements) != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (writeAllowed) CaptureDenialView(ctx context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (writeAllowed) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := (writeAllowed{}).CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func TestElementOperationRetainsCommitAfterStoreFailure(t *testing.T) {
	for _, after := range []bool{false, true} {
		name := "rollback"
		if after {
			name = "lost-response"
		}
		t.Run(name, func(t *testing.T) {
			authority, err := values.NewFactAuthorityRef()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "core.db")
			owner, err := commit.CreateDurable(path, authority, [32]byte{9}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.CloseStore() })
			operations := element.New(owner, writeAllowed{})
			owner.RLock()
			before := owner.CurrentCutLocked()
			owner.RUnlock()
			failure := errors.New("operation Store failure")
			commit.InjectWriteFailureForTest(owner, after, failure)
			id := values.ElementID("10000000-0000-4000-8000-000000000001")
			result, err := operations.CreateRoot(writeActor, id, "root")
			if !errors.Is(err, contract.ErrStore) || !errors.Is(err, failure) || result != (contract.MutationResult{}) {
				t.Fatalf("failed operation changed response: %#v %v", result, err)
			}
			owner.RLock()
			current := owner.CurrentCutLocked()
			_, present := owner.FactsLocked().Elements.Anchor(id)
			owner.RUnlock()
			if present != after || current.Equal(before) == after {
				t.Fatalf("memory disagrees with the durable side: present=%v unchanged=%v", present, current.Equal(before))
			}
			if !commit.MatchesStoreForTest(owner) {
				t.Fatal("full facts or retained history differ from Store")
			}
			retried, err := operations.CreateRoot(writeActor, id, "root")
			if err != nil || retried.Status != contract.MutationCommitted || retried.Record == nil {
				t.Fatalf("exact retry: %#v %v", retried, err)
			}
			if after && !retried.OutcomeCut.Equal(current) {
				t.Fatal("retry changed original committed cut")
			}
			if _, err := operations.CreateRoot(writeActor, "10000000-0000-4000-8000-000000000002", "root"); !errors.Is(err, contract.ErrOperationConflict) {
				t.Fatalf("reused operation binding: %v", err)
			}
			radius, err := contract.NewBoundedSphere(100)
			if err != nil {
				t.Fatal(err)
			}
			rotation, err := contract.NewRotation(1, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			anchor, err := operations.CreateAnchor(writeActor, "10000000-0000-4000-8000-000000000003",
				contract.SpatialTuple{Parent: id, LocalPose: contract.Pose{Rotation: rotation}, Range: radius}, "next-anchor")
			if err != nil || anchor.Status != contract.MutationCommitted {
				t.Fatalf("subsequent write: %#v %v", anchor, err)
			}
			if !commit.MatchesStoreForTest(owner) {
				t.Fatal("subsequent write lost authoritative state")
			}
			if err := owner.CloseStore(); err != nil {
				t.Fatal(err)
			}
			reopened, err := commit.OpenDurable(path, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.CloseStore()
			replay, err := element.New(reopened, writeAllowed{}).CreateRoot(writeActor, id, "root")
			if err != nil || replay.Record == nil || !replay.OutcomeCut.Equal(retried.OutcomeCut) ||
				!replay.Record.Anchor.Revision.Equal(retried.Record.Anchor.Revision) {
				t.Fatalf("reopen changed exact retained outcome: %#v %v", replay, err)
			}
			if !commit.MatchesStoreForTest(reopened) {
				t.Fatal("replay changed durable history")
			}
		})
	}
}

func TestElementOperationPoisonsUnreconcilableStore(t *testing.T) {
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := commit.CreateDurable(filepath.Join(t.TempDir(), "core.db"), authority, [32]byte{8}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	operations := element.New(owner, writeAllowed{})
	for _, id := range []contract.OperationID{"first", "later"} {
		result, err := operations.CreateRoot(writeActor, "10000000-0000-4000-8000-000000000001", id)
		if !errors.Is(err, contract.ErrStore) || result != (contract.MutationResult{}) {
			t.Fatalf("write to poisoned Store: %#v %v", result, err)
		}
		owner.RLock()
		available := owner.AvailableLocked()
		owner.RUnlock()
		if available {
			t.Fatal("unreconcilable owner remained available")
		}
	}
}

func storeElementID(n int) values.ElementID {
	return values.ElementID(fmt.Sprintf("11000000-0000-4000-8000-%012d", n))
}

func storeTuple(t *testing.T, parent values.ElementID, x, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	extent, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: extent}
}

func createElementStore(t *testing.T, name string) (string, *commit.Owner, *element.Operations) {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	owner, err := commit.CreateDurable(path, authority, [32]byte{6}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return path, owner, element.New(owner, writeAllowed{})
}

func openElementStore(t *testing.T, path string) (*commit.Owner, *element.Operations) {
	t.Helper()
	owner, err := commit.OpenDurable(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return owner, element.New(owner, writeAllowed{})
}

func TestDurablePreCommitFailureIsAtomic(t *testing.T) {
	path, owner, operations := createElementStore(t, "precommit.db")
	id := storeElementID(1)
	before, err := operations.ReadElement(writeActor, id, nil)
	if err != nil || before.State != contract.ExistenceNotFound {
		t.Fatalf("initial: %#v %v", before, err)
	}
	injected := errors.New("injected precommit failure")
	commit.InjectWriteFailureForTest(owner, false, injected)
	result, err := operations.CreateRoot(writeActor, id, "precommit-root")
	if !errors.Is(err, contract.ErrStore) || !errors.Is(err, injected) || result != (contract.MutationResult{}) {
		t.Fatalf("failed: %#v %v", result, err)
	}
	after, err := operations.ReadElement(writeActor, id, nil)
	if err != nil || after.State != contract.ExistenceNotFound || !after.Cut.Equal(before.Cut) {
		t.Fatalf("memory rollback: %#v %v", after, err)
	}
	if !commit.MatchesStoreForTest(owner) {
		t.Fatal("rollback differs from Store")
	}
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	reopened, operations := openElementStore(t, path)
	reopenedRead, err := operations.ReadElement(writeActor, id, nil)
	if err != nil || reopenedRead.State != contract.ExistenceNotFound || !reopenedRead.Cut.Equal(before.Cut) {
		t.Fatalf("durable rollback: %#v %v", reopenedRead, err)
	}
	retry, err := operations.CreateRoot(writeActor, id, "precommit-root")
	if err != nil || retry.Status != contract.MutationCommitted || retry.Record == nil {
		t.Fatalf("retry: %#v %v", retry, err)
	}
	if err := reopened.CloseStore(); err != nil {
		t.Fatal(err)
	}
	final, operations := openElementStore(t, path)
	defer final.CloseStore()
	replay, err := operations.CreateRoot(writeActor, id, "precommit-root")
	if err != nil || replay.Status != contract.MutationCommitted || !replay.OutcomeCut.Equal(retry.OutcomeCut) || replay.Record == nil || !replay.Record.Anchor.Revision.Equal(retry.Record.Anchor.Revision) {
		t.Fatalf("replay: %#v %v", replay, err)
	}
}

func TestDurableLostResponsePublishesAndExactRetryReplays(t *testing.T) {
	path, owner, operations := createElementStore(t, "lost-response.db")
	rootID, anchorID := storeElementID(10), storeElementID(11)
	injected := errors.New("injected lost response")
	commit.InjectWriteFailureForTest(owner, true, injected)
	result, err := operations.CreateRoot(writeActor, rootID, "lost-root")
	if !errors.Is(err, contract.ErrStore) || !errors.Is(err, injected) || result != (contract.MutationResult{}) {
		t.Fatalf("lost: %#v %v", result, err)
	}
	read, err := operations.ReadElement(writeActor, rootID, nil)
	if err != nil || read.State != contract.ExistencePresent || read.Record == nil || read.Record.Anchor == nil {
		t.Fatalf("not published: %#v %v", read, err)
	}
	anchor, err := operations.CreateAnchor(writeActor, anchorID, storeTuple(t, rootID, 1, 2), "after-lost-root")
	if err != nil || anchor.Status != contract.MutationCommitted || anchor.Record == nil {
		t.Fatalf("successor: %#v %v", anchor, err)
	}
	replay, err := operations.CreateRoot(writeActor, rootID, "lost-root")
	if err != nil || replay.Status != contract.MutationCommitted || replay.Record == nil || !replay.OutcomeCut.Equal(read.Cut) || !replay.Record.Anchor.Revision.Equal(read.Record.Anchor.Revision) {
		t.Fatalf("retry: %#v %v", replay, err)
	}
	conflict, err := operations.CreateRoot(writeActor, storeElementID(12), "lost-root")
	if !errors.Is(err, contract.ErrOperationConflict) || conflict != (contract.MutationResult{}) {
		t.Fatalf("rebind: %#v %v", conflict, err)
	}
	if !commit.MatchesStoreForTest(owner) {
		t.Fatal("memory differs from Store")
	}
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	reopened, operations := openElementStore(t, path)
	defer reopened.CloseStore()
	for _, id := range []values.ElementID{rootID, anchorID} {
		got, err := operations.ReadElement(writeActor, id, nil)
		if err != nil || got.State != contract.ExistencePresent || got.Record == nil {
			t.Fatalf("reopened %s: %#v %v", id, got, err)
		}
	}
	reopenedReplay, err := operations.CreateRoot(writeActor, rootID, "lost-root")
	if err != nil || reopenedReplay.Status != contract.MutationCommitted || !reopenedReplay.OutcomeCut.Equal(replay.OutcomeCut) || reopenedReplay.Record == nil || !reopenedReplay.Record.Anchor.Revision.Equal(replay.Record.Anchor.Revision) {
		t.Fatalf("reopened retry: %#v %v", reopenedReplay, err)
	}
}

func TestDurableUnreconcilablePersistFailurePoisonsInstance(t *testing.T) {
	path, owner, operations := createElementStore(t, "poison.db")
	id := storeElementID(15)
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	result, err := operations.CreateRoot(writeActor, id, "poison-root")
	if !errors.Is(err, contract.ErrStore) || result != (contract.MutationResult{}) {
		t.Fatalf("persist: %#v %v", result, err)
	}
	read, err := operations.ReadElement(writeActor, id, nil)
	if !errors.Is(err, contract.ErrStore) || read != (contract.ElementRead{}) {
		t.Fatalf("read: %#v %v", read, err)
	}
	retry, err := operations.CreateRoot(writeActor, id, "poison-root")
	if !errors.Is(err, contract.ErrStore) || retry != (contract.MutationResult{}) {
		t.Fatalf("retry: %#v %v", retry, err)
	}
	owner.RLock()
	available := owner.AvailableLocked()
	owner.RUnlock()
	if available {
		t.Fatal("owner not poisoned")
	}
	reopened, operations := openElementStore(t, path)
	defer reopened.CloseStore()
	committed, err := operations.CreateRoot(writeActor, id, "poison-root")
	if err != nil || committed.Status != contract.MutationCommitted || committed.Record == nil {
		t.Fatalf("authoritative reopen: %#v %v", committed, err)
	}
}

func TestDurablePreCommitFailurePreservesNonemptySnapshot(t *testing.T) {
	path, owner, operations := createElementStore(t, "nonempty.db")
	rootID, entityID := storeElementID(30), storeElementID(31)
	root, err := operations.CreateRoot(writeActor, rootID, "nonempty-root")
	if err != nil || root.Status != contract.MutationCommitted {
		t.Fatal(err)
	}
	initial := storeTuple(t, rootID, 1, 2)
	entity, err := operations.CreateEntity(writeActor, entityID, initial, "nonempty-entity")
	if err != nil || entity.Status != contract.MutationCommitted || entity.Record == nil {
		t.Fatalf("entity: %#v %v", entity, err)
	}
	before, err := operations.ReadElement(writeActor, entityID, nil)
	if err != nil || before.Record == nil {
		t.Fatal(err)
	}
	injected := errors.New("injected nonempty precommit failure")
	commit.InjectWriteFailureForTest(owner, false, injected)
	updated := storeTuple(t, rootID, 2, 3)
	failed, err := operations.UpdateEntity(writeActor, entityID, updated, entity.Record.Entity.Revision, "failed-update")
	if !errors.Is(err, contract.ErrStore) || !errors.Is(err, injected) || failed != (contract.MutationResult{}) {
		t.Fatalf("failed: %#v %v", failed, err)
	}
	after, err := operations.ReadElement(writeActor, entityID, nil)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("memory changed: %#v %v", after, err)
	}
	rootReplay, err := operations.CreateRoot(writeActor, rootID, "nonempty-root")
	if err != nil || !reflect.DeepEqual(rootReplay, root) {
		t.Fatalf("prior result: %#v %v", rootReplay, err)
	}
	if !commit.MatchesStoreForTest(owner) {
		t.Fatal("nonempty rollback differs from Store")
	}
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	reopened, operations := openElementStore(t, path)
	reopenedRead, err := operations.ReadElement(writeActor, entityID, nil)
	if err != nil || !reflect.DeepEqual(reopenedRead, before) {
		t.Fatalf("durable changed: %#v %v", reopenedRead, err)
	}
	retry, err := operations.UpdateEntity(writeActor, entityID, updated, entity.Record.Entity.Revision, "failed-update")
	if err != nil || retry.Status != contract.MutationCommitted || retry.Record == nil {
		t.Fatalf("retry: %#v %v", retry, err)
	}
	exact, err := operations.UpdateEntity(writeActor, entityID, updated, entity.Record.Entity.Revision, "failed-update")
	if err != nil || !reflect.DeepEqual(exact, retry) {
		t.Fatalf("exact: %#v %v", exact, err)
	}
	if err := reopened.CloseStore(); err != nil {
		t.Fatal(err)
	}
}

func TestDurableLostResponsePublishesDecisionOnlyState(t *testing.T) {
	path, owner, operations := createElementStore(t, "decision.db")
	rootID, entityID := storeElementID(40), storeElementID(41)
	root, err := operations.CreateRoot(writeActor, rootID, "decision-root")
	if err != nil || root.Status != contract.MutationCommitted {
		t.Fatal(err)
	}
	final := storeTuple(t, rootID, 1, 2)
	entity, err := operations.CreateEntity(writeActor, entityID, final, "decision-entity")
	if err != nil || entity.Status != contract.MutationCommitted || entity.Record == nil {
		t.Fatalf("entity: %#v %v", entity, err)
	}
	commit.InjectWriteFailureForTest(owner, true, errors.New("lost UNCHANGED response"))
	lost, err := operations.UpdateEntity(writeActor, entityID, final, entity.Record.Entity.Revision, "lost-unchanged")
	if !errors.Is(err, contract.ErrStore) || lost != (contract.MutationResult{}) {
		t.Fatalf("lost unchanged: %#v %v", lost, err)
	}
	conflict, err := operations.UpdateEntity(writeActor, entityID, storeTuple(t, rootID, 2, 2), entity.Record.Entity.Revision, "lost-unchanged")
	if !errors.Is(err, contract.ErrOperationConflict) || conflict != (contract.MutationResult{}) {
		t.Fatalf("binding: %#v %v", conflict, err)
	}
	unchanged, err := operations.UpdateEntity(writeActor, entityID, final, entity.Record.Entity.Revision, "lost-unchanged")
	if err != nil || unchanged.Status != contract.MutationUnchanged {
		t.Fatalf("retry unchanged: %#v %v", unchanged, err)
	}
	commit.InjectWriteFailureForTest(owner, true, errors.New("lost rejected response"))
	lostRejected, err := operations.CreateRoot(writeActor, storeElementID(42), "lost-rejected")
	if !errors.Is(err, contract.ErrStore) || lostRejected != (contract.MutationResult{}) {
		t.Fatalf("lost rejected: %#v %v", lostRejected, err)
	}
	conflict, err = operations.CreateRoot(writeActor, storeElementID(43), "lost-rejected")
	if !errors.Is(err, contract.ErrOperationConflict) || conflict != (contract.MutationResult{}) {
		t.Fatalf("rejected binding: %#v %v", conflict, err)
	}
	rejected, err := operations.CreateRoot(writeActor, storeElementID(42), "lost-rejected")
	if !errors.Is(err, contract.ErrConflict) || rejected.Status != contract.MutationRejected {
		t.Fatalf("retry rejected: %#v %v", rejected, err)
	}
	if !commit.MatchesStoreForTest(owner) {
		t.Fatal("decision-only state differs from Store")
	}
	if err := owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	reopened, operations := openElementStore(t, path)
	defer reopened.CloseStore()
	unchangedReplay, err := operations.UpdateEntity(writeActor, entityID, final, entity.Record.Entity.Revision, "lost-unchanged")
	if err != nil || !reflect.DeepEqual(unchangedReplay, unchanged) {
		t.Fatalf("reopened unchanged: %#v %v", unchangedReplay, err)
	}
	rejectedReplay, err := operations.CreateRoot(writeActor, storeElementID(42), "lost-rejected")
	if !errors.Is(err, contract.ErrConflict) || !reflect.DeepEqual(rejectedReplay, rejected) {
		t.Fatalf("reopened rejected: %#v %v", rejectedReplay, err)
	}
}
