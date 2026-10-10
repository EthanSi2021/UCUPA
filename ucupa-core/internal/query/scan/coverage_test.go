package scan

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestCoverageKeepsReadSetOrderAndPartialPosition(t *testing.T) {
	elements, mounts := elementfact.New(), mountfact.New()
	root, firstID, lastID := scanID(701), scanID(702), scanID(703)
	scanAnchor(t, &elements, root, "", 0)
	scanEntity(t, &elements, lastID, root, 0, 0)
	scanEntity(t, &elements, firstID, root, 0, 0)
	before := elements.Clone()
	scope := [32]byte{7}
	query := contract.RegionQuery{StartAnchorID: root, BasisAnchorID: root, Selector: contract.QueryEntities}
	first, err := Coverage(elements.View, mounts.View, transferfact.Visibility{}, scope, query, contract.CoreReadCut{}, 1, nil, false)
	if err != nil || first.Status != contract.QueryPartial || first.Gap != contract.QueryNoGap ||
		len(first.Matches) != 1 || first.Matches[0].ElementID != firstID || first.Continuation == nil {
		t.Fatalf("first page: %#v %v", first, err)
	}
	if queryrequest.QueryScope(*first.Continuation) != scope ||
		queryrequest.QueryLast(*first.Continuation) != firstID ||
		queryrequest.QueryOwner(*first.Continuation) != "" {
		t.Fatal("scan changed the partial position or promoted it into an owner-issued token")
	}
	last, err := Coverage(elements.View, mounts.View, transferfact.Visibility{}, scope, query, contract.CoreReadCut{}, 1, first.Continuation, false)
	if err != nil || last.Status != contract.QueryComplete || last.Continuation != nil ||
		len(last.Matches) != 1 || last.Matches[0].ElementID != lastID ||
		!reflect.DeepEqual(last.Matches[0].Path, []values.ElementID{lastID}) {
		t.Fatalf("last page: %#v %v", last, err)
	}
	if !elements.Equal(before) || mounts.OutgoingCount() != 0 {
		t.Fatal("coverage mutated its input facts")
	}
}

func TestCoveragePreservesBasisBeforeMountAndPublicGap(t *testing.T) {
	elements, mounts := elementfact.New(), mountfact.New()
	root, child := scanID(711), scanID(712)
	scanAnchor(t, &elements, root, "", 0)
	scanEntity(t, &elements, child, root, 0, 0)
	mounts.ReplaceOutgoing(root, mountfact.Record{State: contract.MountActive})
	query := contract.RegionQuery{StartAnchorID: root, BasisAnchorID: scanID(799), Selector: contract.QueryEntities}
	for _, basis := range []values.ElementID{scanID(799), child} {
		query.BasisAnchorID = basis
		page, err := Coverage(elements.View, mounts.View, transferfact.Visibility{}, [32]byte{}, query, contract.CoreReadCut{}, 1, nil, false)
		if !errors.Is(err, contract.ErrInvalid) || !reflect.DeepEqual(page, contract.QueryPage{}) {
			t.Fatalf("basis error lost precedence over mount: %#v %v", page, err)
		}
	}
	query.BasisAnchorID = root
	if _, err := Coverage(elements.View, mounts.View, transferfact.Visibility{}, [32]byte{}, query, contract.CoreReadCut{}, 1, nil, false); !errors.Is(err, contract.ErrMountContextRequired) {
		t.Fatalf("local mount boundary bypassed: %v", err)
	}
	page, err := Coverage(elements.View, mounts.View, transferfact.Visibility{}, [32]byte{}, query, contract.CoreReadCut{}, 1, nil, true)
	if err != nil || page.Status != contract.QueryPartial || page.Gap != contract.QueryUnresolvedCoverage ||
		len(page.Matches) != 1 || page.Matches[0].ElementID != child {
		t.Fatalf("public mount gap: %#v %v", page, err)
	}
	mounts.ReplaceOutgoing(root, mountfact.Record{State: contract.MountNone})
	page, err = Coverage(elements.View, mounts.View, transferfact.Visibility{}, [32]byte{}, query, contract.CoreReadCut{}, 1, nil, false)
	if err != nil || page.Status != contract.QueryComplete || page.Gap != contract.QueryNoGap {
		t.Fatalf("inactive record became content: %#v %v", page, err)
	}
	query.StartAnchorID = scanID(799)
	if _, err := Coverage(elements.View, mounts.View, transferfact.Visibility{}, [32]byte{}, query, contract.CoreReadCut{}, 1, nil, false); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("basis outside the selected closure changed error: %v", err)
	}
}
