package source

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Only the not-yet-public transition control is invoked privately. The mount,
// snapshot, pagination, observation, authorization and contact paths are their
// actual operations, backed by independent durable stores, not fabricated views.
func TestTransferParentHoldPreservesFederationReadAndFreshContact(t *testing.T) {
	ctx := context.Background()
	parent := newSourceFixtureWithStore(t, true, true, &sourcePolicy{}, filepath.Join(t.TempDir(), "parent.db"))
	carrier := newSourceFixtureWithStore(t, true, false, &sourcePolicy{}, filepath.Join(t.TempDir(), "carrier.db"))
	target := newSourceFixtureWithStore(t, true, true, &sourcePolicy{}, filepath.Join(t.TempDir(), "target.db"))
	connectSourceMounts(t, parent, carrier)
	id := parent.createAnchor(t, 60, parent.root, 0, 10)
	a, err := parent.elements.ReadElement(parent.caller.Subject, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parent.mounts.ApplyMount(ctx, contract.LocalCallers{parent.ops.AuthorityRef(): parent.caller.Subject, carrier.ops.AuthorityRef(): carrier.caller.Subject}, "parent-read-mount", contract.MountCommand{
		Kind: contract.MountEstablish, MountAnchorUUID: id, ExpectedElementRevision: a.Record.Anchor.Revision,
		CarrierAuthority: carrier.ops.AuthorityRef(), CarrierNextHop: "local:source-carrier", ParentNextHop: "local:source-parent",
	})
	if err != nil || !result.Settled {
		t.Fatal("mount", result, err)
	}
	entity := carrier.createUnder(t, 70, id, 0)
	e, err := carrier.elements.ReadElement(carrier.caller.Subject, entity, nil)
	if err != nil {
		t.Fatal(err)
	}
	i := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "held-federation-source", Actor: carrier.caller.Subject,
		Source: carrier.ops.AuthorityRef(), Target: target.ops.AuthorityRef(), Boundary: entity,
		Expected: e.Record.Entity.Revision, Final: sourceTuple(t, target.root, 0), SourceAddress: "local:source-carrier", TargetAddress: "local:target"}
	carrier.owner.Lock()
	initial, err := carrier.owner.BeginTransferIntentLocked(i, 4096)
	carrier.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	parent.owner.Lock()
	r := parent.owner.FactsLocked().Mount.Record(id)
	hold, err := parent.owner.ReserveTransferParentLocked(transferfact.ParentIntent{Initial: initial, Role: transferfact.SourceParent,
		Parent: parent.ops.AuthorityRef(), Mount: id, ExpectedElement: a.Record.Anchor.Revision, ExpectedMount: r.Revision, ParentAddress: "local:source-parent"})
	parent.owner.Unlock()
	if err != nil {
		t.Fatal("hold", err)
	}
	parent.create(t, 2, 20)
	parent.create(t, 3, 30)
	owner := parent.ops.AuthorityRef()
	request := federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{
		Scope: parent.root, Sphere: contract.QuerySphere{Radius: 100}, Selector: contract.QueryEntities,
	}}
	part := parent.open(t, "query", true, 0)
	first, err := parent.ops.ProjectFederationReadPart(ctx, parent.caller, owner, part.Basis, request, nil, 1)
	if err != nil || first.Continuation == nil || len(first.Frontiers) != 1 || !first.Frontiers[0].Relation.WriteGateOpen {
		t.Fatal("stable hold stopped frontier", first, err)
	}
	next, err := parent.ops.ProjectFederationReadPart(ctx, parent.caller, owner, part.Basis, request, first.Continuation, 1)
	if err != nil || !next.Complete || next.Gap != contract.QueryNoGap || len(next.Matches) != 1 || first.Coverage != next.Coverage {
		t.Fatal("stable hold invalidated continuation", next, err)
	}
	watch, err := parent.ops.ObserveFederationReadPart(ctx, parent.caller, owner, part.Basis, federationcontract.FederationObserveRequest{Operation: "begin"})
	if err != nil {
		t.Fatal("hold disabled observation", err)
	}
	if _, err = parent.ops.ObserveFederationReadPart(ctx, parent.caller, owner, part.Basis, federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &watch.Cursor, Limit: 1}); err != nil {
		t.Fatal("unchanged hold generated a gap", err)
	}
	parent.auth.epoch++
	if err := parent.ops.CheckFederationReadPart(ctx, parent.caller, owner, part.Basis, false); err == nil {
		t.Fatal("hold bypassed current authorization")
	}
	part = parent.open(t, "query", false, 0)
	first, err = parent.ops.ProjectFederationReadPart(ctx, parent.caller, owner, part.Basis, request, nil, 1)
	if err != nil || len(first.Frontiers) != 1 {
		t.Fatal("reauthorized hold", first, err)
	}
	if _, err = parent.mounts.UpdateMountNextHop(parent.caller.Subject, "held-read-contact", id, r.Key, r.Contact.Revision, "local:repaired-carrier"); err != nil {
		t.Fatal("contact repair waited for hold release", err)
	}
	if err = parent.ops.CheckFederationReadPart(ctx, parent.caller, owner, part.Basis, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("old contact retained read validity", err)
	}
	part = parent.open(t, "query", false, 0)
	fresh, err := parent.ops.ProjectFederationReadPart(ctx, parent.caller, owner, part.Basis, request, nil, 10)
	if err != nil || len(fresh.Frontiers) != 1 || !fresh.Frontiers[0].Relation.WriteGateOpen || fresh.Frontiers[0].Relation.NextHop != "local:repaired-carrier" {
		t.Fatal("fresh proof did not use repaired contact", fresh, err)
	}
	carrier.owner.Lock()
	cancelled, err := carrier.owner.CancelTransferIntentLocked(carrier.caller.Subject, i.Operation, initial.Revision)
	carrier.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	parent.owner.Lock()
	_, err = parent.owner.ReleaseTransferParentLocked(hold, transferfact.ParentRelease{Cancelled: cancelled})
	parent.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = parent.ops.CheckFederationReadPart(ctx, parent.caller, owner, part.Basis, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("released relationship silently reused old receipt", err)
	}
	part = parent.open(t, "query", false, 0)
	if _, err = parent.ops.ProjectFederationReadPart(ctx, parent.caller, owner, part.Basis, request, nil, 10); err != nil {
		t.Fatal("fresh read after release", err)
	}
}
