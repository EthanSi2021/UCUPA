package commit_test

import (
	"bytes"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestQueryRegionDurableReadOnlyAndContinuationLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.db")
	c := assembleQueryFixture(t, createRangeStore(t, path))
	root := queryCreateRoot(t, c, linearOracleID(70000), "query-durable-root")
	firstID := queryCreateEntity(t, c, linearOracleID(70001), queryFixtureTuple(t, root, 0, 0, 0, 0), "query-durable-first")
	secondID := queryCreateEntity(t, c, linearOracleID(70002), queryFixtureTuple(t, root, 0, 0, 0, 0), "query-durable-second")
	query := contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: root, Sphere: contract.QuerySphere{}, Selector: contract.QueryEntities}
	before := commit.CaptureCompleteStateForTest(c.owner)
	beforeFacts := queryFixtureFacts(c.owner)
	beforeBytes := queryInternalStoreBytes(t, path)
	first, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, nil)
	if err != nil || first.Status != contract.QueryPartial || first.Gap != contract.QueryNoGap || first.Continuation == nil ||
		!reflect.DeepEqual(queryInternalIDs(first), []values.ElementID{firstID}) {
		t.Fatalf("first durable page: %#v err=%v", first, err)
	}
	last, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, first.Continuation)
	if err != nil || last.Status != contract.QueryComplete || last.Gap != contract.QueryNoGap || last.Continuation != nil ||
		!reflect.DeepEqual(queryInternalIDs(last), []values.ElementID{secondID}) || !last.Cut.Equal(first.Cut) {
		t.Fatalf("last durable page: %#v err=%v", last, err)
	}
	invalid := query
	invalid.Sphere.Radius = math.NaN()
	failed, err := c.queries.QueryRegion(linearOracleActor, invalid, nil, 1, nil)
	assertQueryInternalError(t, failed, err, contract.ErrInvalid)
	if !commit.MatchesCompleteStateForTest(c.owner, before) || !bytes.Equal(queryInternalStoreBytes(t, path), beforeBytes) {
		t.Fatal("successful or failed query changed durable facts, operations, or file bytes")
	}

	// These durable outcomes add retained operations, but neither changes facts
	// nor expires the already-issued query cut/continuation.
	tuple := queryFixtureTuple(t, root, 0, 0, 0, 0)
	beforeEntity, _ := beforeFacts.Elements.Entity(firstID)
	unchanged, err := c.elements.UpdateEntity(linearOracleActor, firstID, tuple, beforeEntity.Revision, "query-durable-unchanged")
	if err != nil || unchanged.Status != contract.MutationUnchanged || !unchanged.OutcomeCut.Equal(first.Cut) {
		t.Fatalf("UNCHANGED changed the query cut: %#v err=%v", unchanged, err)
	}
	continued, err := c.queries.QueryRegion(linearOracleActor, query, nil, 1, first.Continuation)
	if err != nil || !reflect.DeepEqual(continued, last) {
		t.Fatalf("UNCHANGED expired the cursor: %#v err=%v", continued, err)
	}
	rejected, err := c.elements.CreateEntity(linearOracleActor, firstID, tuple, "query-durable-negative")
	if !errors.Is(err, contract.ErrConflict) || rejected.Status != contract.MutationRejected || !rejected.OutcomeCut.Equal(first.Cut) {
		t.Fatalf("retained rejection changed the query cut: %#v err=%v", rejected, err)
	}
	if delta, equal := commit.QueryOutcomesMatchForTest(c.owner, before); delta != 2 || !equal {
		t.Fatal("non-changing operation fixtures did not retain exactly two outcomes without fact changes")
	}
	afterOutcomes := commit.CaptureCompleteStateForTest(c.owner)
	afterOutcomeBytes := queryInternalStoreBytes(t, path)
	continued, err = c.queries.QueryRegion(linearOracleActor, query, nil, 1, first.Continuation)
	if err != nil || !reflect.DeepEqual(continued, last) {
		t.Fatalf("retained negative expired the cursor: %#v err=%v", continued, err)
	}
	if !commit.MatchesCompleteStateForTest(c.owner, afterOutcomes) || !bytes.Equal(queryInternalStoreBytes(t, path), afterOutcomeBytes) {
		t.Fatal("continuation changed retained outcomes or durable bytes")
	}
	closeQueryFixture(t, c)

	reopened := assembleQueryFixture(t, openRangeStore(t, path))
	defer closeQueryFixture(t, reopened)
	if !commit.MatchesCompleteStateForTest(reopened.owner, afterOutcomes) {
		t.Fatal("reopen changed durable query context")
	}
	reopenedBytes := queryInternalStoreBytes(t, path)
	continued, err = reopened.queries.QueryRegion(linearOracleActor, query, nil, 1, first.Continuation)
	assertQueryInternalError(t, continued, err, contract.ErrReadCutExpired)
	explicit, err := reopened.queries.QueryRegion(linearOracleActor, query, &first.Cut, 1, nil)
	if err != nil || explicit.Cut != first.Cut || !reflect.DeepEqual(explicit.Matches, first.Matches) || explicit.ReadMode != contract.QueryCurrentVerified {
		t.Fatalf("same Store reopen lost explicit current cut: %#v err=%v", explicit, err)
	}
	if !commit.MatchesCompleteStateForTest(reopened.owner, afterOutcomes) || !bytes.Equal(queryInternalStoreBytes(t, path), reopenedBytes) {
		t.Fatal("reopened queries changed durable state")
	}

	otherPath := filepath.Join(t.TempDir(), "other-query.db")
	other := assembleQueryFixture(t, createRangeStore(t, otherPath))
	defer closeQueryFixture(t, other)
	queryCreateRoot(t, other, root, "query-durable-root")
	queryCreateEntity(t, other, firstID, tuple, "query-durable-first")
	queryCreateEntity(t, other, secondID, tuple, "query-durable-second")
	otherBefore, otherBytes := commit.CaptureCompleteStateForTest(other.owner), queryInternalStoreBytes(t, otherPath)
	foreign, err := other.queries.QueryRegion(linearOracleActor, query, nil, 1, first.Continuation)
	assertQueryInternalError(t, foreign, err, contract.ErrInvalid)
	if !commit.MatchesCompleteStateForTest(other.owner, otherBefore) || !bytes.Equal(queryInternalStoreBytes(t, otherPath), otherBytes) {
		t.Fatal("foreign Store continuation failure changed receiving Store")
	}
}
