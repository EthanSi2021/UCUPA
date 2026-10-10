package source

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/mount"
)

type sourceEpochPolicy struct {
	epoch      byte
	expandABA  bool
	authorizes int
	deny       values.PermissionID
}

func (*sourceEpochPolicy) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationValid
}

func (a *sourceEpochPolicy) AuthorizeView(_ context.Context, _ values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	a.authorizes++
	if a.expandABA && a.authorizes == 2 {
		a.epoch++
	}
	for _, requirement := range requirements {
		if requirement.Permission == a.deny {
			return values.AuthorizationDenied, values.AuthorizationView{}
		}
	}
	return values.AuthorizationValid, values.AuthorizationView{a.epoch}
}

func (a *sourceEpochPolicy) ValidateView(_ context.Context, _ values.CallerContext, view values.AuthorizationView, requirements ...values.Requirement) values.AuthorizationDecision {
	if view[0] != a.epoch {
		return values.AuthorizationChanged
	}
	for _, requirement := range requirements {
		if requirement.Permission == a.deny {
			return values.AuthorizationDenied
		}
	}
	return values.AuthorizationValid
}

// The denial port mirrors the deny rule without touching the authorizes
// counter, which counts view grants only.
func (a *sourceEpochPolicy) CaptureDenialView(_ context.Context, caller values.CallerContext, requirement values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	if requirement.Permission == a.deny {
		return values.AuthorizationDenied, sourceAuthorizationDenialDigest(caller.Subject, requirement)
	}
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}

