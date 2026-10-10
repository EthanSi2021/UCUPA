package oracle_test

import (
	"errors"
	"math"
	"testing"
	"ucupa/ucupa-core/contract"
	queryoracle "ucupa/ucupa-core/internal/query/oracle"
)

func TestLinearOracleExpiredCutReturnsNoFacts(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(20), "root")
	boundary := oracleCreateAnchor(t, c, oracleID(21), oracleTuple(t, root, 0, 0, 0, 10), "boundary")
	first, err := queryoracle.Scan(c.owner, boundary, boundary, nil, contract.QuerySphere{Radius: 1}, queryoracle.Both)
	if err != nil {
		t.Fatal(err)
	}
	oracleCreateEntity(t, c, oracleID(22), oracleTuple(t, boundary, 0, 0, 0, 1), "mutation")
	stale, err := queryoracle.Scan(c.owner, boundary, boundary, &first.Cut, contract.QuerySphere{Radius: 1}, queryoracle.Both)
	if !errors.Is(err, contract.ErrReadCutExpired) || len(stale.Records) != 0 || !stale.Cut.Equal(contract.CoreReadCut{}) {
		t.Fatalf("stale result %#v err=%v", stale, err)
	}
}

func TestLinearOracleFailureMatrixReturnsNoFacts(t *testing.T) {
	c, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	root := oracleCreateRoot(t, c, oracleID(30), "matrix-root")
	boundary := oracleCreateAnchor(t, c, oracleID(31), oracleTuple(t, root, 0, 0, 0, 100), "matrix-boundary")
	outside := oracleCreateAnchor(t, c, oracleID(32), oracleTuple(t, root, 0, 0, 0, 100), "matrix-outside")
	entity := oracleCreateEntity(t, c, oracleID(33), oracleTuple(t, boundary, 0, 0, 0, 1), "matrix-entity")
	query := contract.QuerySphere{Radius: 1}

	assertFailure := func(name string, result queryoracle.Result, got, want error) {
		t.Helper()
		if !errors.Is(got, want) || len(result.Records) != 0 || !result.Cut.Equal(contract.CoreReadCut{}) {
			t.Fatalf("%s: result=%#v err=%v want=%v", name, result, got, want)
		}
	}
	result, scanErr := queryoracle.Scan(c.owner, "", boundary, nil, query, queryoracle.Both)
	assertFailure("invalid boundary", result, scanErr, contract.ErrInvalid)
	result, scanErr = queryoracle.Scan(c.owner, boundary, boundary, nil, contract.QuerySphere{Center: contract.Vec3{X: math.NaN()}, Radius: 1}, queryoracle.Both)
	assertFailure("invalid center", result, scanErr, contract.ErrInvalid)
	result, scanErr = queryoracle.Scan(c.owner, boundary, boundary, nil, query, queryoracle.Selector(0))
	assertFailure("invalid selector", result, scanErr, contract.ErrInvalid)
	result, scanErr = queryoracle.Scan(c.owner, oracleID(999), boundary, nil, query, queryoracle.Both)
	assertFailure("missing boundary", result, scanErr, contract.ErrNotFound)
	result, scanErr = queryoracle.Scan(c.owner, boundary, oracleID(998), nil, query, queryoracle.Both)
	assertFailure("missing basis", result, scanErr, contract.ErrNotFound)
	result, scanErr = queryoracle.Scan(c.owner, entity, boundary, nil, query, queryoracle.Both)
	assertFailure("Entity boundary", result, scanErr, contract.ErrConflict)
	result, scanErr = queryoracle.Scan(c.owner, boundary, entity, nil, query, queryoracle.Both)
	assertFailure("Entity basis", result, scanErr, contract.ErrConflict)
	result, scanErr = queryoracle.Scan(c.owner, boundary, outside, nil, query, queryoracle.Both)
	assertFailure("basis outside closure", result, scanErr, contract.ErrConflict)

	foreign, err := newOracleFixture()
	if err != nil {
		t.Fatal(err)
	}
	foreignRoot := oracleCreateRoot(t, foreign, oracleID(34), "matrix-foreign-root")
	foreignResult, err := queryoracle.Scan(foreign.owner, foreignRoot, foreignRoot, nil, query, queryoracle.Both)
	if err != nil {
		t.Fatal(err)
	}
	result, scanErr = queryoracle.Scan(c.owner, boundary, boundary, &foreignResult.Cut, query, queryoracle.Both)
	assertFailure("foreign cut", result, scanErr, contract.ErrInvalid)
}
