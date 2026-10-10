package instance_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var durableActor = values.PrincipalRef{Realm: "durable-realm", Principal: "durable-writer"}

var instanceContact = corecontract.RealmInitialization{Realm: durableActor.Realm, NextHop: "local:durable-realm"}

type durableAllow struct{}

func (durableAllow) Authorize(values.PrincipalRef, ...values.Requirement) bool { return true }

// instancePolicyContact adapts one fixed bool authorization stub to the Realm
// contact port. Address identity is not a variable here, and view evidence
// stays unavailable exactly as it was before the port existed.
type instancePolicyContact struct {
	policy interface {
		Authorize(values.PrincipalRef, ...values.Requirement) bool
	}
}

func (instancePolicyContact) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (contact instancePolicyContact) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	if contact.policy == nil || contact.policy.Authorize(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (instancePolicyContact) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationUnavailable, values.AuthorizationView{}
}

func (instancePolicyContact) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func (instancePolicyContact) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}

func (instancePolicyContact) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func TestDurableEmptyReopenPreservesInitialCut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	spatial := createDurable(t, path)
	before, err := spatial.Elements().ReadElement(durableActor, durableID(1), nil)
	if err != nil || before.State != corecontract.ExistenceNotFound || before.Record != nil ||
		before.Cut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("initial absence: %#v err=%v", before, err)
	}
	closeDurable(t, spatial)

	reopened := openDurable(t, path)
	defer closeDurable(t, reopened)
	after, err := reopened.Elements().ReadElement(durableActor, durableID(1), nil)
	if err != nil || after.State != corecontract.ExistenceNotFound || !after.Cut.Equal(before.Cut) {
		t.Fatalf("reopened initial absence: %#v err=%v", after, err)
	}
}

func TestDurableReopenReconstructsStructureReadsAndProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "structure.db")
	spatial := createDurable(t, path)
	rootID, leftID, rightID, entityID := durableID(2), durableID(3), durableID(4), durableID(5)
	rootResult, err := spatial.Elements().CreateRoot(durableActor, rootID, "structure-root")
	root := durableCommitted(t, rootResult, err)
	leftResult, err := spatial.Elements().CreateAnchor(
		durableActor, leftID, durableTuple(t, rootID, 10, 0, 0, 20), "structure-left")

	left := durableCommitted(t, leftResult, err)
	rightResult, err := spatial.Elements().CreateAnchor(
		durableActor, rightID, durableTuple(t, rootID, -10, 0, 0, 20), "structure-right")

	durableCommitted(t, rightResult, err)
	entityResult, err := spatial.Elements().CreateEntity(
		durableActor, entityID, durableTuple(t, leftID, 3, 0, 0, 1), "structure-entity")

	entity := durableCommitted(t, entityResult, err)
	beforeList, err := spatial.Elements().ListDirect(durableActor, root.Record.Anchor.ID, 256, nil)
	if err != nil || beforeList.Continuation != nil || len(beforeList.Records) != 2 {
		t.Fatalf("before ListDirect: %#v err=%v", beforeList, err)
	}
	beforeProjection, err := spatial.Elements().ProjectToAncestor(
		durableActor, entity.Record.Entity.ID, root.Record.Anchor.ID, &beforeList.Cut)

	if err != nil || beforeProjection.Pose.Translation != (corecontract.Vec3{X: 13}) ||
		!beforeProjection.SubjectRevision.Equal(entity.Record.Entity.Revision) {
		t.Fatalf("before projection: %#v err=%v", beforeProjection, err)
	}
	if left.Record.Anchor.ID != leftID {
		t.Fatal("left fixture identity changed")
	}
	closeDurable(t, spatial)

	reopened := openDurable(t, path)
	defer closeDurable(t, reopened)
	afterList, err := reopened.Elements().ListDirect(durableActor, rootID, 256, nil)
	if err != nil || afterList.Continuation != nil || !afterList.Cut.Equal(beforeList.Cut) ||
		!reflect.DeepEqual(afterList.Records, beforeList.Records) {
		t.Fatalf("reopened ListDirect: got=%#v err=%v want=%#v", afterList, err, beforeList)
	}
	afterProjection, err := reopened.Elements().ProjectToAncestor(durableActor, entityID, rootID, &afterList.Cut)
	if err != nil || !reflect.DeepEqual(afterProjection, beforeProjection) {
		t.Fatalf("reopened projection: got=%#v err=%v want=%#v", afterProjection, err, beforeProjection)
	}
}