func (a *sourceEpochPolicy) ValidateDenialView(ctx context.Context, caller values.CallerContext, view values.AuthorizationDenialView, requirement values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	if _, current := a.CaptureDenialView(ctx, caller, requirement); current != view {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

type sourceRelationWorld struct {
	owners  []sourceFixture
	callers contract.LocalCallers
	hops    []values.NextHop
}

func newSourceRelationWorld(t *testing.T) *sourceRelationWorld {
	t.Helper()
	w := &sourceRelationWorld{
		owners: []sourceFixture{
			newSourceFixtureWithRoot(t, true, true),
			newSourceFixtureWithRoot(t, true, false),
			newSourceFixtureWithRoot(t, true, false),
			newSourceFixtureWithRoot(t, true, false),
		},
		callers: make(contract.LocalCallers),
		hops:    []values.NextHop{"local:relation-a", "local:relation-b", "local:relation-c", "local:relation-d"},
	}
	endpoints := make(map[values.NextHop]*mount.Operations, len(w.owners))
	for i, owner := range w.owners {
		w.callers[owner.ops.AuthorityRef()] = owner.caller.Subject
		endpoints[w.hops[i]] = owner.mounts
	}
	for _, owner := range w.owners {
		if err := owner.mounts.ConfigureMountTransport(&sourceMountTransport{source: owner.ops.AuthorityRef(), endpoints: endpoints}); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func (w *sourceRelationWorld) establish(t *testing.T, parent, carrier int, id, parentID values.ElementID, radius float64, operation contract.OperationID) contract.MountControlResult {
	t.Helper()
	created, err := w.owners[parent].elements.CreateAnchor(w.owners[parent].caller.Subject, id, sourceTupleWithRadius(t, parentID, 0, radius), contract.OperationID("anchor-"+string(operation)))
	if err != nil || created.Record == nil || created.Record.Anchor == nil {
		t.Fatalf("create mount anchor: %#v %v", created, err)
	}
	command := contract.MountCommand{
		Kind:                    contract.MountEstablish,
		MountAnchorUUID:         id,
		ExpectedElementRevision: created.Record.Anchor.Revision,
		CarrierAuthority:        w.owners[carrier].ops.AuthorityRef(),
		CarrierNextHop:          w.hops[carrier],
		ParentNextHop:           w.hops[parent],
	}
	result, err := w.owners[parent].mounts.ApplyMount(context.Background(), w.callers, operation, command)
	if err != nil || result.Decision != contract.MountCommitted || !result.Settled {
		t.Fatalf("establish %s: %#v %v", operation, result, err)
	}
	return result
}

func openRelationPart(t *testing.T, owner sourceFixture, coordinator values.FactAuthorityRef, boundary values.ElementID, incoming contract.MountKey, intent string, prepareObserve bool) federationcontract.FederationReadPart {
	t.Helper()
	part, err := owner.ops.OpenFederationReadPart(context.Background(), owner.caller, coordinator, federationcontract.FederationReadRequest{
		Boundary: boundary, Incoming: incoming, Intent: intent, PrepareObserve: prepareObserve, Purpose: [32]byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return part
}

func relationQuery(scope values.ElementID) federationcontract.FederationProjectionRequest {
	return federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{
		Scope: scope, Sphere: contract.QuerySphere{Radius: 1000}, Selector: contract.QueryBoth,
	}}
}

func TestSourceNestedAdjacencyAndExternalBoundary(t *testing.T) {
	// Preserves the former root federation relation test with four actual fact
	// owners and three real Mount handshakes; no external boundary is fabricated.
	w := newSourceRelationWorld(t)
	a, b, c := w.owners[0], w.owners[1], w.owners[2]
	m, n, sibling := sourceID(9710), sourceID(9720), sourceID(9730)
	ab := w.establish(t, 0, 1, m, a.root, 20, "part-ab")
	bc := w.establish(t, 1, 2, n, m, 10, "part-bc")
	w.establish(t, 0, 3, sibling, a.root, 10, "part-ad")
	leaf := c.createUnder(t, 9740, n, 0)

	parent := openRelationPart(t, a, a.ops.AuthorityRef(), a.root, contract.MountKey{}, "query", false)
	page, err := a.ops.ProjectFederationReadPart(context.Background(), a.caller, a.ops.AuthorityRef(), parent.Basis, relationQuery(a.root), nil, 256)
	if err != nil || !page.Complete || len(page.Matches) != 3 || len(page.Frontiers) != 2 {
		t.Fatalf("multiple outgoing mounts: %#v %v", page, err)
	}
	for _, frontier := range page.Frontiers {
		if !frontier.Relation.Key.Valid() || !frontier.Relation.Contact.Valid() || !frontier.Relation.WriteGateOpen {
			t.Fatalf("missing exact relation/contact: %#v", frontier)
		}
	}

	partB := openRelationPart(t, b, a.ops.AuthorityRef(), m, ab.MountKey, "query", false)
	rowsB, err := b.ops.ProjectFederationReadPart(context.Background(), b.caller, a.ops.AuthorityRef(), partB.Basis, relationQuery(m), nil, 256)
	if err != nil || len(rowsB.Matches) != 1 || rowsB.Matches[0].ID != n || len(rowsB.Frontiers) != 1 || partB.Incoming == nil {
		t.Fatalf("B must have one real N, not fake M: %#v %#v %v", partB, rowsB, err)
	}

	// A is the authenticated coordinator while B remains C's immediate spatial
	// parent. The source binding must not conflate those two identities.
	partC := openRelationPart(t, c, a.ops.AuthorityRef(), n, bc.MountKey, "query", true)
	rowsC, err := c.ops.ProjectFederationReadPart(context.Background(), c.caller, a.ops.AuthorityRef(), partC.Basis, relationQuery(n), nil, 256)
	if err != nil || len(rowsC.Matches) != 1 || rowsC.Matches[0].ID != leaf || len(rowsC.Matches[0].Path) == 0 || rowsC.Matches[0].Path[0] != leaf {
		t.Fatalf("indirect coordinator/external boundary: %#v %v", rowsC, err)
	}
	if partC.Incoming == nil || rowsB.Frontiers[0].Relation.Key != partC.Incoming.Key || rowsB.Frontiers[0].Relation.Admission != partC.Incoming.Admission || rowsB.Frontiers[0].Relation.Range != partC.Incoming.Range || partC.Incoming.Contact.Owner != c.ops.AuthorityRef() {
		t.Fatal("owners cannot seal exact admitted edge")
	}

	bad := bc.MountKey
	bad.ParentAuthority = a.ops.AuthorityRef()
	if _, err := c.ops.OpenFederationReadPart(context.Background(), c.caller, a.ops.AuthorityRef(), federationcontract.FederationReadRequest{Boundary: n, Incoming: bad, Intent: "read", Purpose: [32]byte{1}}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("fake external boundary accepted", err)
	}

	mountRead, err := c.mounts.ReadMount(c.caller.Subject, n)
	if err != nil || !mountRead.Incoming {
		t.Fatal("read incoming mount", mountRead, err)
	}
	if _, err := c.mounts.UpdateMountNextHop(c.caller.Subject, "part-parent-contact", n, mountRead.MountKey, mountRead.NextHopRevision, "local:repaired-b"); err != nil {
		t.Fatal(err)
	}
	if err := c.ops.CheckFederationReadPart(context.Background(), c.caller, a.ops.AuthorityRef(), partC.Basis, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("retained relation contact silently replaced", err)
	}
	if partC.Preparation == nil {
		t.Fatal("observable source omitted preparation")
	}
	if _, err := c.ops.ObserveFederationReadPart(context.Background(), c.caller, a.ops.AuthorityRef(), partC.Basis, federationcontract.FederationObserveRequest{Operation: "begin"}); !errors.Is(err, values.PublicGap) {
		t.Fatal("invalid source relation didn't GAP Observe", err)
	}
}

func TestSourceOriginalGrantAcrossExpansionAndContinuation(t *testing.T) {
	for _, expand := range []bool{false, true} {
		t.Run(map[bool]string{false: "stable-expansion", true: "aba-during-expansion"}[expand], func(t *testing.T) {
			auth := &sourceEpochPolicy{epoch: 1, expandABA: expand}
			f := newSourceFixtureWithAuthorizer(t, true, true, auth)
			part, err := f.ops.OpenFederationReadPart(context.Background(), f.caller, f.ops.AuthorityRef(), federationcontract.FederationReadRequest{Boundary: f.root, Intent: "read", Purpose: [32]byte{1}, PrepareObserve: true})
			if err != nil {
				t.Fatal(err)
			}
			if auth.authorizes != 1 {
				t.Fatal("projection must retain one original grant, not expand or reauthorize it", auth.authorizes)
			}
			auth.epoch++
			if err := f.ops.CheckFederationReadPart(context.Background(), f.caller, f.ops.AuthorityRef(), part.Basis, false); !errors.Is(err, values.PublicInvalidContext) {
				t.Fatal("continuation renewed original grant", err)
			}
			if _, err := f.ops.ProjectFederationReadPart(context.Background(), f.caller, f.ops.AuthorityRef(), part.Basis, federationcontract.FederationProjectionRequest{Kind: "read", Element: f.root}, nil, 1); !errors.Is(err, values.PublicInvalidContext) {
				t.Fatal("revoked source disclosed records", err)
			}
			if auth.authorizes != 1 {
				t.Fatal("continuation acquired a replacement grant", auth.authorizes)
			}
		})
	}
}

func TestSourceQueryNeedsOnlyQueryGrant(t *testing.T) {
	for _, deny := range []values.PermissionID{contract.PermissionReadElement, contract.PermissionListDirect, contract.PermissionMountRead, contract.PermissionQueryRegion} {
		t.Run(string(deny), func(t *testing.T) {
			auth := &sourceEpochPolicy{epoch: 1, deny: deny}
			f := newSourceFixtureWithAuthorizer(t, false, true, auth)
			part, err := f.ops.OpenFederationReadPart(context.Background(), f.caller, f.ops.AuthorityRef(), federationcontract.FederationReadRequest{Boundary: f.root, Intent: "query", Purpose: [32]byte{1}})
			if deny == contract.PermissionQueryRegion {
				if !errors.Is(err, values.PublicDenied) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatalf("query added %s requirement: %v", deny, err)
			}
			page, err := f.ops.ProjectFederationReadPart(context.Background(), f.caller, f.ops.AuthorityRef(), part.Basis, federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{Scope: f.root, Sphere: contract.QuerySphere{Radius: 1}, Selector: contract.QueryBoth}}, nil, 1)
			if err != nil || len(page.Matches) != 1 || len(page.Records) != 0 {
				t.Fatalf("query-only projection: %+v %v", page, err)
			}
			if _, err := f.ops.ProjectFederationReadPart(context.Background(), f.caller, f.ops.AuthorityRef(), part.Basis, federationcontract.FederationProjectionRequest{Kind: "read", Element: f.root}, nil, 1); !errors.Is(err, values.PublicDenied) {
				t.Fatalf("query upgraded to Read: %v", err)
			}
		})
	}
}
