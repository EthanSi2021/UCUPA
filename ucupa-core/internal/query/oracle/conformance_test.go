package oracle_test

import (
	"fmt"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
)

func TestLinearOracleRangeAndCrossBranchSemantics(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(1), "root")
	boundary := oracleCreateAnchor(t, c, oracleID(2), oracleTuple(t, root, 0, 0, 0, 100), "boundary")
	left := oracleCreateAnchor(t, c, oracleID(3), oracleTuple(t, boundary, -20, 0, 0, 20), "left")
	basisParent := oracleCreateAnchor(t, c, oracleID(4), oracleTuple(t, left, 0, 0, 0, 10), "basis-parent")
	basis := oracleCreateAnchor(t, c, oracleID(5), oracleTuple(t, basisParent, 0, 0, 0, 5), "basis")
	right := oracleCreateAnchor(t, c, oracleID(6), oracleTuple(t, boundary, 20, 0, 0, 30), "right")
	empty := oracleCreateAnchor(t, c, oracleID(7), oracleTuple(t, right, 1, 0, 0, 1), "empty")
	large := oracleCreateEntity(t, c, oracleID(8), oracleTuple(t, right, 15, 0, 0, 15), "large")

	query := contract.QuerySphere{Center: contract.Vec3{}, Radius: 40}
	entities, err := queryoracle.Scan(c.owner, boundary, basis, nil, query, queryoracle.Entities)
	if err != nil {
		t.Fatal(err)
	}
	if !oracleHas(entities.Records, large) {
		t.Fatalf("selector truncated traversal or missed center-outside tangent Entity: %#v", entities.Records)
	}
	if oracleHas(entities.Records, empty) {
		t.Fatal("entity selector returned Anchor")
	}

	anchors, err := queryoracle.Scan(c.owner, boundary, basis, nil, query, queryoracle.Anchors)
	if err != nil {
		t.Fatal(err)
	}
	if !oracleHas(anchors.Records, empty) {
		t.Fatalf("empty tangent Anchor missing: %#v", anchors.Records)
	}
	if oracleHas(anchors.Records, large) {
		t.Fatal("anchor selector returned Entity")
	}

	both, err := queryoracle.Scan(c.owner, boundary, basis, nil, query, queryoracle.Both)
	if err != nil || !oracleHas(both.Records, large) || !oracleHas(both.Records, empty) {
		t.Fatalf("both result %#v err=%v", both, err)
	}
	if !both.Cut.Equal(entities.Cut) || !both.Cut.Equal(anchors.Cut) {
		t.Fatal("unchanged oracle reads selected different cuts")
	}
}