func TestDurableFactsOutcomesAndTombstonesReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facts.db")
	spatial := createDurable(t, path)

	rootID, anchorID, entityID := durableID(10), durableID(11), durableID(12)
	rootResult, err := spatial.Elements().CreateRoot(durableActor, rootID, "create-root")
	root := durableCommitted(t, rootResult, err)
	anchorFinal := durableTuple(t, rootID, 0, 0, 0, 20)
	anchorResult, err := spatial.Elements().CreateAnchor(durableActor, anchorID, anchorFinal, "create-anchor")
	anchor := durableCommitted(t, anchorResult, err)
	entityFinal := durableTuple(t, anchorID, 1, 0, 0, 2)
	entityResult, err := spatial.Elements().CreateEntity(durableActor, entityID, entityFinal, "create-entity")
	entity := durableCommitted(t, entityResult, err)

	movedFinal := durableTuple(t, anchorID, 2, 0, 0, 3)
	movedResult, err := spatial.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, entity.Record.Entity.Revision, "update-entity")

	moved := durableCommitted(t, movedResult, err)
	unchanged, err := spatial.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, moved.Record.Entity.Revision, "unchanged-entity")

	if err != nil || unchanged.Status != corecontract.MutationUnchanged || unchanged.Record == nil ||
		unchanged.Record.Entity == nil || !unchanged.OutcomeCut.Equal(moved.OutcomeCut) ||
		!reflect.DeepEqual(unchanged.Record, moved.Record) {
		t.Fatalf("UNCHANGED: %#v err=%v", unchanged, err)
	}
	unchangedRead, err := spatial.Elements().ReadElement(durableActor, entityID, nil)
	if err != nil || unchangedRead.Record == nil || !unchangedRead.Cut.Equal(moved.OutcomeCut) ||
		!reflect.DeepEqual(unchangedRead.Record, moved.Record) {
		t.Fatalf("UNCHANGED advanced or changed Entity truth: %#v err=%v", unchangedRead, err)
	}

	stale, staleErr := spatial.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, entity.Record.Entity.Revision, "stale-update")

	durableRejected(t, stale, staleErr, corecontract.ErrRevisionConflict)
	foreign, err := core.New(instancePolicyContact{durableAllow{}}, instanceContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	foreignRootResult, err := foreign.Elements().CreateRoot(durableActor, durableID(98), "foreign-root")
	foreignRoot := durableCommitted(t, foreignRootResult, err)
	foreignRevision, foreignRevisionErr := spatial.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, foreignRoot.Record.Anchor.Revision, "foreign-revision")

	durableRejected(t, foreignRevision, foreignRevisionErr, corecontract.ErrRevisionConflict)
	missingFinal := durableTuple(t, anchorID, 0, 0, 0, 1)
	missing, missingErr := spatial.Elements().UpdateEntity(
		durableActor, durableID(99), missingFinal, moved.Record.Entity.Revision, "missing-update")

	durableRejected(t, missing, missingErr, corecontract.ErrNotFound)
	secondRoot, secondRootErr := spatial.Elements().CreateRoot(durableActor, durableID(13), "second-root")
	durableRejected(t, secondRoot, secondRootErr, corecontract.ErrConflict)

	deletedResult, err := spatial.Elements().DeleteEntity(
		durableActor, entityID, moved.Record.Entity.Revision, "delete-entity")

	deleted := durableCommitted(t, deletedResult, err)
	if deleted.TombstoneRevision.Equal(corecontract.Revision{}) || deleted.Record != nil {
		t.Fatalf("delete receipt: %#v", deleted)
	}
	reuse, reuseErr := spatial.Elements().CreateEntity(
		durableActor, entityID, durableTuple(t, anchorID, 0, 0, 0, 1), "reuse-tombstone")

	durableRejected(t, reuse, reuseErr, corecontract.ErrConflict)
	anchorMovedFinal := durableTuple(t, rootID, 4, 0, 0, 25)
	anchorMovedResult, err := spatial.Elements().UpdateAnchor(
		durableActor, anchorID, anchorMovedFinal, anchor.Record.Anchor.Revision, "update-anchor")

	anchorMoved := durableCommitted(t, anchorMovedResult, err)
	anchorUnchanged, err := spatial.Elements().UpdateAnchor(
		durableActor, anchorID, anchorMovedFinal, anchorMoved.Record.Anchor.Revision, "unchanged-anchor")

	if err != nil || anchorUnchanged.Status != corecontract.MutationUnchanged {
		t.Fatalf("Anchor UNCHANGED: %#v err=%v", anchorUnchanged, err)
	}
	anchorDeletedResult, err := spatial.Elements().DeleteAnchor(
		durableActor, anchorID, anchorMoved.Record.Anchor.Revision, "delete-anchor")

	anchorDeleted := durableCommitted(t, anchorDeletedResult, err)
	finalRead, err := spatial.Elements().ReadElement(durableActor, durableID(100), nil)
	if err != nil {
		t.Fatal(err)
	}
	closeDurable(t, spatial)

	reopened := openDurable(t, path)
	defer closeDurable(t, reopened)
	rootRead, err := reopened.Elements().ReadElement(durableActor, rootID, nil)
	if err != nil || rootRead.Record == nil || rootRead.Record.Anchor == nil ||
		!rootRead.Record.Anchor.Revision.Equal(root.Record.Anchor.Revision) {
		t.Fatalf("reopened root: %#v err=%v", rootRead, err)
	}
	anchorRead, err := reopened.Elements().ReadElement(durableActor, anchorID, nil)
	if err != nil || anchorRead.State != corecontract.ExistenceNotFound || anchorRead.Record != nil ||
		!anchorRead.Cut.Equal(finalRead.Cut) {
		t.Fatalf("reopened Anchor: %#v err=%v", anchorRead, err)
	}
	deletedRead, err := reopened.Elements().ReadElement(durableActor, entityID, nil)
	if err != nil || deletedRead.State != corecontract.ExistenceNotFound || deletedRead.Record != nil ||
		!deletedRead.Cut.Equal(finalRead.Cut) {
		t.Fatalf("reopened tombstone: %#v err=%v", deletedRead, err)
	}

	replay, replayErr := reopened.Elements().CreateRoot(durableActor, rootID, "create-root")
	assertReplay(t, root, replay, replayErr)
	replay, replayErr = reopened.Elements().CreateAnchor(durableActor, anchorID, anchorFinal, "create-anchor")
	assertReplay(t, anchor, replay, replayErr)
	replay, replayErr = reopened.Elements().CreateEntity(durableActor, entityID, entityFinal, "create-entity")
	assertReplay(t, entity, replay, replayErr)
	replay, replayErr = reopened.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, entity.Record.Entity.Revision, "update-entity")

	assertReplay(t, moved, replay, replayErr)
	replay, replayErr = reopened.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, moved.Record.Entity.Revision, "unchanged-entity")

	assertReplay(t, unchanged, replay, replayErr)
	replay, replayErr = reopened.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, entity.Record.Entity.Revision, "stale-update")

	assertRejectedReplay(t, stale, corecontract.ErrRevisionConflict, replay, replayErr)
	replay, replayErr = reopened.Elements().UpdateEntity(
		durableActor, entityID, movedFinal, foreignRoot.Record.Anchor.Revision, "foreign-revision")

	assertRejectedReplay(t, foreignRevision, corecontract.ErrRevisionConflict, replay, replayErr)
	replay, replayErr = reopened.Elements().UpdateEntity(
		durableActor, durableID(99), missingFinal, moved.Record.Entity.Revision, "missing-update")

	assertRejectedReplay(t, missing, corecontract.ErrNotFound, replay, replayErr)
	replay, replayErr = reopened.Elements().CreateRoot(
		durableActor, durableID(13), "second-root")

	assertRejectedReplay(t, secondRoot, corecontract.ErrConflict, replay, replayErr)
	replay, replayErr = reopened.Elements().DeleteEntity(
		durableActor, entityID, moved.Record.Entity.Revision, "delete-entity")

	assertReplay(t, deleted, replay, replayErr)
	replay, replayErr = reopened.Elements().CreateEntity(
		durableActor, entityID, durableTuple(t, anchorID, 0, 0, 0, 1), "reuse-tombstone")

	assertRejectedReplay(t, reuse, corecontract.ErrConflict, replay, replayErr)
	replay, replayErr = reopened.Elements().UpdateAnchor(
		durableActor, anchorID, anchorMovedFinal, anchor.Record.Anchor.Revision, "update-anchor")

	assertReplay(t, anchorMoved, replay, replayErr)
	replay, replayErr = reopened.Elements().UpdateAnchor(
		durableActor, anchorID, anchorMovedFinal, anchorMoved.Record.Anchor.Revision, "unchanged-anchor")

	assertReplay(t, anchorUnchanged, replay, replayErr)
	replay, replayErr = reopened.Elements().DeleteAnchor(
		durableActor, anchorID, anchorMoved.Record.Anchor.Revision, "delete-anchor")

	assertReplay(t, anchorDeleted, replay, replayErr)

	freshReuse, freshReuseErr := reopened.Elements().CreateAnchor(
		durableActor, entityID, durableTuple(t, rootID, 0, 0, 0, 1), "fresh-reuse-entity-as-anchor")

	durableRejected(t, freshReuse, freshReuseErr, corecontract.ErrConflict)
	freshAnchorReuse, freshAnchorReuseErr := reopened.Elements().CreateEntity(
		durableActor, anchorID, durableTuple(t, rootID, 0, 0, 0, 1), "fresh-reuse-anchor-as-entity")

	durableRejected(t, freshAnchorReuse, freshAnchorReuseErr, corecontract.ErrConflict)

	different, err := reopened.Elements().CreateRoot(durableActor, durableID(14), "create-root")
	if !errors.Is(err, corecontract.ErrOperationConflict) || different != (corecontract.MutationResult{}) {
		t.Fatalf("different binding: %#v err=%v", different, err)
	}
}

