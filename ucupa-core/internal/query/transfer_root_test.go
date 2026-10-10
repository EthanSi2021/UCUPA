package query

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func TestRootTransferQueriesNeverReportOldSpaceAsEmpty(t *testing.T) {
	ttl := time.Hour
	source := newQueryFixture(t, contract.Config{SnapshotRetention: &ttl}, true)
	old, err := source.queries.QueryRegion(queryActor, source.region, nil, 1, nil)
	if err != nil || old.Snapshot == nil || old.Continuation == nil {
		t.Fatal("source page", err)
	}
	defer source.queries.ReleaseReadSnapshot(queryActor, *old.Snapshot)
	root, err := source.elements.ReadElement(queryActor, source.region.StartAnchorID, nil)
	if err != nil || root.Record == nil {
		t.Fatal(err)
	}
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	target, err := commit.CreateDurable(filepath.Join(t.TempDir(), "root-target.db"), authority, [32]byte{20}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer target.CloseStore()
	settings, err := NormalizeConfig(contract.Config{SnapshotRetention: &ttl})
	if err != nil {
		t.Fatal(err)
	}
	authorizer := queryAuthorize(func(values.PrincipalRef, ...values.Requirement) bool { return true })
	queries := New(target, authorizer, [32]byte{24}, settings)
	defer queries.Close()
	elements := element.New(target, authorizer)
	i := transferfact.Intent{Profile: transferfact.RootSuccessor, Operation: "query-root-transfer", Actor: queryActor,
		Source: source.owner.AuthorityRef(), Target: authority, Boundary: root.Record.Anchor.ID, Expected: root.Record.Anchor.Revision,
		SourceAddress: "local:source", TargetAddress: "local:target"}
	source.owner.Lock()
	intent, err := source.owner.BeginTransferIntentLocked(i, 4096)
	if err != nil {
		source.owner.Unlock()
		t.Fatal(err)
	}
	fenced, err := source.owner.FenceTransferSourceLocked(queryActor, i.Operation, intent.Revision, transferfact.ParentHold{})
	if err != nil {
		source.owner.Unlock()
		t.Fatal(err)
	}
	f, raw, err := source.owner.TransferSourceFenceLocked(queryActor, i.Operation)
	source.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	checkOld := func() {
		t.Helper()
		p, err := source.queries.QueryRegion(queryActor, source.region, nil, 256, nil)
		if !errors.Is(err, contract.ErrUnavailable) || len(p.Matches) != 0 {
			t.Fatal("retired root became complete empty space", p, err)
		}
		if _, err := source.elements.ReadElement(queryActor, i.Boundary, nil); !errors.Is(err, contract.ErrUnavailable) {
			t.Fatal("old root absence", err)
		}
		if _, err := source.queries.ReadElementSnapshot(queryActor, source.ids[0], *old.Snapshot); !errors.Is(err, contract.ErrUnavailable) {
			t.Fatal("retained old fact disclosed", err)
		}
		if _, err := source.queries.QueryRegion(queryActor, source.region, nil, 1, old.Continuation); !errors.Is(err, contract.ErrUnavailable) && !errors.Is(err, contract.ErrReadCutExpired) {
			t.Fatal("old cursor continued", err)
		}
	}
	checkOld()
	target.Lock()
	accepted, err := target.AcceptTransferTargetLocked(queryActor, f, raw, nil, transferfact.ParentHold{})
	target.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// Acceptance itself is not an active root or a readable partial install.
	if r, err := elements.ReadElement(queryActor, i.Boundary, nil); err != nil || r.Record != nil {
		t.Fatal("acceptance exposed root", r, err)
	}
	source.owner.Lock()
	decision, err := source.owner.CommitTransferSourceLocked(queryActor, accepted, fenced.Revision)
	source.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	target.Lock()
	installed, err := target.InstallTransferTargetLocked(decision)
	target.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	source.owner.Lock()
	_, err = source.owner.SettleTransferSourceLocked(transferfact.CoreCompletion{Installed: transferfact.CoreInstallation{Source: installed}, At: installed.At, Revision: installed.Revision})
	source.owner.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	checkOld()
	page, err := queries.QueryRegion(queryActor, source.region, nil, 1, nil)
	if err != nil || page.Snapshot == nil || page.Continuation == nil || len(page.Matches) != 1 {
		t.Fatal("target pagination", page, err)
	}
	defer queries.ReleaseReadSnapshot(queryActor, *page.Snapshot)
	read, err := queries.ReadElementSnapshot(queryActor, i.Boundary, *page.Snapshot)
	if err != nil || read.Record == nil || read.Record.Anchor.ID != i.Boundary || read.Record.Anchor.ParentAnchorID != nil || read.Record.Anchor.LocalPose != nil || read.Record.Anchor.Range.Kind() != contract.RangeUnboundedRoot {
		t.Fatal("target root snapshot", read, err)
	}
	seen := map[values.ElementID]bool{page.Matches[0].ElementID: true}
	for page.Continuation != nil {
		page, err = queries.QueryRegion(queryActor, source.region, nil, 1, page.Continuation)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page.Matches {
			if seen[m.ElementID] {
				t.Fatal("duplicate page")
			}
			seen[m.ElementID] = true
		}
	}
	if page.Status != contract.QueryComplete || len(seen) != len(source.ids) {
		t.Fatal("root descendants lost")
	}
}
