package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

const facetWorldAddress values.NextHop = "local:facet-world"

type facetWorld struct {
	policy         *realminstance.Realm
	actor, service values.PrincipalRef
	cores          []*core.Core
	facets         []*facet.Instance
	coreLinks      *composition.LocalFacetCoreConnections
	facetLinks     *composition.LocalFacetConnections
	realmLinks     *composition.LocalRealmConnections
	paths          []string
	counter        int
}

func newFacetWorld(t *testing.T) *facetWorld {
	return newFacetWorldConfigured(t, corecontract.Config{}, facet.Config{})
}
func newFacetWorldConfigured(t *testing.T, cc corecontract.Config, fc facet.Config) *facetWorld {
	return newFacetWorldCredentials(t, cc, fc, nil)
}
func newFacetWorldCredentials(t *testing.T, cc corecontract.Config, fc facet.Config, credentials []realm.CredentialBootstrap) *facetWorld {
	t.Helper()
	w := &facetWorld{actor: values.PrincipalRef{Realm: "facet-world", Principal: "caller"}, service: values.PrincipalRef{Realm: "facet-world", Principal: "service"}, coreLinks: &composition.LocalFacetCoreConnections{}, facetLinks: &composition.LocalFacetConnections{}, realmLinks: &composition.LocalRealmConnections{}}
	var err error
	bindings := []realm.PermissionBinding{binding(w.actor, realm.PermissionPutBinding, values.RealmScope(w.actor.Realm)), binding(w.actor, realm.PermissionDeleteBinding, values.RealmScope(w.actor.Realm))}
	if len(credentials) > 0 {
		bindings = append(bindings, binding(w.actor, realm.PermissionRevokeCredential, values.RealmScope(w.actor.Realm)))
	}
	w.policy, err = realminstance.New(realm.Seed{ID: w.actor.Realm, LocalPrincipals: []values.PrincipalID{w.actor.Principal, w.service.Principal}, Credentials: credentials, Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.realmLinks.BindEndpoint(facetWorldAddress, w.policy); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		path := filepath.Join(t.TempDir(), "core.db")
		w.paths = append(w.paths, path)
		c, err := core.CreateDurable(w.realmLinks, corecontract.RealmInitialization{Realm: w.actor.Realm, NextHop: facetWorldAddress}, path, cc)
		if err != nil {
			t.Fatal(err)
		}
		w.cores = append(w.cores, c)
		if err = w.coreLinks.BindEndpoint(values.NextHop(fmt.Sprintf("local:core%d", i)), composition.LocalFacetCoreEndpoint{Read: c.Facets(), Control: c.Facets(), Access: c.Access()}); err != nil {
			t.Fatal(err)
		}
		a, err := w.facetLinks.ForCore(w.service)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.Facets().SetFacetAcceptanceSource(a); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		path := filepath.Join(t.TempDir(), "facet.db")
		w.paths = append(w.paths, path)
		caller, err := w.policy.TrustLocalCaller(w.service)
		if err != nil {
			t.Fatal(err)
		}
		source, err := w.coreLinks.ForFacetCaller(caller)
		if err != nil {
			t.Fatal(err)
		}
		f, err := facet.CreateDurable(w.realmLinks, source, facetcontract.RealmInitialization{Realm: w.actor.Realm, NextHop: facetWorldAddress}, path, fc)
		if err != nil {
			t.Fatal(err)
		}
		w.facets = append(w.facets, f)
		if err = w.facetLinks.BindEndpoint(values.NextHop(fmt.Sprintf("local:facet%d", i)), f); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, c := range w.cores {
			_ = c.Close()
		}
		for _, f := range w.facets {
			_ = f.Close()
		}
	})
	return w
}
func (w *facetWorld) grant(t *testing.T, actor values.PrincipalRef, p values.PermissionID, scope values.OperationScope) {
	t.Helper()
	w.counter++
	if _, err := w.policy.PutBinding(w.actor, binding(actor, p, scope), w.policy.BindingSetRevision(), realm.OperationID(fmt.Sprint("grant-", w.counter))); err != nil {
		t.Fatal(err)
	}
}
func (w *facetWorld) root(t *testing.T, ci int, id values.ElementID) {
	c := w.cores[ci]
	w.grant(t, w.actor, corecontract.PermissionCreateRoot, values.CoreElementScope(c.AuthorityRef(), id))
	if _, err := c.Elements().CreateRoot(w.actor, id, corecontract.OperationID("root-"+string(id))); err != nil {
		t.Fatal(err)
	}
}
func (w *facetWorld) attach(t *testing.T, ci, fi int, id values.ElementID) values.FacetAcceptanceKey {
	t.Helper()
	ctx := context.Background()
	c, f := w.cores[ci], w.facets[fi]
	for _, p := range []values.PermissionID{corecontract.PermissionReadElement, corecontract.PermissionFacetPropose, corecontract.PermissionFacetActivate, corecontract.PermissionFacetCancel, corecontract.PermissionFacetDetach, corecontract.PermissionFacetContactUpdate} {
		w.grant(t, w.actor, p, values.CoreElementScope(c.AuthorityRef(), id))
	}
	for _, p := range []values.PermissionID{corecontract.PermissionFacetProposalRead, corecontract.PermissionFacetAttachmentRead} {
		w.grant(t, w.service, p, values.CoreElementScope(c.AuthorityRef(), id))
	}
	for _, p := range []values.PermissionID{facetcontract.PermissionAccept, facetcontract.PermissionReadAcceptance, facetcontract.PermissionCoreContactUpdate, facetcontract.PermissionListTypes} {
		w.grant(t, w.actor, p, values.FacetElementScope(f.AuthorityRef(), id))
	}
	w.grant(t, w.service, facetcontract.PermissionReadAcceptance, values.FacetElementScope(f.AuthorityRef(), id))
	for _, p := range []values.PermissionID{facetcontract.PermissionPut, facetcontract.PermissionGet, facetcontract.PermissionDelete} {
		w.grant(t, w.actor, p, values.FacetKeyScope(f.AuthorityRef(), id, fixtureType("content")))
	}
	read, err := c.Elements().ReadElement(w.actor, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := c.Facets().ProposeFacet(w.actor, id, f.AuthorityRef(), values.NextHop(fmt.Sprintf("local:facet%d", fi)), read.Attachment.Revision, corecontract.OperationID("propose-"+string(id)))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.Namespace.AcceptAttachment(ctx, w.actor, proposal.Pending, values.NextHop(fmt.Sprintf("local:core%d", ci)), facetcontract.OperationID("accept-"+string(id)))
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != facetcontract.Committed || !accepted.Acceptance.AcceptedAt.Valid() {
		t.Fatal("acceptance")
	}
	if _, err = f.Namespace.Get(ctx, w.actor, proposal.Pending, fixtureType("content")); !errors.Is(err, facetcontract.ErrNamespaceNone) {
		t.Fatalf("served before activation: %v", err)
	}
	if _, err = c.Facets().ActivateFacet(ctx, w.actor, id, proposal.Attachment.Revision, corecontract.OperationID("activate-"+string(id))); err != nil {
		t.Fatal(err)
	}
	return proposal.Pending
}
func TestP4MultipleOwnersSpecifiedContactsAndReopen(t *testing.T) {
	w := newFacetWorld(t)
	ctx := context.Background()
	const a values.ElementID = "00000000-0000-4000-8000-00000000fb01"
	const b values.ElementID = "00000000-0000-4000-8000-00000000fb02"
	const cID values.ElementID = "00000000-0000-4000-8000-00000000fb03"
	w.root(t, 0, a)
	w.root(t, 1, cID)
	c0 := w.cores[0]
	w.grant(t, w.actor, corecontract.PermissionCreateEntity, values.CoreElementScope(c0.AuthorityRef(), a))
	r, _ := corecontract.NewRotation(1, 0, 0, 0)
	pose, _ := corecontract.NewPose(corecontract.Vec3{}, r)
	sphere, _ := corecontract.NewBoundedSphere(1)
	if _, err := c0.Elements().CreateEntity(w.actor, b, corecontract.SpatialTuple{Parent: a, LocalPose: pose, Range: sphere}, "create-child"); err != nil {
		t.Fatal(err)
	}
	keys := []values.FacetAcceptanceKey{w.attach(t, 0, 0, a), w.attach(t, 0, 1, b), w.attach(t, 1, 0, cID)}
	for i, k := range keys {
		ci, fi := 0, 0
		if i == 1 {
			fi = 1
		}
		if i == 2 {
			ci = 1
		}
		read, err := w.cores[ci].Elements().ReadElement(w.actor, k.Element, nil)
		if err != nil {
			t.Fatal(err)
		}
		target, err := w.facetLinks.Connect(read.Attachment.NextHop, read.Attachment.Key.Facet)
		if err != nil {
			t.Fatal(err)
		}
		if target != w.facets[fi] {
			t.Fatal("wrong owner")
		}
		wrote, err := target.Namespace.Put(ctx, w.actor, read.Attachment.Key, fixtureType("content"), fixturePayload(fmt.Sprint(i)), facetcontract.ExpectAbsent(), facetcontract.OperationID(fmt.Sprint("put", i)))
		if err != nil || wrote.Core.Authority != k.Core || wrote.Acceptance.Key != k {
			t.Fatalf("write: %+v %v", wrote, err)
		}
	}
	// Repair both directions while the previous addresses are not bound.
	if err := w.coreLinks.UnbindEndpoint("local:core0"); err != nil {
		t.Fatal(err)
	}
	if err := w.facetLinks.UnbindEndpoint("local:facet0"); err != nil {
		t.Fatal(err)
	}
	f0 := w.facets[0]
	contact, err := f0.Namespace.ReadAcceptance(w.actor, keys[0])
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := f0.Namespace.UpdateCoreNextHop(w.actor, "reverse-repair", keys[0], contact.NextHopRevision, "local:repaired-core")
	if err != nil {
		t.Fatal(err)
	}
	read, err := c0.Elements().ReadElement(w.actor, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	forward, err := c0.Facets().UpdateFacetNextHop(w.actor, "forward-repair", keys[0], read.Attachment.NextHopRevision, "local:repaired-facet")
	if err != nil {
		t.Fatal(err)
	}
	if err = w.coreLinks.BindEndpoint("local:repaired-core", composition.LocalFacetCoreEndpoint{Read: c0.Facets(), Control: c0.Facets(), Access: c0.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = w.facetLinks.BindEndpoint("local:repaired-facet", f0); err != nil {
		t.Fatal(err)
	}
	if _, err = w.facetLinks.Connect("local:facet0", keys[0].Facet); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatal("implicit fallback")
	}
	if err = w.facetLinks.BindEndpoint("local:wrong", w.facets[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = w.facetLinks.Connect("local:wrong", keys[0].Facet); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatal("wrong target")
	}
	got, err := f0.Namespace.Get(ctx, w.actor, keys[0], fixtureType("content"))
	if err != nil || !bytes.Equal(got.Payload, fixturePayload("0")) {
		t.Fatal(err)
	}
	// Reopening restores relation-owned contacts; host rebinds only transport.
	coreOwner, facetOwner := c0.AuthorityRef(), f0.AuthorityRef()
	if err = c0.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f0.Close(); err != nil {
		t.Fatal(err)
	}
	w.cores[0], err = core.OpenDurable(w.realmLinks, w.paths[0], corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	source, _ := w.coreLinks.ForFacet(w.service)
	w.facets[0], err = facet.OpenDurable(w.realmLinks, source, w.paths[2], facet.Config{})
	if err != nil {
		t.Fatal(err)
	}
	c0, f0 = w.cores[0], w.facets[0]
	if c0.AuthorityRef() != coreOwner || f0.AuthorityRef() != facetOwner {
		t.Fatal("reopen identity")
	}
	acceptanceSource, _ := w.facetLinks.ForCore(w.service)
	if err = c0.Facets().SetFacetAcceptanceSource(acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err = w.coreLinks.BindEndpoint("local:repaired-core", composition.LocalFacetCoreEndpoint{Read: c0.Facets(), Control: c0.Facets(), Access: c0.Access()}); err != nil {
		t.Fatal(err)
	}
	if err = w.facetLinks.BindEndpoint("local:repaired-facet", f0); err != nil {
		t.Fatal(err)
	}
	rc, err := f0.Namespace.ReadAcceptance(w.actor, keys[0])
	if err != nil || rc.CoreNextHop != reverse.CoreNextHop || rc.NextHopRevision != reverse.NextHopRevision {
		t.Fatal("reverse lost")
	}
	cr, err := c0.Elements().ReadElement(w.actor, a, nil)
	if err != nil || cr.Attachment.NextHop != forward.NextHop || cr.Attachment.NextHopRevision != forward.NextHopRevision {
		t.Fatal("forward lost")
	}
	got, err = f0.Namespace.Get(ctx, w.actor, keys[0], fixtureType("content"))
	if err != nil || fixtureText(got.Payload) != "0" {
		t.Fatal("read after restart", err)
	}
	if _, err = c0.Facets().DetachFacet(w.actor, a, cr.Attachment.Revision, "detach"); err != nil {
		t.Fatal(err)
	}
	got, err = f0.Namespace.Get(ctx, w.actor, keys[0], fixtureType("content"))
	if !errors.Is(err, facetcontract.ErrNamespaceDetached) || got.Cut.Valid() || !got.Core.Cut.Valid() {
		t.Fatal("detached facts leaked", err)
	}
}