func TestDurableCanonicalBitsAndOpaqueOperationBytesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bits.db")
	spatial := createDurable(t, path)
	rawActor := values.PrincipalRef{
		Realm:     values.RealmID(string([]byte{0xff, 'r'})),
		Principal: values.PrincipalID(string([]byte{0x80, 'p'})),
	}
	rootID := durableID(20)
	invalid, invalidErr := spatial.Elements().CreateRoot(rawActor, rootID, "")
	if !errors.Is(invalidErr, corecontract.ErrInvalid) || invalid != (corecontract.MutationResult{}) {
		t.Fatalf("zero-byte OperationID: %#v err=%v", invalid, invalidErr)
	}
	invalid, invalidErr = spatial.Elements().CreateRoot(rawActor, rootID, corecontract.OperationID(strings.Repeat("x", 129)))
	if !errors.Is(invalidErr, corecontract.ErrInvalid) || invalid != (corecontract.MutationResult{}) {
		t.Fatalf("129-byte OperationID: %#v err=%v", invalid, invalidErr)
	}
	rawOperationID := corecontract.OperationID(string([]byte{0xff}))
	rootResult, rootErr := spatial.Elements().CreateRoot(rawActor, rootID, rawOperationID)
	root := durableCommitted(t, rootResult, rootErr)
	maxOperationID := corecontract.OperationID(strings.Repeat("m", 128))
	maxResult, maxErr := spatial.Elements().CreateRoot(rawActor, durableID(22), maxOperationID)
	durableRejected(t, maxResult, maxErr, corecontract.ErrConflict)

	rotation, err := corecontract.NewRotation(2, 1, -0.5, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{
		X: math.MaxFloat64, Y: math.SmallestNonzeroFloat64, Z: -math.SmallestNonzeroFloat64,
	}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := corecontract.NewBoundedSphere(math.SmallestNonzeroFloat64)
	if err != nil {
		t.Fatal(err)
	}
	entityID := durableID(21)
	createdResult, err := spatial.Elements().CreateEntity(
		rawActor, entityID,
		corecontract.SpatialTuple{Parent: root.Record.Anchor.ID, LocalPose: pose, Range: sphere},
		"create-extreme-entity")

	created := durableCommitted(t, createdResult, err)
	closeDurable(t, spatial)

	reopened := openDurable(t, path)
	defer closeDurable(t, reopened)
	replay, replayErr := reopened.Elements().CreateRoot(rawActor, root.Record.Anchor.ID, rawOperationID)
	assertReplay(t, root, replay, replayErr)
	maxReplay, maxReplayErr := reopened.Elements().CreateRoot(rawActor, durableID(22), maxOperationID)
	assertRejectedReplay(t, maxResult, corecontract.ErrConflict, maxReplay, maxReplayErr)
	differentActor, differentActorErr := reopened.Elements().CreateRoot(durableActor, rootID, rawOperationID)
	if !errors.Is(differentActorErr, corecontract.ErrOperationConflict) ||
		differentActor != (corecontract.MutationResult{}) {
		t.Fatalf("opaque actor binding conflict: %#v err=%v", differentActor, differentActorErr)
	}
	read, err := reopened.Elements().ReadElement(rawActor, entityID, nil)
	if err != nil || read.Record == nil || read.Record.Entity == nil {
		t.Fatalf("read extreme Entity: %#v err=%v", read, err)
	}
	got := read.Record.Entity
	if !got.Revision.Equal(created.Record.Entity.Revision) ||
		math.Float64bits(got.LocalPose.Translation.X) != math.Float64bits(pose.Translation.X) ||
		math.Float64bits(got.LocalPose.Translation.Y) != math.Float64bits(pose.Translation.Y) ||
		math.Float64bits(got.LocalPose.Translation.Z) != math.Float64bits(pose.Translation.Z) ||
		got.LocalPose.Rotation.Pivot() != pose.Rotation.Pivot() ||
		got.LocalPose.Rotation.Ratios() != pose.Rotation.Ratios() ||
		math.Float64bits(got.Range.Radius()) != math.Float64bits(sphere.Radius()) {
		t.Fatalf("canonical bits changed: got=%#v want=%#v", got, pose)
	}
}

