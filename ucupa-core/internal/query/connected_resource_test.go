package query_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	querypkg "ucupa/ucupa-core/internal/query"
)

func captureRootQuery(root values.ElementID) contract.PathRegionQuery {
	path := contract.Path{OriginAnchorID: root}
	return contract.PathRegionQuery{StartPath: path, BasisPath: path, Sphere: contract.QuerySphere{Radius: 100}, Selector: contract.QueryBoth}
}

// The connected owner separately checks its bounded metadata. This half of the
// original scenario checks the actual local query owners, not copied counters.
func TestConnectedReviewCurrentContextsRetainNoQueryFacts(t *testing.T) {
	f := newCaptureFixture(t, false, 2)
	query := captureRootQuery(f.root)
	var oldest contract.ConnectedCut
	for i := 0; i < 7; i++ {
		page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, nil, 256, nil)
		if err != nil || page.Status != contract.QueryComplete || page.ReadMode != contract.QueryCurrentVerified {
			t.Fatalf("query %d: %#v %v", i, page, err)
		}
		if i == 0 {
			oldest = page.Cut
		}
		for _, owner := range []*captureOwner{f.parent, f.carrier} {
			counts := querypkg.ReadResourceCountsForTest(owner.queries)
			if counts.Views != 0 || counts.Handles != 0 || counts.Active != 0 || counts.Bytes != 0 {
				t.Fatalf("CurrentVerified retained owner facts: %#v", counts)
			}
		}
	}
	page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, &oldest, 256, nil)
	if !errors.Is(err, contract.ErrReadCutExpired) || !reflect.DeepEqual(page, contract.ConnectedQueryPage{}) {
		t.Fatalf("evicted context silently recaptured latest: %#v %v", page, err)
	}
}

func TestConnectedReviewRequiredBasisFailureCannotBecomePartial(t *testing.T) {
	for _, scenario := range []string{"unreachable", "denied", "caller-missing", "parent-denied-and-unreachable"} {
		t.Run(scenario, func(t *testing.T) {
			f := newCaptureFixture(t, true, 128)
			query := captureRootQuery(f.root)
			query.BasisPath.Down = []values.ElementID{f.mount, f.branch}
			want := contract.ErrDenied
			switch scenario {
			case "unreachable":
				f.transport.offline = true
				want = contract.ErrUnavailable
			case "denied":
				f.carrier.policy.set(func(values.PrincipalRef, ...values.Requirement) bool { return false })
			case "caller-missing":
				delete(f.callers, f.carrier.owner.AuthorityRef())
			case "parent-denied-and-unreachable":
				f.transport.offline = true
				f.parent.policy.set(func(values.PrincipalRef, ...values.Requirement) bool { return false })
			}
			page, err := f.parent.connected.QueryRegionPath(context.Background(), f.callers, query, nil, 256, nil)
			if !errors.Is(err, want) || !reflect.DeepEqual(page, contract.ConnectedQueryPage{}) {
				t.Fatalf("required Basis failure became data or wrong diagnosis: %#v %v; want %v", page, err, want)
			}
			for _, owner := range []*captureOwner{f.parent, f.carrier} {
				counts := querypkg.ReadResourceCountsForTest(owner.queries)
				if counts.Handles != 0 || counts.Active != 0 {
					t.Fatalf("failed Basis capture leaked pins=%d active=%d", counts.Handles, counts.Active)
				}
			}
		})
	}
}
