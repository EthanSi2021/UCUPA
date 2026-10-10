package source

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	"ucupa/ucupa-core/internal/access"
)

func (f sourceFixture) open(t *testing.T, intent string, prepare bool, mode contract.QueryReadMode) federationcontract.FederationReadPart {
	t.Helper()
	p, err := f.ops.OpenFederationReadPart(context.Background(), f.caller, f.ops.AuthorityRef(), federationcontract.FederationReadRequest{Boundary: f.root, Intent: intent, PrepareObserve: prepare, Mode: mode, Purpose: [32]byte{9}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSourceHasOnlyOriginalServiceMethods(t *testing.T) {
	typ := reflect.TypeOf((*Operations)(nil))
	var got []string
	for i := 0; i < typ.NumMethod(); i++ {
		got = append(got, typ.Method(i).Name)
	}
	want := []string{"AuthorityRef", "OpenFederationReadPart", "ProjectFederationReadPart", "CheckFederationReadPart", "ReleaseFederationReadPart", "ObserveFederationReadPart", "CaptureFederationDenial", "CheckFederationDenial"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	var absent *Operations
	if absent.AuthorityRef() != "" {
		t.Fatal("nil source has identity")
	}
	if _, err := absent.OpenFederationReadPart(context.Background(), values.CallerContext{}, "", federationcontract.FederationReadRequest{}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal(err)
	}
}

func TestSourceAdmissionBorrowAndPrivateRetirement(t *testing.T) {
	f := newSourceFixture(t, false)
	other := newSourceFixture(t, false)
	ctx := context.Background()
	req := federationcontract.FederationReadRequest{Boundary: f.root, Intent: "query", Purpose: [32]byte{1}}
	if err := f.ops.admission.Acquire(); err != nil {
		t.Fatal(err)
	}
	defer f.ops.admission.Release()
	borrowed := access.WithAdmission(ctx, f.ops.admission)
	if _, err := f.ops.OpenFederationReadPart(ctx, f.caller, f.ops.AuthorityRef(), req); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("ordinary source bypassed admission", err)
	}
	local, err := OpenLocal(f.ops, borrowed, f.caller, req)
	if err != nil {
		t.Fatal("same envelope double admitted", err)
	}
	if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, f.ops.AuthorityRef(), local.Part().Basis); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("ordinary release borrowed admission", err)
	}
	if err := other.ops.admission.Acquire(); err != nil {
		t.Fatal(err)
	}
	defer other.ops.admission.Release()
	if _, err := other.ops.OpenFederationReadPart(borrowed, other.caller, other.ops.AuthorityRef(), req); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal("cross owner borrowed slot", err)
	}
	cancelled, cancel := context.WithCancel(borrowed)
	cancel()
	if _, err := f.ops.OpenFederationReadPart(cancelled, f.caller, f.ops.AuthorityRef(), req); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("cancelled context borrowed", err)
	}
	if err := local.Release(cancelled); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("cancelled retirement released", err)
	}
	if err := local.Release(ctx); err != nil {
		t.Fatal("retirement reentered full admission", err)
	}
	if err := local.Release(ctx); err != nil {
		t.Fatal("retirement not idempotent", err)
	}
	if err := local.Check(borrowed, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("released receipt live", err)
	}
	// Preserve the original local adapter's context.WithValue(nil,...) failure;
	// do not quietly broaden its contract while changing where release runs.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("nil retirement context no longer rejected")
			}
		}()
		_ = local.Release(nil)
	}()
	if _, err := f.ops.OpenFederationReadPart(nil, f.caller, f.ops.AuthorityRef(), req); !errors.Is(err, values.PublicInvalid) {
		t.Fatal("nil service context", err)
	}
}

