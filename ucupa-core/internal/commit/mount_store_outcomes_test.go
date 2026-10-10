package commit_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestMountStoreInterleavedFactsAndNoCutOutcomesReplay(t *testing.T) {
	f := newMountStoreFixture(t, true)
	child, err := f.b.elements.CreateEntity(rangeStoreActor, rangeStoreID(902), rangeStoreTuple(t, f.anchor, 20, 1), "mount-store-child")
	if err != nil || child.Status != corecontract.MutationCommitted {
		t.Fatalf("B child: %#v %v", child, err)
	}
	update := f.updateCommand(t, 70)
	rangeResult := f.apply(t, "mount-store-range", update)
	if rangeResult.Record == nil {
		t.Fatal("range update lost original Element result")
	}
	// An ordinary write must accept the Revision actually issued by a control
	// commit; it may not assume every Element Revision has an Element event.
	poseOnly := rangeStoreTuple(t, f.root, 1, 70)
	pose, err := f.a.elements.UpdateAnchor(rangeStoreActor, f.anchor, poseOnly, rangeResult.Record.Anchor.Revision, "mount-store-pose")
	if err != nil || pose.Status != corecontract.MutationCommitted {
		t.Fatalf("ordinary pose after control: %#v %v", pose, err)
	}
	mount, err := f.a.mounts.ReadMount(rangeStoreActor, f.anchor)
	if err != nil {
		t.Fatal(err)
	}
	unchangedCommand := corecontract.MountCommand{Kind: corecontract.MountUpdate, MountAnchorUUID: f.anchor, ExpectedElementRevision: pose.Record.Anchor.Revision, ExpectedMountRevision: mount.MountRevision, Final: poseOnly}
	before := mountStoreFacts(f.a).CurrentCut()
	unchanged := f.apply(t, "mount-store-unchanged", unchangedCommand)
	if unchanged.Changed || mountStoreFacts(f.a).CurrentCut() != before {
		t.Fatal("unchanged control advanced fact cut")
	}
	rejected, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "mount-store-rejected", update)
	if err != nil || rejected.Decision != corecontract.MountAborted || rejected.AbortCause != corecontract.MountAbortRevisionConflict || !rejected.Settled || mountStoreFacts(f.a).CurrentCut() != before {
		t.Fatalf("stale control: %#v %v", rejected, err)
	}
	address, err := f.a.mounts.UpdateMountNextHop(rangeStoreActor, "mount-store-new-address", f.anchor, mount.MountKey, mount.NextHopRevision, "local:b-new")
	if err != nil || !address.Changed {
		t.Fatal(err)
	}
	before = mountStoreFacts(f.a).CurrentCut()
	noAddress, err := f.a.mounts.UpdateMountNextHop(rangeStoreActor, "mount-store-same-address", f.anchor, mount.MountKey, address.NextHopRevision, "local:b-new")
	if err != nil || noAddress.Changed || mountStoreFacts(f.a).CurrentCut() != before {
		t.Fatalf("same address: %#v %v", noAddress, err)
	}
	badAddress, err := f.a.mounts.UpdateMountNextHop(rangeStoreActor, "mount-store-stale-address", f.anchor, mount.MountKey, mount.NextHopRevision, "local:b")
	if !errors.Is(err, corecontract.ErrRevisionConflict) || badAddress != (corecontract.NextHopUpdateResult{}) || mountStoreFacts(f.a).CurrentCut() != before {
		t.Fatalf("stale address: %#v %v", badAddress, err)
	}
	want := commit.CaptureCompleteStateForTest(f.a.owner)
	for _, id := range []corecontract.OperationID{"mount-store-unchanged", "mount-store-rejected"} {
		if err := commit.ValidateChangedMountOutcomeForTest(f.a.owner, id); err == nil {
			t.Fatalf("no-cut original outcome %q was not replay-validated", id)
		}
	}
	f.reopen(t)
	if got := f.apply(t, "mount-store-unchanged", unchangedCommand); !reflect.DeepEqual(got, unchanged) {
		t.Fatal("unchanged control original result changed")
	}
	got, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "mount-store-rejected", update)
	if err != nil || !reflect.DeepEqual(got, rejected) {
		t.Fatalf("rejected original result changed: %#v %v", got, err)
	}
	gotAddress, err := f.a.mounts.UpdateMountNextHop(rangeStoreActor, "mount-store-same-address", f.anchor, mount.MountKey, address.NextHopRevision, "local:b-new")
	if err != nil || gotAddress != noAddress {
		t.Fatalf("unchanged address replay: %#v %v", gotAddress, err)
	}
	_, err = f.a.mounts.UpdateMountNextHop(rangeStoreActor, "mount-store-stale-address", f.anchor, mount.MountKey, mount.NextHopRevision, "local:b")
	if !errors.Is(err, corecontract.ErrRevisionConflict) || !commit.MatchesCompleteStateForTest(f.a.owner, want) {
		t.Fatalf("replay changed cut/history: %v", err)
	}
}

