package value

import (
	"encoding/binary"
	"reflect"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

const (
	recordOwner  values.FactAuthorityRef = "00000000-0000-4000-8000-000000008001"
	recordFacet  values.FactAuthorityRef = "00000000-0000-4000-8000-000000008002"
	recordRoot   values.ElementID        = "00000000-0000-4000-8000-000000008003"
	recordAnchor values.ElementID        = "00000000-0000-4000-8000-000000008004"
	recordEntity values.ElementID        = "00000000-0000-4000-8000-000000008005"
)

func recordValues(t *testing.T) (token.PublicSigner, contract.Revision, contract.Pose, contract.BoundedSphere) {
	t.Helper()
	revision, err := (contract.ServiceRevision{Incarnation: [32]byte{1}, Element: recordEntity, Sequence: 7}).ElementRevision()
	if err != nil {
		t.Fatal(err)
	}
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{X: 1, Y: -2, Z: 3}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := contract.NewBoundedSphere(4)
	if err != nil {
		t.Fatal(err)
	}
	return token.NewPublicSigner([32]byte{9}, recordOwner), revision, pose, sphere
}

func TestRecordJSONPreservesRootAnchorAndEntityWireShapes(t *testing.T) {
	signer, revision, pose, sphere := recordValues(t)
	root := ElementRecordJSON(signer, contract.ElementRecord{Kind: contract.ElementAnchor, Anchor: &contract.AnchorRecord{ID: recordRoot, Range: geometry.UnboundedRootRange(), Revision: revision}})
	parent := recordRoot
	anchor := ElementRecordJSON(signer, contract.ElementRecord{Kind: contract.ElementAnchor, Anchor: &contract.AnchorRecord{ID: recordAnchor, ParentAnchorID: &parent, LocalPose: &pose, Range: geometry.BoundedRange(sphere), Revision: revision}})
	entity := ElementRecordJSON(signer, contract.ElementRecord{Kind: contract.ElementEntity, Entity: &contract.EntityRecord{ID: recordEntity, ContainingAnchorID: recordAnchor, LocalPose: pose, Range: sphere, Revision: revision}})
	for name, test := range map[string]struct {
		got  map[string]any
		want string
	}{
		"root":   {root, `{"element":"00000000-0000-4000-8000-000000008003","kind":"anchor","range":{"kind":"unbounded-root"},"revision":{"kind":"record-revision","owner":"00000000-0000-4000-8000-000000008001","value":"TaxbMQMSUR16tJjDSzbaLWWE_Vgu__ABvLQxE6nZLZw"}}`},
		"anchor": {anchor, `{"element":"00000000-0000-4000-8000-000000008004","kind":"anchor","parent":"00000000-0000-4000-8000-000000008003","pose":{"rotation":{"pivot":1,"ratios":[0,0,0]},"translation":[1,-2,3]},"range":{"kind":"bounded-sphere","radius":4},"revision":{"kind":"record-revision","owner":"00000000-0000-4000-8000-000000008001","value":"TaxbMQMSUR16tJjDSzbaLWWE_Vgu__ABvLQxE6nZLZw"}}`},
		"entity": {entity, `{"element":"00000000-0000-4000-8000-000000008005","kind":"entity","parent":"00000000-0000-4000-8000-000000008004","pose":{"rotation":{"pivot":1,"ratios":[0,0,0]},"translation":[1,-2,3]},"range":{"kind":"bounded-sphere","radius":4},"revision":{"kind":"record-revision","owner":"00000000-0000-4000-8000-000000008001","value":"TaxbMQMSUR16tJjDSzbaLWWE_Vgu__ABvLQxE6nZLZw"}}`},
	} {
		raw, err := values.CanonicalJSONValue(test.got)
		if err != nil || string(raw) != test.want {
			t.Fatalf("%s wire shape: %s err=%v", name, raw, err)
		}
	}
}

func TestElementReadAndAttachmentStatesPreserveClosedShapes(t *testing.T) {
	signer, revision, pose, sphere := recordValues(t)
	missing := ElementReadJSON(signer, contract.ElementRead{State: contract.ExistenceNotFound})
	if !reflect.DeepEqual(missing, map[string]any{"state": "NOT_FOUND"}) {
		t.Fatal(missing)
	}
	record := contract.ElementRecord{Kind: contract.ElementEntity, Entity: &contract.EntityRecord{ID: recordEntity, ContainingAnchorID: recordAnchor, LocalPose: pose, Range: sphere, Revision: revision}}
	for state, status := range map[contract.FacetAttachmentState]string{contract.FacetNone: "NONE", contract.FacetActive: "ACTIVE", contract.FacetDetached: "DETACHED", contract.FacetRetired: "RETIRED"} {
		attachment := contract.FacetAttachment{State: state}
		if state != contract.FacetNone {
			attachment = recordAttachment(t, state)
		}
		got := ElementReadJSON(signer, contract.ElementRead{State: contract.ExistencePresent, Record: &record, Attachment: attachment})
		projected := got["attachment"].(map[string]any)
		if got["state"] != "PRESENT" || got["record"] == nil || projected["state"] != status {
			t.Fatalf("state %d: %#v", state, got)
		}
		if state != contract.FacetNone {
			key := projected["key"].(map[string]any)
			if key["core"] != string(recordOwner) || key["facet"] != string(recordFacet) || key["element"] != string(recordEntity) {
				t.Fatalf("relationship fields: %#v", projected)
			}
			if state == contract.FacetActive {
				if projected["nextHop"] != "local:facet" || projected["contactRevision"] == nil {
					t.Fatal("active relationship lost contact", projected)
				}
			} else if projected["nextHop"] != nil || projected["contactRevision"] != nil {
				t.Fatal("non-active relationship exposed contact", projected)
			}
		}
	}
}

func recordAttachment(t *testing.T, state contract.FacetAttachmentState) contract.FacetAttachment {
	t.Helper()
	raw := make([]byte, 76)
	raw[0] = 1
	copy(raw[32:68], recordEntity)
	binary.BigEndian.PutUint64(raw[68:], 3)
	proposal, err := values.NewAttachmentRevision(raw)
	if err != nil {
		t.Fatal(err)
	}
	key := values.FacetAcceptanceKey{Core: recordOwner, Facet: recordFacet, Element: recordEntity, Proposal: proposal}
	a := contract.FacetAttachment{State: state, Revision: proposal, Key: key}
	if state == contract.FacetActive {
		a.NextHop, a.NextHopRevision = "local:facet", token.FacetContactRevisionFromParts([32]byte{2}, key, 4)
	}
	return a
}

func TestPathAndRecordValuesDoNotAliasInputsOrEachOther(t *testing.T) {
	path := []values.ElementID{recordAnchor, recordEntity}
	first := PathJSON(recordRoot, path)
	path[0] = recordRoot
	down := first["down"].([]any)
	if down[0] != string(recordAnchor) {
		t.Fatal("PathJSON retained input slice")
	}
	down[0] = "changed"
	second := PathJSON(recordRoot, []values.ElementID{recordAnchor, recordEntity})
	if second["down"].([]any)[0] != string(recordAnchor) {
		t.Fatal("PathJSON reused returned slice")
	}
	signer, revision, pose, sphere := recordValues(t)
	record := contract.ElementRecord{Kind: contract.ElementEntity, Entity: &contract.EntityRecord{ID: recordEntity, ContainingAnchorID: recordAnchor, LocalPose: pose, Range: sphere, Revision: revision}}
	a, b := ElementRecordJSON(signer, record), ElementRecordJSON(signer, record)
	a["kind"] = "changed"
	a["pose"].(map[string]any)["translation"].([]any)[0] = float64(99)
	if b["kind"] != "entity" || b["pose"].(map[string]any)["translation"].([]any)[0] != float64(1) {
		t.Fatal("record results share mutable maps or slices")
	}
}

func TestAttachmentAndPathJSONPreserveCompleteWireShapes(t *testing.T) {
	signer, _, _, _ := recordValues(t)
	for _, test := range []struct {
		state contract.FacetAttachmentState
		want  string
	}{
		{contract.FacetNone, `{"revision":{"kind":"attachment-revision","owner":"00000000-0000-4000-8000-000000008001","value":"RYMD8Kl_Gz3t6xt9laOPHzmObHph45DSeaXGOCe0XGM"},"state":"NONE"}`},
		{contract.FacetActive, `{"contactRevision":{"kind":"contact-revision","owner":"00000000-0000-4000-8000-000000008001","value":"IMBeqVD7ZIY6UWKL8P1Xepqm9ZY-or5CSf5XNrtusOU"},"key":{"core":"00000000-0000-4000-8000-000000008001","element":"00000000-0000-4000-8000-000000008005","facet":"00000000-0000-4000-8000-000000008002","proposal":{"kind":"attachment-revision","owner":"00000000-0000-4000-8000-000000008001","value":"DLsniVCd6C23yYsPZApmCUqwCGb10os-KTPO7lrIl9g"}},"nextHop":"local:facet","revision":{"kind":"attachment-revision","owner":"00000000-0000-4000-8000-000000008001","value":"DLsniVCd6C23yYsPZApmCUqwCGb10os-KTPO7lrIl9g"},"state":"ACTIVE"}`},
		{contract.FacetDetached, `{"key":{"core":"00000000-0000-4000-8000-000000008001","element":"00000000-0000-4000-8000-000000008005","facet":"00000000-0000-4000-8000-000000008002","proposal":{"kind":"attachment-revision","owner":"00000000-0000-4000-8000-000000008001","value":"DLsniVCd6C23yYsPZApmCUqwCGb10os-KTPO7lrIl9g"}},"revision":{"kind":"attachment-revision","owner":"00000000-0000-4000-8000-000000008001","value":"DLsniVCd6C23yYsPZApmCUqwCGb10os-KTPO7lrIl9g"},"state":"DETACHED"}`},
		{contract.FacetRetired, `{"key":{"core":"00000000-0000-4000-8000-000000008001","element":"00000000-0000-4000-8000-000000008005","facet":"00000000-0000-4000-8000-000000008002","proposal":{"kind":"attachment-revision","owner":"00000000-0000-4000-8000-000000008001","value":"DLsniVCd6C23yYsPZApmCUqwCGb10os-KTPO7lrIl9g"}},"revision":{"kind":"attachment-revision","owner":"00000000-0000-4000-8000-000000008001","value":"DLsniVCd6C23yYsPZApmCUqwCGb10os-KTPO7lrIl9g"},"state":"RETIRED"}`},
	} {
		attachment := contract.FacetAttachment{State: test.state}
		if test.state != contract.FacetNone {
			attachment = recordAttachment(t, test.state)
		}
		raw, err := values.CanonicalJSONValue(AttachmentJSON(signer, attachment))
		if err != nil || string(raw) != test.want {
			t.Fatalf("attachment %d wire shape: %s err=%v", test.state, raw, err)
		}
	}
	raw, err := values.CanonicalJSONValue(PathJSON(recordRoot, []values.ElementID{recordAnchor, recordEntity}))
	const wantPath = `{"down":["00000000-0000-4000-8000-000000008004","00000000-0000-4000-8000-000000008005"],"origin":"00000000-0000-4000-8000-000000008003","up":"0"}`
	if err != nil || string(raw) != wantPath {
		t.Fatalf("path wire shape: %s err=%v", raw, err)
	}
}