func TestSourceFixedCutAndSnapshotRelease(t *testing.T) {
	// Preserves the former root Federation-part fixed-cut matrix at the source
	// owner, while Registry/query tests retain the exact resource accounting.
	for _, retained := range []bool{false, true} {
		for _, force := range []bool{false, true} {
			t.Run(map[bool]string{false: "current", true: "retained"}[retained]+map[bool]string{false: "-auto", true: "-forced"}[force], func(t *testing.T) {
				f := newSourceFixture(t, retained)
				for n := uint64(2); n < 7; n++ {
					f.create(t, n, float64(n))
				}
				mode := contract.QueryReadMode(0)
				if force {
					mode = contract.QueryCurrentVerified
				}
				p := f.open(t, "list", false, mode)
				projection := federationcontract.FederationProjectionRequest{Kind: "list", Element: f.root}
				ctx := context.Background()
				owner := f.ops.AuthorityRef()
				page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, projection, nil, 2)
				if err != nil || page.Complete || len(page.Records) != 2 || page.Continuation == nil {
					t.Fatal(page, err)
				}
				for _, record := range page.Records {
					if record.Attachment != nil {
						t.Fatal("list disclosed attachment", record)
					}
				}
				if page.Records[0].Pose == nil || len(page.Records[0].Path) == 0 {
					t.Fatal("list record omitted mutable DTO fields", page.Records[0])
				}
				page.Records[0].Pose.Translation.X = 999
				page.Records[0].Path[0] = sourceID(999)
				repeat, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, projection, nil, 2)
				if err != nil || repeat.Records[0].Pose == nil || repeat.Records[0].Pose.Translation.X != 2 || len(repeat.Records[0].Path) == 0 || repeat.Records[0].Path[0] != sourceID(2) {
					t.Fatal("projection aliases facts", repeat, err)
				}
				f.owner.Lock()
				read, err := f.ops.registry.ResolveReadLocked(f.caller, p.Basis)
				f.owner.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				f.move(t, 4, 42)
				checkErr := f.ops.CheckFederationReadPart(ctx, f.caller, owner, p.Basis, false)
				if !retained || force {
					if !errors.Is(checkErr, values.PublicInvalidContext) {
						t.Fatal("current source check rebased", checkErr)
					}
				} else if checkErr != nil {
					t.Fatal("retained continuation seal", checkErr)
				}
				if err := f.ops.CheckFederationReadPart(ctx, f.caller, owner, p.Basis, true); !errors.Is(err, values.PublicInvalidContext) {
					t.Fatal("initial seal accepted stale source", err)
				}
				next, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, projection, page.Continuation, 2)
				if !retained || force {
					if !errors.Is(err, values.PublicInvalidContext) {
						t.Fatal("current source rebased", next, err)
					}
				} else {
					if err != nil || len(next.Records) != 2 || next.Records[0].Pose.Translation.X != 4 || next.Records[0].Attachment != nil {
						t.Fatal("retained source changed", next, err)
					}
					last, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, projection, next.Continuation, 2)
					if err != nil || !last.Complete || len(last.Records) != 1 || last.Continuation != nil || last.Records[0].Attachment != nil {
						t.Fatal(last, err)
					}
				}
				if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, p.Basis); err != nil {
					t.Fatal(err)
				}
				if read.Snapshot != nil && f.ops.queries.SnapshotPresent(read.Snapshot) {
					t.Fatal("source release retained snapshot")
				}
				if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, p.Basis); err != nil {
					t.Fatal("expired release", err)
				}
			})
		}
	}
}

