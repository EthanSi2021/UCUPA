package commit_test

import (
	"crypto/rand"
	"errors"
	"os"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	querypkg "ucupa/ucupa-core/internal/query"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
)

// queryFixture assembles the actual read/write operations around one commit
// owner. It is not a second Core facade and exposes no private state.
type queryFixture struct {
	owner    *commit.Owner
	elements *element.Operations
	queries  *querypkg.Operations
}

func newQueryFixture(t *testing.T) *queryFixture {
	t.Helper()
	owner, elements := linearOracleOwner(t)
	return assembleQueryFixture(t, &rangeStoreOwner{owner: owner, elements: elements})
}

func assembleQueryFixture(t *testing.T, spatial *rangeStoreOwner) *queryFixture {
	t.Helper()
	settings, err := querypkg.NormalizeConfig(contract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var run [32]byte
	if _, err := rand.Read(run[:]); err != nil {
		t.Fatal(err)
	}
	fixture := &queryFixture{
		owner: spatial.owner, elements: spatial.elements,
		queries: querypkg.New(spatial.owner, linearOraclePolicy{}, run, settings),
	}
	t.Cleanup(func() { closeQueryFixture(t, fixture) })
	return fixture
}

func closeQueryFixture(t *testing.T, fixture *queryFixture) {
	t.Helper()
	fixture.owner.Lock()
	first := fixture.owner.BeginCloseLocked()
	fixture.owner.Unlock()
	fixture.queries.Close()
	if first {
		if err := fixture.owner.CloseStore(); err != nil {
			t.Fatal(err)
		}
	}
}

func queryFixtureFacts(owner *commit.Owner) commit.Facts {
	owner.RLock()
	defer owner.RUnlock()
	return owner.FactsLocked()
}

func queryFixtureAvailable(owner *commit.Owner) bool {
	owner.RLock()
	defer owner.RUnlock()
	return owner.AvailableLocked()
}

func queryFixtureTuple(t *testing.T, parent values.ElementID, x, y, z, radius float64) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: x, Y: y, Z: z}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: sphere}
}

func queryCreateRoot(t *testing.T, c *queryFixture, id values.ElementID, op contract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.elements.CreateRoot(linearOracleActor, id, op)
	if err != nil || result.Record == nil {
		t.Fatalf("root %#v err=%v", result, err)
	}
	return result.Record.Anchor.ID
}

func queryCreateAnchor(t *testing.T, c *queryFixture, id values.ElementID, tuple contract.SpatialTuple, op contract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.elements.CreateAnchor(linearOracleActor, id, tuple, op)
	if err != nil || result.Record == nil {
		t.Fatalf("anchor %#v err=%v", result, err)
	}
	return result.Record.Anchor.ID
}

func queryCreateEntity(t *testing.T, c *queryFixture, id values.ElementID, tuple contract.SpatialTuple, op contract.OperationID) values.ElementID {
	t.Helper()
	result, err := c.elements.CreateEntity(linearOracleActor, id, tuple, op)
	if err != nil || result.Record == nil {
		t.Fatalf("entity %#v err=%v", result, err)
	}
	return result.Record.Entity.ID
}

