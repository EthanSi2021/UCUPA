package facet_test

import (
	"context"
	"encoding/binary"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/facet"
)

type testAuthorizer struct {
	local    bool
	decision values.AuthorizationDecision
	validate values.AuthorizationDecision
	before   func()
	view     values.AuthorizationView
	caller   values.CallerContext
}

func TestFacetOperationsExposeOnlyActualRelationshipEntry(t *testing.T) {
	want := []string{"ActivateFacet", "ApplyFacetControl", "AuthorityRef", "BeginFacetReplacement", "CancelFacetProposal", "CancelFacetReplacement", "CheckAttachmentControl", "DetachFacet", "InspectFacetControl", "InspectFacetReplacement", "ProposeFacet", "ReadAttachmentControl", "ReadFacetAttachment", "ReadFacetProposal", "ReadFacetReplacementRecord", "ReadFacetSuccessor", "ReadProposalControl", "ReadSuccessorControl", "RecoverFacetReplacement", "ReleaseFacet", "ResolveProposalControl", "SetFacetAcceptanceSource", "SetFacetDispositionSource", "UpdateFacetNextHop"}
	typ := reflect.TypeOf((*facet.Operations)(nil))
	got := make([]string, typ.NumMethod())
	for i := range got {
		got[i] = typ.Method(i).Name
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relationship entry: got %v, want %v", got, want)
	}
	var absent *facet.Operations
	if absent.AuthorityRef() != "" {
		t.Fatal("nil entry has identity")
	}
	if _, err := absent.ReadFacetProposal(facetActor, facetEntity); !errors.Is(err, contract.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := absent.ReadFacetAttachment(facetActor, facetEntity); !errors.Is(err, contract.ErrInvalid) {
		t.Fatal(err)
	}
	if err := absent.SetFacetAcceptanceSource(nil); !errors.Is(err, contract.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := absent.InspectFacetControl(context.Background(), values.CallerContext{}, facetEntity); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal(err)
	}
}

func (a *testAuthorizer) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	if a.local {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (a *testAuthorizer) AuthorizeView(ctx context.Context, caller values.CallerContext, _ ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	if a.caller != (values.CallerContext{}) && caller != a.caller {
		return values.AuthorizationDenied, values.AuthorizationView{}
	}
	return a.decision, a.view
}

func (a *testAuthorizer) ValidateView(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, _ ...values.Requirement) values.AuthorizationDecision {
	if a.before != nil {
		a.before()
	}
	if (a.caller != (values.CallerContext{}) && caller != a.caller) || view != a.view {
		return values.AuthorizationChanged
	}
	return a.validate
}

func (*testAuthorizer) CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (*testAuthorizer) ValidateDenialView(_ context.Context, _ values.CallerContext, view values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	return values.AuthorizationChanged
}

type acceptanceSource func(context.Context, values.NextHop, values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error)

func (f acceptanceSource) ReadFacetAcceptance(ctx context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
	return f(ctx, hop, key)
}

var facetActor = values.PrincipalRef{Realm: "facet-operation-test", Principal: "writer"}

const (
	facetRoot   values.ElementID = "32000000-0000-4000-8000-000000000001"
	facetEntity values.ElementID = "32000000-0000-4000-8000-000000000002"
)

type fixture struct {
	owner     *commit.Owner
	elements  *element.Operations
	facets    *facet.Operations
	admission *access.Admission
	auth      *testAuthorizer
	path      string
	entity    contract.MutationResult
}

func publicLimits(limit uint64) values.PublicLimits {
	return values.PublicLimits{ContextTTL: time.Minute, ObservationTTL: time.Minute, ConcurrentReads: limit}
}

func newFixture(t *testing.T, durable bool) fixture {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	var owner *commit.Owner
	path := ""
	if durable {
		path = filepath.Join(t.TempDir(), "core.db")
		owner, err = commit.CreateDurable(path, authority, [32]byte{32}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	} else {
		owner, err = commit.New(authority, [32]byte{32}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.CloseStore() })
	auth := &testAuthorizer{local: true, decision: values.AuthorizationValid, validate: values.AuthorizationValid, view: values.AuthorizationView{1}}
	elements := element.New(owner, auth)
	admission := access.New(auth, owner, publicLimits(8))
	facets := facet.New(owner, auth, admission, nil)
	if _, err = elements.CreateRoot(facetActor, facetRoot, "root"); err != nil {
		t.Fatal(err)
	}
	entity, err := elements.CreateEntity(facetActor, facetEntity, tuple(t, facetRoot), "entity")
	if err != nil {
		t.Fatal(err)
	}
	return fixture{owner: owner, elements: elements, facets: facets, admission: admission, auth: auth, path: path, entity: entity}
}

func tuple(t *testing.T, parent values.ElementID) contract.SpatialTuple {
	t.Helper()
	rotation, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	radius, err := contract.NewBoundedSphere(1)
	if err != nil {
		t.Fatal(err)
	}
	return contract.SpatialTuple{Parent: parent, LocalPose: pose, Range: radius}
}

func accepted(key values.FacetAcceptanceKey) contract.FacetAcceptanceEvidence {
	var raw [40]byte
	raw[0] = 7
	binary.BigEndian.PutUint64(raw[32:], 2)
	cut, _ := values.NewFacetReadCut(raw[:])
	return contract.FacetAcceptanceEvidence{Key: key, AcceptedAt: cut}
}

func proposal(t *testing.T, f fixture, operation contract.OperationID) contract.FacetAttachmentResult {
	t.Helper()
	read, err := f.elements.ReadElement(facetActor, facetEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.facets.ProposeFacet(facetActor, facetEntity, target, "local:facet", read.Attachment.Revision, operation)
	if err != nil || r.Status != contract.MutationCommitted {
		t.Fatalf("propose = %#v, %v", r, err)
	}
	return r
}

type factProjection struct {
	cut        contract.CoreReadCut
	attachment contract.FacetAttachment
	pending    values.FacetAcceptanceKey
	nextHop    values.NextHop
}

func facts(t *testing.T, owner *commit.Owner, id values.ElementID) factProjection {
	t.Helper()
	owner.RLock()
	defer owner.RUnlock()
	r, ok := owner.FactsLocked().Attachments.Record(id)
	if !ok {
		t.Fatal("attachment fact missing")
	}
	return factProjection{cut: owner.CurrentCutLocked(), attachment: r.Attachment, pending: r.Pending, nextHop: r.NextHop}
}

func TestFacetLocalLifecycleAndDurableExactReplay(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "durable"}[durable], func(t *testing.T) {
			f := newFixture(t, durable)
			p := proposal(t, f, "propose")
			if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, hop values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
				if hop != "local:repaired" || key != p.Pending {
					t.Fatalf("acceptance request = %q %#v", hop, key)
				}
				return accepted(key), nil
			})); err != nil {
				t.Fatal(err)
			}
			repaired, err := f.facets.UpdateFacetNextHop(facetActor, "repair", p.Pending, p.NextHopRevision, "local:repaired")
			if err != nil || repaired.Attachment.Revision != p.Attachment.Revision || repaired.NextHopRevision == p.NextHopRevision {
				t.Fatalf("repair = %#v, %v", repaired, err)
			}
			active, err := f.facets.ActivateFacet(context.Background(), facetActor, facetEntity, p.Attachment.Revision, "activate")
			if err != nil || active.Attachment.State != contract.FacetActive || active.NextHop != "local:repaired" {
				t.Fatalf("activate = %#v, %v", active, err)
			}
			detached, err := f.facets.DetachFacet(facetActor, facetEntity, active.Attachment.Revision, "detach")
			if err != nil || detached.Attachment.State != contract.FacetDetached || detached.Attachment.Key != p.Pending || detached.NextHop != "" {
				t.Fatalf("detach = %#v, %v", detached, err)
			}
			if durable {
				before := facts(t, f.owner, facetEntity)
				f.owner.Lock()
				f.owner.BeginCloseLocked()
				f.owner.Unlock()
				if err = f.owner.CloseStore(); err != nil {
					t.Fatal(err)
				}
				owner, openErr := commit.OpenDurable(f.path, nil)
				if openErr != nil {
					t.Fatal(openErr)
				}
				defer owner.CloseStore()
				f.owner, f.facets = owner, facet.New(owner, f.auth, access.New(f.auth, owner, publicLimits(8)), nil)
				replay, replayErr := f.facets.ActivateFacet(context.Background(), facetActor, facetEntity, p.Attachment.Revision, "activate")
				if replayErr != nil || replay != active || facts(t, owner, facetEntity) != before {
					t.Fatalf("durable replay = %#v, %v", replay, replayErr)
				}
			}
		})
	}
}

