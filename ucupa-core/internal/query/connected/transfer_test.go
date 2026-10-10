package connected

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/query/exchange"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestConnectedTransferRetainsGapAndRejectsOldContext(t *testing.T) {
	f := newReviewFixtureWithParentStore(t, true, 128, filepath.Join(t.TempDir(), "parent.db"))
	id := reviewID(9150)
	created, err := f.parent.elements.CreateEntity(reviewActor, id, reviewTuple(t, f.root, 30, 1), "departing-entity")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	q := reviewRootQuery(f.root)
	first, err := f.parent.connected.QueryRegionPath(ctx, f.callers, q, nil, 1, nil)
	if err != nil || first.Continuation == nil {
		t.Fatalf("initial connected query: %#v %v", first, err)
	}
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "connected-source-fence", Actor: reviewActor, Source: f.parent.owner.AuthorityRef(),
		Target: values.FactAuthorityRef(reviewID(9400)), Boundary: id, Expected: created.Record.Entity.Revision,
		Final: reviewTuple(t, reviewID(9500), 0, 1), SourceAddress: "local:guard-a", TargetAddress: "local:transfer-target"}
	f.parent.owner.Lock()
	record, err := f.parent.owner.BeginTransferIntentLocked(i, 4096)
	if err == nil {
		_, err = f.parent.owner.FenceTransferSourceLocked(reviewActor, i.Operation, record.Revision, transferfact.ParentHold{})
	}
	f.parent.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if page, err := f.parent.connected.QueryRegionPath(ctx, f.callers, q, nil, 1, first.Continuation); !errors.Is(err, contract.ErrReadCutExpired) || len(page.Matches) != 0 {
		t.Fatalf("old connected context crossed fence: %#v %v", page, err)
	}
	page, err := f.parent.connected.QueryRegionPath(ctx, f.callers, q, nil, 256, nil)
	if err != nil || page.Status != contract.QueryPartial || page.Gap&contract.QueryUnresolvedCoverage == 0 {
		t.Fatalf("composite dropped source gap: %#v %v", page, err)
	}
	carrierFound := false
	for _, match := range page.Matches {
		if match.ElementID == id {
			t.Fatal("inactive source member disclosed")
		}
		carrierFound = carrierFound || match.Source.Authority == f.carrier.owner.AuthorityRef()
	}
	if !carrierFound {
		t.Fatal("unrelated carrier results discarded")
	}
	if read, err := f.parent.connected.ResolvePath(ctx, f.callers, contract.Path{OriginAnchorID: f.root, Down: []values.ElementID{id}}, nil); !errors.Is(err, contract.ErrUnavailable) || read.Record != nil {
		t.Fatalf("departed path became ordinary absence: %#v %v", read, err)
	}
	direct, err := f.parent.connected.ListDirectPath(ctx, f.callers, contract.Path{OriginAnchorID: f.root}, nil, 256, nil)
	if err != nil || direct.Status != contract.QueryPartial || direct.Gap != contract.QueryUnresolvedCoverage {
		t.Fatalf("direct listing lost source gap: %#v %v", direct, err)
	}
	lease, err := f.parent.queries.Capture(reviewActor, f.root, nil, contract.QueryCurrentVerified)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Finish()
	for _, grant := range []exchange.Grant{
		{Kind: exchange.Query, Scope: f.mount},
		{Kind: exchange.Query, Scope: f.root, ProofOnly: true},
		{Kind: exchange.Resolve, Scope: f.mount},
	} {
		filtered := filterConnectedFacts(lease, grant)
		if members, fences, parents := filtered.Transfers.Counts(); members != 0 || fences != 0 || parents != 0 {
			t.Fatalf("unrelated/private source evidence leaked: %+v %d %d", grant, members, fences)
		}
	}
	filtered := filterConnectedFacts(lease, exchange.Grant{Kind: exchange.Query, Scope: f.root})
	if members, fences, parents := filtered.Transfers.Counts(); members != 1 || fences != 1 || parents != 0 {
		t.Fatalf("authorized source envelope omitted: %d %d", members, fences)
	}
}