func TestSourceProjectionPrunesUnrelatedPendingMount(t *testing.T) {
	// Preserves the former root pending-mount projection case with the actual
	// Mount control path. Prepare is paused only after ApplyMount has published
	// its pending reservation, so neither source state nor a frontier is faked.
	parent := newSourceFixture(t, true)
	carrier := newSourceCarrierFixture(t)
	transport := connectSourceMounts(t, parent, carrier)
	mountID := sourceID(60)
	created, err := parent.elements.CreateAnchor(parent.caller.Subject, mountID, sourceTupleWithRadius(t, parent.root, 100, 10), "source-mount-anchor")
	if err != nil || created.Record == nil || created.Record.Anchor == nil {
		t.Fatal("create mount anchor", created, err)
	}
	callers := contract.LocalCallers{
		parent.ops.AuthorityRef():  parent.caller.Subject,
		carrier.ops.AuthorityRef(): carrier.caller.Subject,
	}
	establish := contract.MountCommand{
		Kind:                    contract.MountEstablish,
		MountAnchorUUID:         mountID,
		ExpectedElementRevision: created.Record.Anchor.Revision,
		CarrierAuthority:        carrier.ops.AuthorityRef(),
		CarrierNextHop:          "local:source-carrier",
		ParentNextHop:           "local:source-parent",
	}
	established, err := parent.mounts.ApplyMount(context.Background(), callers, "source-establish", establish)
	if err != nil || established.Decision != contract.MountCommitted || !established.Settled {
		t.Fatal("establish real mount", established, err)
	}

	stable := parent.open(t, "query", false, 0)
	if stable.ReadMode != contract.QueryRetained {
		t.Fatal("fixture did not issue retained receipt", stable.ReadMode)
	}
	request := federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{
		Scope: parent.root, Sphere: contract.QuerySphere{Radius: 1}, Selector: contract.QueryBoth,
	}}
	page, err := parent.ops.ProjectFederationReadPart(context.Background(), parent.caller, parent.ops.AuthorityRef(), stable.Basis, request, nil, 10)
	if err != nil || !page.Complete || page.Gap != contract.QueryNoGap || len(page.Frontiers) != 0 {
		t.Fatal("off-query mount not pruned", page, err)
	}

	mountRead, err := parent.mounts.ReadMount(parent.caller.Subject, mountID)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := parent.elements.ReadElement(parent.caller.Subject, mountID, nil)
	if err != nil || anchor.Record == nil || anchor.Record.Anchor == nil {
		t.Fatal("read mount anchor", anchor, err)
	}
	update := contract.MountCommand{
		Kind:                    contract.MountUpdate,
		MountAnchorUUID:         mountID,
		ExpectedElementRevision: anchor.Record.Anchor.Revision,
		ExpectedMountRevision:   mountRead.MountRevision,
		Final:                   sourceTupleWithRadius(t, parent.root, 0, 5),
	}
	entered := make(chan struct{}, 1)
	release := make(chan struct{}, 1)
	transport.pauseEntered, transport.pauseRelease = entered, release
	type mountResult struct {
		result contract.MountControlResult
		err    error
	}
	done := make(chan mountResult, 1)
	completed := false
	defer func() {
		if completed {
			return
		}
		select {
		case release <- struct{}{}:
		default:
		}
		select {
		case <-done:
		case <-time.After(4 * time.Second):
		}
	}()
	go func() {
		result, applyErr := parent.mounts.ApplyMount(context.Background(), callers, "source-pending-range", update)
		done <- mountResult{result: result, err: applyErr}
	}()
	select {
	case <-entered:
	case early := <-done:
		completed = true
		t.Fatal("mount update did not reach bounded Prepare barrier", early.result, early.err)
	case <-time.After(3 * time.Second):
		t.Fatal("mount update did not reach bounded Prepare barrier")
	}
	pending, err := parent.mounts.ReadMountOperation(parent.caller.Subject, mountID, "source-pending-range")
	if err != nil || pending.Decision != contract.MountPending || pending.Settled {
		t.Fatal("barrier did not retain a real pending operation", pending, err)
	}
	if err := parent.ops.CheckFederationReadPart(context.Background(), parent.caller, parent.ops.AuthorityRef(), stable.Basis, false); err != nil {
		t.Fatal("unrelated pending mount invalidated retained receipt", err)
	}

	pendingPart := parent.open(t, "query", false, 0)
	request.Query.Sphere.Center.X = 100
	page, err = parent.ops.ProjectFederationReadPart(context.Background(), parent.caller, parent.ops.AuthorityRef(), pendingPart.Basis, request, nil, 10)
	if err != nil || page.Complete || page.Gap != contract.QueryUnresolvedCoverage || len(page.Frontiers) != 0 {
		t.Fatal("intersecting pending mount fabricated coverage", page, err)
	}
	if err := parent.ops.ReleaseFederationReadPart(context.Background(), parent.caller, parent.ops.AuthorityRef(), stable.Basis); err != nil {
		t.Fatal(err)
	}
	if err := parent.ops.ReleaseFederationReadPart(context.Background(), parent.caller, parent.ops.AuthorityRef(), pendingPart.Basis); err != nil {
		t.Fatal(err)
	}
	release <- struct{}{}
	select {
	case applied := <-done:
		completed = true
		if applied.err != nil || applied.result.Decision != contract.MountCommitted || !applied.result.Settled {
			t.Fatal("real pending mount did not settle", applied.result, applied.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("real pending mount did not settle")
	}
}

func TestSourceIntentPayloadAndCallerSourceBinding(t *testing.T) {
	// Preserves the former root IntentPayloadAndBinding case against the real
	// source entry instead of a Core facade or private Registry state.
	f := newSourceFixture(t, true)
	ctx := context.Background()
	owner := f.ops.AuthorityRef()
	otherSource := newSourceFixture(t, false).ops.AuthorityRef()
	for _, intent := range []string{"read", "list", "query"} {
		t.Run(intent, func(t *testing.T) {
			part := f.open(t, intent, false, 0)
			projection := federationcontract.FederationProjectionRequest{Kind: intent, Element: f.root}
			if intent == "query" {
				projection.Element = ""
				projection.Query = &federationcontract.FederationQueryRequest{
					Scope: f.root, Sphere: contract.QuerySphere{Radius: 1}, Selector: contract.QueryBoth,
				}
			}
			page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, projection, nil, 2)
			if err != nil {
				t.Fatal(err)
			}
			switch intent {
			case "read":
				if len(page.Records) != 1 || page.Records[0].Attachment == nil || len(page.Matches) != 0 {
					t.Fatal("read payload shape", page)
				}
			case "list":
				if len(page.Records) != 0 || len(page.Matches) != 0 {
					t.Fatal("list payload shape", page)
				}
			case "query":
				if len(page.Records) != 0 || len(page.Matches) != 1 || page.Matches[0].ID != f.root {
					t.Fatal("query payload shape", page)
				}
			}

			otherCaller := f.caller
			otherCaller.Establishment[0] = 1
			if err := f.ops.CheckFederationReadPart(ctx, otherCaller, owner, part.Basis, false); !errors.Is(err, values.PublicDenied) {
				t.Fatal("caller substituted during check", err)
			}
			if err := f.ops.CheckFederationReadPart(ctx, f.caller, otherSource, part.Basis, false); !errors.Is(err, values.PublicDenied) {
				t.Fatal("source substituted during check", err)
			}
			if err := f.ops.ReleaseFederationReadPart(ctx, otherCaller, owner, part.Basis); !errors.Is(err, values.PublicDenied) {
				t.Fatal("caller released another part", err)
			}
			if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, otherSource, part.Basis); !errors.Is(err, values.PublicDenied) {
				t.Fatal("source released another part", err)
			}
			if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, part.Basis); err != nil {
				t.Fatal(err)
			}
			if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, part.Basis); err != nil {
				t.Fatal("release not idempotent", err)
			}
		})
	}
}