func TestMountStoreNoCutControlOutcomesAreAtomic(t *testing.T) {
	for _, after := range []bool{false, true} {
		for _, rejected := range []bool{false, true} {
			t.Run(fmt.Sprintf("after=%v/rejected=%v", after, rejected), func(t *testing.T) {
				f := newMountStoreFixture(t, true)
				command := f.updateCommand(t, 100)
				if rejected {
					// This is an actually issued but stale Element token.
					f.apply(t, "advance-range", f.updateCommand(t, 80))
				}
				before := mountStoreFacts(f.a).CurrentCut()
				failure := errors.New("no-cut control Store response")
				commit.InjectMountStoreFailureForTest(f.a.owner, after, 1, failure)
				result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "no-cut-fault", command)
				if !errors.Is(err, corecontract.ErrStore) || !reflect.DeepEqual(result, corecontract.MountControlResult{}) {
					t.Fatalf("uncertain no-cut result: %#v %v", result, err)
				}
				if mountStoreFacts(f.a).CurrentCut() != before {
					t.Fatal("no-cut outcome advanced cut")
				}
				f.a.owner.RLock()
				_, retained := f.a.owner.MountOperationLocked("no-cut-fault")
				f.a.owner.RUnlock()
				if retained != after {
					t.Fatal("retained no-cut outcome was not atomic")
				}
				f.reopen(t)
				result, err = f.a.mounts.ApplyMount(context.Background(), f.callers, "no-cut-fault", command)
				if err != nil || !result.Settled || result.Changed || (result.Decision == corecontract.MountAborted) != rejected {
					t.Fatalf("no-cut retry: %#v %v", result, err)
				}
			})
		}
	}
}

func TestMountStoreAddressOutcomesAreAtomicOnBothOwners(t *testing.T) {
	for _, incoming := range []bool{false, true} {
		for _, after := range []bool{false, true} {
			for _, outcome := range []string{"changed", "unchanged", "rejected"} {
				t.Run(fmt.Sprintf("incoming=%v/after=%v/%s", incoming, after, outcome), func(t *testing.T) {
					f := newMountStoreFixture(t, true)
					core := f.a
					if incoming {
						core = f.b
					}
					old, err := core.mounts.ReadMount(rangeStoreActor, f.anchor)
					if err != nil {
						t.Fatal(err)
					}
					expected, next := old.NextHopRevision, old.NextHop
					if outcome == "changed" {
						next += "-new"
					}
					if outcome == "rejected" {
						if _, err := core.mounts.UpdateMountNextHop(rangeStoreActor, "earlier-address", f.anchor, old.MountKey, expected, next+"-new"); err != nil {
							t.Fatal(err)
						}
					}
					previous := commit.CaptureCompleteStateForTest(core.owner)
					failure := errors.New("address commit response")
					commit.InjectMountStoreFailureForTest(core.owner, after, 1, failure)
					id := corecontract.OperationID(string([]byte{0, 0xff, 'a', 0x80}))
					result, err := core.mounts.UpdateMountNextHop(rangeStoreActor, id, f.anchor, old.MountKey, expected, next)
					if !errors.Is(err, corecontract.ErrStore) || !errors.Is(err, failure) || result != (corecontract.NextHopUpdateResult{}) {
						t.Fatalf("uncertain address: %#v %v", result, err)
					}
					retained := commit.MountAddressRetainedForTest(core.owner, id)
					if retained != after || (!after && !commit.MatchesCompleteStateForTest(core.owner, previous)) {
						t.Fatal("address/outcome atomicity failed")
					}
					f.reopen(t)
					core = f.a
					if incoming {
						core = f.b
					}
					result, err = core.mounts.UpdateMountNextHop(rangeStoreActor, id, f.anchor, old.MountKey, expected, next)
					if outcome == "rejected" {
						if !errors.Is(err, corecontract.ErrRevisionConflict) || result != (corecontract.NextHopUpdateResult{}) {
							t.Fatalf("retained address rejection: %#v %v", result, err)
						}
					} else if err != nil || result.Changed != (outcome == "changed") {
						t.Fatalf("address result: %#v %v", result, err)
					}
					beforeRetry := commit.CaptureCompleteStateForTest(core.owner)
					f.reopen(t)
					core = f.a
					if incoming {
						core = f.b
					}
					again, againErr := core.mounts.UpdateMountNextHop(rangeStoreActor, id, f.anchor, old.MountKey, expected, next)
					if again != result || !errors.Is(againErr, err) || !commit.MatchesCompleteStateForTest(core.owner, beforeRetry) {
						t.Fatal("opaque address OperationID did not exactly replay")
					}
				})
			}
		}
	}
}
