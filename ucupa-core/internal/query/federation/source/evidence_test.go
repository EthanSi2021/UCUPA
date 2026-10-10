package source

import (
	"context"
	"slices"
	"testing"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	federationprojection "ucupa/ucupa-core/internal/query/federation/projection"
	federationvalue "ucupa/ucupa-core/internal/query/federation/value"

	"ucupa/shared/values"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/token"
)

func TestFederationReadEvidenceOpaqueAndBound(t *testing.T) {
	f := newSourceFixture(t, false)
	c, caller := f.ops, f.caller
	part := f.open(t, "query", false, 0)
	c.owner.Lock()
	basis, err := c.registry.ResolveReadLocked(caller, part.Basis)
	c.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	cut, view := federationReadStamps(basis)
	if part.Subject != caller.Subject || cut != part.CutStamp || view != part.AuthorizationStamp || cut == view || cut == ([32]byte{}) || view == ([32]byte{}) || view == [32]byte(basis.View) || cut == token.Scope(basis.Cut) {
		t.Fatal("invalid or raw evidence")
	}
	for _, change := range []func(*accessregistry.ReadUse){
		func(b *accessregistry.ReadUse) { b.Cut = token.FromParts(token.Scope(b.Cut), token.Serial(b.Cut)+1) },
		func(b *accessregistry.ReadUse) {
			scope := token.Scope(b.Cut)
			scope[0] ^= 1
			b.Cut = token.FromParts(scope, token.Serial(b.Cut))
		},
	} {
		clone := basis
		change(&clone)
		changed, unchanged := federationReadStamps(clone)
		if changed == cut || unchanged != view {
			t.Fatal("cut evidence not isolated")
		}
	}
	for _, change := range []func(*accessregistry.ReadUse){
		func(b *accessregistry.ReadUse) { b.View[0] ^= 1 },
		func(b *accessregistry.ReadUse) { b.Caller.Establishment[0] ^= 1 },
		func(b *accessregistry.ReadUse) { b.Caller.Subject.Realm = "other" },
		func(b *accessregistry.ReadUse) { b.Caller.Subject.Principal = "other" },
	} {
		clone := basis
		change(&clone)
		unchanged, changed := federationReadStamps(clone)
		if changed == view || unchanged != cut {
			t.Fatal("view evidence not bound")
		}
	}
	other := f.open(t, "query", false, 0)
	if other.CutStamp == cut || other.AuthorizationStamp == view {
		t.Fatal("independent captures correlatable")
	}
	clone := basis
	clone.Source.Request.Purpose[0] ^= 1
	x, y := federationReadStamps(clone)
	if x == cut || y == view {
		t.Fatal("request purpose not bound")
	}
	clone = basis
	clone.Token.Owner = values.FactAuthorityRef("00000000-0000-4000-8000-000000000099")
	x, y = federationReadStamps(clone)
	if x == cut || y == view {
		t.Fatal("owner not bound")
	}
}

func TestFederationCoverageBindsFullScanNotPage(t *testing.T) {
	f := newSourceFixture(t, true)
	c, root, caller := f.ops, f.root, f.caller
	if _, err := f.elements.CreateEntity(caller.Subject, sourceID(93901), sourceTuple(t, root, 2), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.elements.CreateEntity(caller.Subject, sourceID(93902), sourceTuple(t, root, 8), "two"); err != nil {
		t.Fatal(err)
	}
	part := f.open(t, "query", false, 0)
	q := federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{Scope: root, Sphere: corecontract.QuerySphere{Radius: 100}, Selector: corecontract.QueryBoth}}
	ctx := context.Background()
	first, err := c.ProjectFederationReadPart(ctx, caller, c.AuthorityRef(), part.Basis, q, nil, 1)
	if err != nil || first.Coverage == ([32]byte{}) || first.Continuation == nil {
		t.Fatal(first, err)
	}
	second, err := c.ProjectFederationReadPart(ctx, caller, c.AuthorityRef(), part.Basis, q, first.Continuation, 256)
	if err != nil || !second.Complete || first.Coverage != second.Coverage {
		t.Fatal("pagination changed scan proof", second, err)
	}
	if len(first.Parents) != 1 || first.Parents[0].Parent != root || first.Parents[0].Adjacency == ([32]byte{}) || !slices.Equal(first.Parents, second.Parents) {
		t.Fatal("parent closure not fixed across pages", first.Parents, second.Parents)
	}
	if _, err := f.elements.CreateEntity(caller.Subject, sourceID(93903), sourceTuple(t, root, 4), "later"); err != nil {
		t.Fatal(err)
	}
	repeat, err := c.ProjectFederationReadPart(ctx, caller, c.AuthorityRef(), part.Basis, q, nil, 256)
	if err != nil || repeat.Coverage != first.Coverage || !slices.Equal(repeat.Parents, first.Parents) {
		t.Fatal("retained coverage followed current write", repeat, err)
	}
	q.Query.Sphere.Radius = 1
	different, err := c.ProjectFederationReadPart(ctx, caller, c.AuthorityRef(), part.Basis, q, nil, 256)
	if err != nil || different.Coverage == first.Coverage {
		t.Fatal("different query reused evidence", different, err)
	}
	if first.Coverage == federationvalue.ProjectionTraversal(first) || first.Coverage == part.CutStamp || first.Coverage == part.AuthorizationStamp {
		t.Fatal("raw or unrelated evidence reused")
	}
}

func TestFederationParentsDoNotDiscloseUnmatchedBranches(t *testing.T) {
	f := newSourceFixture(t, true)
	c, root, caller := f.ops, f.root, f.caller
	near := f.createAnchor(t, 97001, root, 0, 10)
	far := f.createAnchor(t, 97002, root, 100, 10)
	f.createUnder(t, 97003, near, 0)
	f.createUnder(t, 97004, far, 0)
	part := f.open(t, "query", false, 0)
	q := federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{Scope: root, Sphere: corecontract.QuerySphere{Radius: 2}, Selector: corecontract.QueryEntities}}
	page, err := c.ProjectFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, q, nil, 1)
	if err != nil || len(page.Parents) != 2 || page.Parents[0].Parent != root || page.Parents[1].Parent != near {
		t.Fatal("unmatched branch disclosed or matched path missing", page.Parents, err)
	}
	if _, err := c.ProjectFederationReadPart(context.Background(), caller, c.AuthorityRef(), part.Basis, federationcontract.FederationProjectionRequest{Kind: "list", Element: root}, nil, 1); err != values.PublicDenied {
		t.Fatal("query upgraded to list", err)
	}
	private := map[values.ElementID][32]byte{}
	c.owner.RLock()
	facts := c.owner.FactsLocked()
	projection := federationprojection.New(facts.Elements, facts.Mount, facts.Attachments, c.owner.SourceVisibilityLocked(), c.owner.SigningLocked())
	local, _, _, err := projection.Project(context.Background(), root, q, "", 1)
	c.owner.RUnlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range local.Parents {
		private[p.Parent] = p.Adjacency
	}
	for _, p := range page.Parents {
		digest, ok := private[p.Parent]
		if !ok {
			t.Fatal("missing private parent digest")
		}
		if p.Adjacency == digest {
			t.Fatal("raw adjacency digest disclosed")
		}
	}
}