func TestSourceExactProjectionAndCursorConditions(t *testing.T) {
	// Preserves the former root ExactStepsAndCursorConditions case: exact
	// geometry is projected by the real source and every continuation binds
	// rotation's private components independently from Selector.
	f := newSourceFixture(t, true)
	f.create(t, 20, 2)
	f.create(t, 21, 10)
	rotation, err := contract.NewRotation(1, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := contract.NewPose(contract.Vec3{}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	part := f.open(t, "query", false, 0)
	request := federationcontract.FederationProjectionRequest{Kind: "query", Query: &federationcontract.FederationQueryRequest{
		Scope: f.root, Sphere: contract.QuerySphere{Center: contract.Vec3{Y: 3}}, Selector: contract.QueryEntities,
		ToBasis: []federationcontract.FederationPoseStep{{Pose: pose}},
	}}
	ctx := context.Background()
	owner := f.ops.AuthorityRef()
	page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, request, nil, 1)
	if err != nil || !page.Complete || len(page.Matches) != 1 || page.Matches[0].ID != sourceID(20) || len(page.Records) != 0 {
		t.Fatal("rotated tangent range omitted", page, err)
	}
	request.Query.Sphere = contract.QuerySphere{Radius: 100}
	page, err = f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, request, nil, 1)
	if err != nil || page.Continuation == nil {
		t.Fatal("query continuation missing", page, err)
	}
	identity, err := contract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	identityPose, err := contract.NewPose(contract.Vec3{}, identity)
	if err != nil {
		t.Fatal(err)
	}
	changed := *request.Query
	changed.ToBasis = []federationcontract.FederationPoseStep{{Pose: identityPose}}
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationProjectionRequest{Kind: "query", Query: &changed}, page.Continuation, 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("rotation not bound into cursor", err)
	}
	changed = *request.Query
	changed.Selector = contract.QueryBoth
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationProjectionRequest{Kind: "query", Query: &changed}, page.Continuation, 1); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("selector not bound into cursor", err)
	}
	if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, part.Basis); err != nil {
		t.Fatal(err)
	}
}

