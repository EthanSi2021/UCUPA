package commit_test

import (
	"testing"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestQueryRegionCurrentCorruptionFailsClosed(t *testing.T) {
	for _, name := range []string{"missing parent", "anchor cycle", "root range", "excluded kind", "outside scope", "unavailable state", "closed"} {
		t.Run(name, func(t *testing.T) {
			c := newQueryFixture(t)
			root := queryCreateRoot(t, c, linearOracleID(60000), "query-corrupt-root")
			anchor := queryCreateAnchor(t, c, linearOracleID(60001), queryFixtureTuple(t, root, 0, 0, 0, 1), "query-corrupt-anchor")
			queryCreateEntity(t, c, linearOracleID(60002), queryFixtureTuple(t, anchor, 0, 0, 0, 0), "query-corrupt-entity")
			query := contract.RegionQuery{StartAnchorID: root, UpDepth: 0, BasisAnchorID: root, Sphere: contract.QuerySphere{}, Selector: contract.QueryBoth}
			if name == "excluded kind" {
				query.Selector = contract.QueryAnchors
			}
			if name == "outside scope" {
				query.StartAnchorID, query.BasisAnchorID = anchor, anchor
			}
			if err := commit.InjectQueryRegionCorruptionForTest(c.owner, name); err != nil {
				t.Fatal(err)
			}
			hadState := queryFixtureAvailable(c.owner)
			var before commit.CompleteStateProofForTest
			if hadState {
				before = commit.CaptureCompleteStateForTest(c.owner)
			}
			page, err := c.queries.QueryRegion(linearOracleActor, query, nil, 256, nil)
			assertQueryInternalError(t, page, err, contract.ErrStore)
			if !hadState {
				if queryFixtureAvailable(c.owner) {
					t.Fatal("failed query restored or replaced unavailable state")
				}
			} else if !commit.MatchesCompleteStateForTest(c.owner, before) {
				t.Fatal("failed query mutated corrupt facts or retained outcomes")
			}
		})
	}
}