func createDurable(t *testing.T, path string) *core.Core {
	t.Helper()
	spatial, err := core.CreateDurable(instancePolicyContact{durableAllow{}}, instanceContact, path, corecontract.Config{})
	if err != nil {
		t.Fatalf("CreateDurable: %v", err)
	}
	return spatial
}

func openDurable(t *testing.T, path string) *core.Core {
	t.Helper()
	spatial, err := core.OpenDurable(instancePolicyContact{durableAllow{}}, path, corecontract.Config{})
	if err != nil {
		t.Fatalf("OpenDurable: %v", err)
	}
	return spatial
}

func closeDurable(t *testing.T, spatial *core.Core) {
	t.Helper()
	if err := spatial.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func durableID(serial uint64) values.ElementID {
	return values.ElementID(fmt.Sprintf("10000000-0000-4000-8000-%012x", serial))
}

func durableTuple(t *testing.T, parent values.ElementID, x, y, z, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{X: x, Y: y, Z: z}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func durableCommitted(t *testing.T, result corecontract.MutationResult, err error) corecontract.MutationResult {
	t.Helper()
	if err != nil || result.Status != corecontract.MutationCommitted || result.OutcomeCut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("committed outcome: %#v err=%v", result, err)
	}
	return result
}

func durableRejected(t *testing.T, result corecontract.MutationResult, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || result.Status != corecontract.MutationRejected || result.Record != nil ||
		result.OutcomeCut.Equal(corecontract.CoreReadCut{}) {
		t.Fatalf("rejected outcome: %#v err=%v want=%v", result, err, want)
	}
}

func assertReplay(t *testing.T, want corecontract.MutationResult, got corecontract.MutationResult, err error) {
	t.Helper()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("replay: got=%#v err=%v want=%#v", got, err, want)
	}
}

func assertRejectedReplay(
	t *testing.T,
	want corecontract.MutationResult,
	wantErr error,
	got corecontract.MutationResult,
	err error,
) {
	t.Helper()
	if !errors.Is(err, wantErr) || !reflect.DeepEqual(got, want) {
		t.Fatalf("rejected replay: got=%#v err=%v want=%#v/%v", got, err, want, wantErr)
	}
}