func assertQueryMatchesOracle(t *testing.T, c *queryFixture, query contract.RegionQuery, want []values.ElementID) {
	t.Helper()
	var selector queryoracle.Selector
	switch query.Selector {
	case contract.QueryAnchors:
		selector = queryoracle.Anchors
	case contract.QueryEntities:
		selector = queryoracle.Entities
	case contract.QueryBoth:
		selector = queryoracle.Both
	default:
		t.Fatalf("unsupported test selector: %v", query.Selector)
	}
	oracle, err := queryoracle.Scan(c.owner, query.StartAnchorID, query.BasisAnchorID, nil, contract.QuerySphere{Center: query.Sphere.Center, Radius: query.Sphere.Radius}, selector)
	if err != nil {
		t.Fatalf("same-cut oracle: %v", err)
	}
	var matches []contract.QueryMatch
	var continuation *contract.QueryContinuation
	at := &oracle.Cut
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber > len(oracle.Records)+1 {
			t.Fatal("query pagination did not terminate")
		}
		limit := uint16(1 + pageNumber%2)
		page, pageErr := c.queries.QueryRegion(linearOracleActor, query, at, limit, continuation)
		if pageErr != nil || !page.Cut.Equal(oracle.Cut) || page.Gap != contract.QueryNoGap || len(page.Matches) > int(limit) {
			t.Fatalf("complete oracle page %d: %#v err=%v", pageNumber, page, pageErr)
		}
		matches = append(matches, page.Matches...)
		if page.Continuation == nil {
			if page.Status != contract.QueryComplete {
				t.Fatalf("last exact page is not COMPLETE: %#v", page)
			}
			break
		}
		if page.Status != contract.QueryPartial || len(page.Matches) == 0 {
			t.Fatalf("non-progressing or complete intermediate page: %#v", page)
		}
		at, continuation = nil, page.Continuation
	}
	if len(matches) != len(oracle.Records) {
		t.Fatalf("public/oracle count: got=%d want=%d", len(matches), len(oracle.Records))
	}
	var got []values.ElementID
	for i, match := range matches {
		got = append(got, match.ElementID)
		record := oracle.Records[i]
		var id values.ElementID
		var revision contract.Revision
		if record.Kind == contract.ElementAnchor {
			id, revision = record.Anchor.ID, record.Anchor.Revision
		} else {
			id, revision = record.Entity.ID, record.Entity.Revision
		}
		if match.ElementID != id || match.Kind != record.Kind || !match.Revision.Equal(revision) {
			t.Fatalf("match differs from same-cut oracle: got=%#v want=%#v", match, record)
		}
		assertQueryInternalPath(t, queryFixtureFacts(c.owner), query.StartAnchorID, match)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query differs from independent known IDs: got=%v want=%v", got, want)
	}
}

func assertQueryInternalPath(t *testing.T, state commit.Facts, scope values.ElementID, match contract.QueryMatch) {
	t.Helper()
	current := scope
	for _, id := range match.Path {
		var parent values.ElementID
		if anchor, exists := state.Elements.Anchor(id); exists {
			parent = anchor.Parent
		} else if entity, exists := state.Elements.Entity(id); exists {
			parent = entity.Parent
		} else {
			t.Fatalf("path references absent Element %q", id)
		}
		if parent != current {
			t.Fatalf("non-edge path: current=%q next=%q parent=%q", current, id, parent)
		}
		current = id
	}
	if current != match.ElementID || (match.ElementID == scope && len(match.Path) != 0) {
		t.Fatalf("path is not relative to Scope with included terminal: %#v", match)
	}
}

func assertQueryInternalError(t *testing.T, page contract.QueryPage, got, want error) {
	t.Helper()
	if !errors.Is(got, want) || !reflect.DeepEqual(page, contract.QueryPage{}) {
		t.Fatalf("query failure: page=%#v err=%v want=%v", page, got, want)
	}
}

func assertQueryInternalGapPage(t *testing.T, page contract.QueryPage, err error, want []values.ElementID, more bool) {
	t.Helper()
	if err != nil || page.Status != contract.QueryPartial || page.Gap != contract.QueryExactWorkLimit || page.Cut.Equal(contract.CoreReadCut{}) ||
		(page.Continuation != nil) != more || !reflect.DeepEqual(queryInternalIDs(page), want) {
		t.Fatalf("numeric gap page: %#v err=%v wantIDs=%v more=%v", page, err, want, more)
	}
}

func queryInternalIDs(page contract.QueryPage) []values.ElementID {
	var ids []values.ElementID
	for _, match := range page.Matches {
		ids = append(ids, match.ElementID)
	}
	return ids
}

func queryInternalStoreBytes(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