func TestSourcePathProjectionAndIntentIsolation(t *testing.T) {
	// Preserves the former root PathAndIntentIsolation recipe, including the
	// repeated up/down node that makes this exact Spine four elements long.
	f := newSourceFixture(t, false)
	a := f.createAnchor(t, 30, f.root, 0, 20)
	b := f.createAnchor(t, 31, a, 1, 10)
	e := f.createUnder(t, 32, b, 0)
	part := f.open(t, "query", false, 0)
	request := federationcontract.FederationProjectionRequest{Kind: "path", Path: &federationcontract.FederationPathRequest{
		Origin: b, Up: 1, Down: []values.ElementID{b, e},
	}}
	ctx := context.Background()
	owner := f.ops.AuthorityRef()
	page, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, request, nil, 1)
	if err != nil || page.Path == nil || page.Path.Element != e || page.Path.ActualUp != 1 || page.Path.RemainingUp != 0 || len(page.Path.RemainingDown) != 0 || len(page.Path.Spine) != 4 || len(page.Records) != 0 {
		t.Fatal("path recipe changed", page, err)
	}
	for _, kind := range []string{"read", "list"} {
		if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, federationcontract.FederationProjectionRequest{Kind: kind, Element: b}, nil, 1); !errors.Is(err, values.PublicDenied) {
			t.Fatal("query upgraded to "+kind, err)
		}
	}
	request.Element = b
	if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, part.Basis, request, nil, 1); !errors.Is(err, values.PublicInvalid) {
		t.Fatal("path union accepted extra arm", err)
	}
	if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, part.Basis); err != nil {
		t.Fatal(err)
	}
}

