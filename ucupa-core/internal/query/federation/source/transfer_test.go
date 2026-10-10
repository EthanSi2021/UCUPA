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

func TestDurableSourceFenceInvalidatesFederationBasisAndPreservesFreshGap(t *testing.T) {
	f := newSourceFixtureWithStore(t, true, true, &sourcePolicy{}, filepath.Join(t.TempDir(), "core.db"))
	f.create(t, 2, 2)
	f.create(t, 3, 3)
	part := f.open(t, "query", false, 0)
	if part.ReadMode != contract.QueryRetained {
		t.Fatal("fixture did not retain the original source basis")
	}
	ctx := context.Background()
	owner := f.ops.AuthorityRef()
	request := federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{Scope: f.root, Sphere: contract.QuerySphere{Radius: 10}, Selector: contract.QueryEntities}}
	first, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, request, nil, 1)
	if err != nil || first.Continuation == nil {
		t.Fatalf("first: %#v %v", first, err)
	}
	watch := f.open(t, "query", true, 0)
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, watch.Basis, request, nil, 256); err != nil {
		t.Fatal(err)
	}
	watchBegin, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, watch.Basis, federationcontract.FederationObserveRequest{Operation: "begin"})
	if err != nil {
		t.Fatal(err)
	}
	unrelated := f.open(t, "read", true, 0)
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, unrelated.Basis, federationcontract.FederationProjectionRequest{Kind: "read", Element: sourceID(3)}, nil, 1); err != nil {
		t.Fatal(err)
	}
	unrelatedBegin, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, unrelated.Basis, federationcontract.FederationObserveRequest{Operation: "begin"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.elements.ReadElement(f.caller.Subject, sourceID(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	intent := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "federation-fence", Actor: f.caller.Subject, Source: owner, Target: target,
		Boundary: sourceID(2), Expected: read.Record.Entity.Revision, Final: sourceTuple(t, sourceID(9000), 2),
		SourceAddress: "local:source", TargetAddress: "local:target"}
	f.owner.Lock()
	record, err := f.owner.BeginTransferIntentLocked(intent, 4096)
	if err == nil {
		_, err = f.owner.FenceTransferSourceLocked(f.caller.Subject, intent.Operation, record.Revision, transferfact.ParentHold{})
	}
	f.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ops.CheckFederationReadPart(ctx, f.caller, owner, part.Basis, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatalf("retained basis crossed source fence: %v", err)
	}
	// The own unsettled fence is neither ordinary data nor a final loss: the
	// old observation holds its proof pending, and only a settled decision
	// discloses terminal evidence while a stuck one loses its window.
	if page, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, watch.Basis, federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &watchBegin.Cursor, Limit: 1}); err != nil || len(page.Groups) != 0 {
		t.Fatalf("source fence became ordinary observation data: %#v %v", page, err)
	}
	if page, err := f.ops.ObserveFederationReadPart(ctx, f.caller, owner, unrelated.Basis, federationcontract.FederationObserveRequest{Operation: "poll", Cursor: &unrelatedBegin.Cursor, Limit: 1}); err != nil || len(page.Groups) != 0 {
		t.Fatalf("unrelated observation was terminated: %#v %v", page, err)
	}
	if page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, request, first.Continuation, 1); !errors.Is(err, values.PublicInvalidContext) || len(page.Matches) != 0 {
		t.Fatalf("retained projection crossed source fence: %#v %v", page, err)
	}
	exact := federationcontract.FederationProjectionRequest{Kind: "read-at", Element: sourceID(2)}
	if page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, exact, nil, 1); !errors.Is(err, values.PublicUnavailable) || len(page.Records) != 0 {
		t.Fatalf("historical source exact read disclosed: %#v %v", page, err)
	}
	exact.Element = sourceID(3)
	if page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, exact, nil, 1); err != nil || len(page.Records) != 1 {
		t.Fatalf("unrelated exact read stopped: %#v %v", page, err)
	}
	fresh := f.open(t, "query", false, 0)
	page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, fresh.Basis, request, nil, 256)
	if err != nil || page.Complete || page.Gap != contract.QueryUnresolvedCoverage || len(page.Matches) != 1 || page.Coverage == ([32]byte{}) {
		t.Fatalf("fresh source gap missing: %#v %v", page, err)
	}
	request.Query.Sphere = contract.QuerySphere{Center: contract.Vec3{X: 50}}
	page, err = f.ops.ProjectFederationReadPart(ctx, f.caller, owner, fresh.Basis, request, nil, 256)
	if err != nil || !page.Complete || page.Gap != contract.QueryNoGap || len(page.Matches) != 0 {
		t.Fatalf("proved exclusion stopped: %#v %v", page, err)
	}
}