func TestFacetCancelIsExactAndDoesNotChangeSpatialRecord(t *testing.T) {
	f := newFixture(t, false)
	p := proposal(t, f, "proposal")
	before, err := f.elements.ReadElement(facetActor, facetEntity, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.facets.CancelFacetProposal(facetActor, facetEntity, p.Attachment.Revision, "cancel")
	if err != nil || cancelled.Pending.Valid() || cancelled.NextHop != "" || cancelled.Attachment.State != contract.FacetNone {
		t.Fatalf("cancel = %#v, %v", cancelled, err)
	}
	again, err := f.facets.CancelFacetProposal(facetActor, facetEntity, p.Attachment.Revision, "cancel")
	if err != nil || again != cancelled {
		t.Fatalf("cancel replay = %#v, %v", again, err)
	}
	after, err := f.elements.ReadElement(facetActor, facetEntity, nil)
	if err != nil || before.Record.Entity.Revision != after.Record.Entity.Revision {
		t.Fatal("attachment cancellation changed the spatial record")
	}
}

func TestFacetAcceptanceRunsOutsideLockAndRechecksCurrentState(t *testing.T) {
	for _, mode := range []string{"cancel", "delete", "revoke"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t, false)
			p := proposal(t, f, "proposal")
			if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
				switch mode {
				case "cancel":
					if _, err := f.facets.CancelFacetProposal(facetActor, facetEntity, p.Attachment.Revision, "cancel-inside-source"); err != nil {
						t.Fatal(err)
					}
				case "delete":
					if _, err := f.elements.DeleteEntity(facetActor, facetEntity, f.entity.Record.Entity.Revision, "delete-inside-source"); err != nil {
						t.Fatal(err)
					}
				case "revoke":
					f.auth.local = false
				}
				return accepted(key), nil
			})); err != nil {
				t.Fatal(err)
			}
			_, err := f.facets.ActivateFacet(context.Background(), facetActor, facetEntity, p.Attachment.Revision, "late-activate")
			switch mode {
			case "revoke":
				if !errors.Is(err, contract.ErrDenied) {
					t.Fatalf("revoke error = %v", err)
				}
			case "cancel":
				if !errors.Is(err, contract.ErrAttachmentRevisionConflict) {
					t.Fatalf("cancel error = %v", err)
				}
			case "delete":
				if !errors.Is(err, contract.ErrNotFound) && !errors.Is(err, contract.ErrAttachmentRevisionConflict) {
					t.Fatalf("delete error = %v", err)
				}
			}
		})
	}
}