func TestSourceMissingRetainedSnapshotInvalidatesBasis(t *testing.T) {
	f := newSourceFixtureWithRoot(t, true, false)
	f.caller = values.CallerContext{Subject: values.PrincipalRef{Realm: "oracle-realm", Principal: "oracle"}}
	f.root = "10000000-0000-4000-8000-000000016b48"
	if _, err := f.elements.CreateRoot(f.caller.Subject, f.root, "federation-root"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	part, err := f.ops.OpenFederationReadPart(ctx, f.caller, f.ops.AuthorityRef(), federationcontract.FederationReadRequest{Boundary: f.root, Intent: "read", Purpose: [32]byte{1}})
	if err != nil || part.ReadMode != contract.QueryRetained {
		t.Fatal(part, err)
	}
	f.owner.Lock()
	basis, err := f.ops.registry.ResolveReadLocked(f.caller, part.Basis)
	f.owner.Unlock()
	if err != nil || basis.Snapshot == nil {
		t.Fatal("no real retained snapshot", err)
	}
	// Exact timer/borrow/view/byte reclamation belongs to the snapshot owner.
	// This source regression proves that a missing retained state is not replaced
	// by current data and its source receipt can still be released exactly once.
	f.ops.queries.DiscardOwnedSnapshot(*basis.Snapshot)
	if err := f.ops.CheckFederationReadPart(ctx, f.caller, f.ops.AuthorityRef(), part.Basis, false); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("missing snapshot became current read", err)
	}
	for i := 0; i < 2; i++ {
		if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, f.ops.AuthorityRef(), part.Basis); err != nil {
			t.Fatal("release after snapshot loss", err)
		}
	}
	f.owner.Lock()
	_, err = f.ops.registry.ResolveReadLocked(f.caller, part.Basis)
	f.owner.Unlock()
	if !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("basis survived release", err)
	}
}

func TestSourceReadAtHasIndependentAuthorization(t *testing.T) {
	// Preserves the former root ReadAt matrix: the extra Read grant is bound
	// independently, while spatial facts and Attachment remain at one cut.
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "retained"}[retained], func(t *testing.T) {
			f := newSourceFixture(t, retained)
			f.create(t, 2, 2)
			f.auth.denyRead = true
			p := f.open(t, "query", false, 0)
			ctx := context.Background()
			owner := f.ops.AuthorityRef()
			request := federationcontract.FederationProjectionRequest{Kind: "read-at", Element: sourceID(2)}
			if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, request, nil, 1); !errors.Is(err, values.PublicDenied) {
				t.Fatal("query authorized read", err)
			}
			f.auth.denyRead = false
			first, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, request, nil, 1)
			if err != nil || len(first.Records) != 1 || first.Records[0].Pose == nil || first.Records[0].Attachment == nil || !first.Records[0].Revision.Valid() {
				t.Fatal("independently authorized exact read", first, err)
			}
			f.auth.readEpoch++
			if _, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, request, nil, 1); !errors.Is(err, values.PublicInvalidContext) {
				t.Fatal("read view silently replaced", err)
			}
			if err := f.ops.CheckFederationReadPart(ctx, f.caller, owner, p.Basis, false); err != nil {
				t.Fatal("read invalidated independent query view", err)
			}
			f.auth.readEpoch = 0
			f.move(t, 2, 42)
			old, err := f.ops.ProjectFederationReadPart(ctx, f.caller, owner, p.Basis, request, nil, 1)
			if retained {
				if err != nil || len(old.Records) != 1 || old.Records[0].Pose == nil || old.Records[0].Pose.Translation.X != 2 || old.Records[0].Revision != first.Records[0].Revision || !reflect.DeepEqual(old.Records[0].Attachment, first.Records[0].Attachment) {
					t.Fatal("historical exact read mixed latest state", old, err)
				}
			} else if !errors.Is(err, values.PublicInvalidContext) {
				t.Fatal("current basis silently recaptured", old, err)
			}
			other := f.caller
			other.Establishment[0] = 1
			if err := f.ops.ReleaseFederationReadPart(ctx, other, owner, p.Basis); !errors.Is(err, values.PublicDenied) {
				t.Fatal("other caller released", err)
			}
			if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, "52000000-0000-4000-8000-000000000099", p.Basis); !errors.Is(err, values.PublicDenied) {
				t.Fatal("other source released", err)
			}
			if err := f.ops.ReleaseFederationReadPart(ctx, f.caller, owner, p.Basis); err != nil {
				t.Fatal(err)
			}
		})
	}
}
