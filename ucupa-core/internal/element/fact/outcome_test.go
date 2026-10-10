package fact

import (
	"errors"
	"reflect"
	"testing"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

func TestOutcomeResultPreservesReceiptClassification(t *testing.T) {
	cut := token.FromParts([32]byte{1}, 5)
	revision := testRevision(testAnchorID, 3)
	anchor := Anchor{ID: testAnchorID, Parent: testRootID, LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(testSphere(t, 2)), Revision: revision}
	entity := Entity{ID: testEntityID, Parent: testAnchorID, LocalPose: geometry.IdentityPose(), Range: testSphere(t, 1), Revision: testRevision(testEntityID, 4)}
	for _, tc := range []struct {
		name    string
		outcome Outcome
		err     error
		empty   bool
	}{
		{"committed anchor", Outcome{Status: contract.MutationCommitted, Kind: contract.ElementAnchor, Anchor: anchor}, nil, false},
		{"unchanged anchor", Outcome{Status: contract.MutationUnchanged, Kind: contract.ElementAnchor, Anchor: anchor}, nil, false},
		{"committed entity", Outcome{Status: contract.MutationCommitted, Kind: contract.ElementEntity, Entity: entity}, nil, false},
		{"unchanged entity", Outcome{Status: contract.MutationUnchanged, Kind: contract.ElementEntity, Entity: entity}, nil, false},
		{"delete", Outcome{Status: contract.MutationCommitted, Tombstone: revision}, nil, false},
		{"not found", Outcome{Status: contract.MutationRejected, Rejection: NotFound}, contract.ErrNotFound, false},
		{"conflict", Outcome{Status: contract.MutationRejected, Rejection: Conflict}, contract.ErrConflict, false},
		{"revision conflict", Outcome{Status: contract.MutationRejected, Rejection: RevisionConflict}, contract.ErrRevisionConflict, false},
		{"unknown status", Outcome{}, contract.ErrStore, true},
		{"unknown kind", Outcome{Status: contract.MutationCommitted, Kind: 255}, contract.ErrStore, true},
		{"committed rejection", Outcome{Status: contract.MutationCommitted, Rejection: NotFound}, contract.ErrStore, true},
		{"unchanged deletion", Outcome{Status: contract.MutationUnchanged, Tombstone: revision}, contract.ErrStore, true},
		{"missing deletion revision", Outcome{Status: contract.MutationCommitted}, contract.ErrStore, true},
		{"missing rejection", Outcome{Status: contract.MutationRejected}, contract.ErrStore, true},
		{"unknown rejection", Outcome{Status: contract.MutationRejected, Rejection: 255}, contract.ErrStore, true},
		{"rejected record", Outcome{Status: contract.MutationRejected, Rejection: Conflict, Kind: contract.ElementAnchor, Anchor: anchor}, contract.ErrStore, true},
		{"rejected tombstone", Outcome{Status: contract.MutationRejected, Rejection: Conflict, Tombstone: revision}, contract.ErrStore, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.outcome
			result, err := tc.outcome.Result(cut)
			if !errors.Is(err, tc.err) || tc.outcome != before {
				t.Fatalf("result error %v; outcome changed: %v", err, tc.outcome != before)
			}
			if tc.empty {
				if result != (contract.MutationResult{}) {
					t.Fatalf("invalid receipt exposed result: %#v", result)
				}
				return
			}
			if result.Status != tc.outcome.Status || result.OutcomeCut != cut {
				t.Fatalf("receipt classification/cut changed: %#v", result)
			}
			if tc.outcome.Kind == contract.ElementAnchor && (result.Record == nil || !reflect.DeepEqual(*result.Record, anchor.Record())) {
				t.Fatal("original anchor receipt changed")
			}
			if tc.outcome.Kind == contract.ElementEntity && (result.Record == nil || !reflect.DeepEqual(*result.Record, entity.Record())) {
				t.Fatal("original entity receipt changed")
			}
			if tc.outcome.Kind == 0 && (result.Record != nil || result.TombstoneRevision != tc.outcome.Tombstone) {
				t.Fatal("deletion or rejection projection changed")
			}
		})
	}
	for _, outcome := range []Outcome{
		{Status: contract.MutationCommitted, Kind: contract.ElementAnchor, Anchor: anchor},
		{Status: contract.MutationRejected, Rejection: NotFound},
	} {
		result, err := outcome.Result(contract.CoreReadCut{})
		if !errors.Is(err, contract.ErrStore) || result != (contract.MutationResult{}) {
			t.Fatal("invalid cut must fail before a receipt can be returned")
		}
	}
}

func TestRecordProjectionDoesNotAliasPrivateFacts(t *testing.T) {
	root := Anchor{ID: testRootID, Range: geometry.UnboundedRootRange(), Revision: testRevision(testRootID, 1)}
	if record := root.Record(); record.Anchor.ParentAnchorID != nil || record.Anchor.LocalPose != nil {
		t.Fatal("root acquired a parent or pose")
	}
	child := Anchor{ID: testAnchorID, Parent: testRootID, LocalPose: geometry.IdentityPose(), Range: geometry.BoundedRange(testSphere(t, 2)), Revision: testRevision(testAnchorID, 2)}
	before := child
	first, second := child.Record(), child.Record()
	*first.Anchor.ParentAnchorID = testEntityID
	first.Anchor.LocalPose.Translation.X = 9
	first.Anchor.ID = testEntityID
	if child != before || *second.Anchor.ParentAnchorID != testRootID || second.Anchor.LocalPose.Translation.X != 0 || second.Anchor.ID != testAnchorID {
		t.Fatal("public anchor aliases private fact or another result")
	}
	entity := Entity{ID: testEntityID, Parent: testAnchorID, Range: testSphere(t, 1), Revision: testRevision(testEntityID, 3)}
	r := entity.Record()
	r.Entity.ContainingAnchorID = testRootID
	if entity.Parent != testAnchorID || entity.Record().Entity.ContainingAnchorID != testAnchorID {
		t.Fatal("public entity aliases private fact")
	}
}