func TestLinearOracleMatchesIndependentSameCutCorpus(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(200), "differential-root")
	left := oracleCreateAnchor(t, c, oracleID(201), oracleTuple(t, root, -20, 0, 0, 30), "differential-left")
	basis := oracleCreateAnchor(t, c, oracleID(202), oracleTuple(t, left, 5, 0, 0, 5), "differential-basis")
	leftEntity := oracleCreateEntity(t, c, oracleID(203), oracleTuple(t, left, 8, 0, 0, 2), "differential-left-entity")
	right := oracleCreateAnchor(t, c, oracleID(204), oracleTuple(t, root, 25, 0, 0, 40), "differential-right")
	rightChild := oracleCreateAnchor(t, c, oracleID(205), oracleTuple(t, right, 5, 0, 0, 4), "differential-right-child")
	rightEntity := oracleCreateEntity(t, c, oracleID(206), oracleTuple(t, right, 15, 0, 0, 15), "differential-right-entity")
	far := oracleCreateAnchor(t, c, oracleID(207), oracleTuple(t, root, 100, 0, 0, 1), "differential-far")

	type referenceElement struct {
		id        values.ElementID
		kind      contract.ElementKind
		worldX    int64
		radius    int64
		unbounded bool
	}
	reference := []referenceElement{
		{id: root, kind: contract.ElementAnchor, unbounded: true},
		{id: left, kind: contract.ElementAnchor, worldX: -20, radius: 30},
		{id: basis, kind: contract.ElementAnchor, worldX: -15, radius: 5},
		{id: leftEntity, kind: contract.ElementEntity, worldX: -12, radius: 2},
		{id: right, kind: contract.ElementAnchor, worldX: 25, radius: 40},
		{id: rightChild, kind: contract.ElementAnchor, worldX: 30, radius: 4},
		{id: rightEntity, kind: contract.ElementEntity, worldX: 40, radius: 15},
		{id: far, kind: contract.ElementAnchor, worldX: 100, radius: 1},
	}
	cases := []struct {
		name     string
		centerX  int64
		radius   int64
		selector queryoracle.Selector
	}{
		{name: "point at basis", selector: queryoracle.Both},
		{name: "entities at left occurrence", centerX: 3, selector: queryoracle.Entities},
		{name: "cross branch point", centerX: 60, selector: queryoracle.Both},
		{name: "far tangent", centerX: 100, radius: 14, selector: queryoracle.Anchors},
	}

	var commonCut contract.CoreReadCut
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, scanErr := queryoracle.Scan(c.owner,
				root, basis, nil,
				contract.QuerySphere{Center: contract.Vec3{X: float64(test.centerX)}, Radius: float64(test.radius)},
				test.selector,
			)
			if scanErr != nil {
				t.Fatal(scanErr)
			}
			if commonCut.Equal(contract.CoreReadCut{}) {
				commonCut = result.Cut
			} else if !result.Cut.Equal(commonCut) {
				t.Fatal("unchanged differential scans used different cuts")
			}

			want := make([]values.ElementID, 0)
			for _, element := range reference {
				if test.selector == queryoracle.Anchors && element.kind != contract.ElementAnchor ||
					test.selector == queryoracle.Entities && element.kind != contract.ElementEntity {
					continue
				}
				delta := element.worldX - (-15) - test.centerX
				if delta < 0 {
					delta = -delta
				}
				if element.unbounded || delta <= element.radius+test.radius {
					want = append(want, element.id)
				}
			}
			elementfact.SortIDs(want)
			got := make([]values.ElementID, 0, len(result.Records))
			for _, record := range result.Records {
				if record.Kind == contract.ElementAnchor {
					got = append(got, record.Anchor.ID)
				} else {
					got = append(got, record.Entity.ID)
				}
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("oracle differs from independent corpus: got=%v want=%v", got, want)
			}
		})
	}
}

func TestLinearOracleOrdersByCanonicalUUIDBytes(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	rootID := values.ElementID("ffffffff-ffff-4fff-bfff-ffffffffffff")
	firstID := values.ElementID("00000000-0000-4000-8000-000000000001")
	secondID := values.ElementID("00000000-0000-4000-8000-0000000000ff")
	thirdID := values.ElementID("10000000-0000-4000-9000-000000000000")
	fourthID := values.ElementID("7fffffff-ffff-4fff-afff-ffffffffffff")
	root := oracleCreateRoot(t, c, rootID, "order-root")

	// Deliberately create both kinds in an order unrelated to canonical bytes.
	oracleCreateEntity(t, c, fourthID, oracleTuple(t, root, 0, 0, 0, 0), "order-fourth")
	oracleCreateAnchor(t, c, secondID, oracleTuple(t, root, 0, 0, 0, 0), "order-second")
	oracleCreateEntity(t, c, thirdID, oracleTuple(t, root, 0, 0, 0, 0), "order-third")
	oracleCreateAnchor(t, c, firstID, oracleTuple(t, root, 0, 0, 0, 0), "order-first")

	result, err := queryoracle.Scan(c.owner, root, root, nil, contract.QuerySphere{}, queryoracle.Both)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]values.ElementID, 0, len(result.Records))
	for _, record := range result.Records {
		switch record.Kind {
		case contract.ElementAnchor:
			got = append(got, record.Anchor.ID)
		case contract.ElementEntity:
			got = append(got, record.Entity.ID)
		default:
			t.Fatalf("unknown result kind: %#v", record)
		}
	}
	// This expected order is handwritten from the 16 RFC UUID bytes. It must
	// not use sortElementIDs or another production ordering helper.
	want := []values.ElementID{firstID, secondID, thirdID, fourthID, rootID}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("canonical UUID-byte order: got=%v want=%v", got, want)
	}
}
