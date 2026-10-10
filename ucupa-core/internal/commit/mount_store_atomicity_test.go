package commit_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func TestMountDurableEveryControlCommitFailureRecoversOriginalIntent(t *testing.T) {
	stages := []struct {
		name, owner string
		ordinal     int
	}{
		{"reserve", "a", 1}, {"prepare", "b", 1}, {"decide", "a", 2}, {"close", "b", 2}, {"settle", "a", 3},
	}
	for _, change := range []string{"establish", "range", "revoke"} {
		for _, stage := range stages {
			if change == "revoke" && (stage.name == "prepare" || stage.name == "decide") {
				continue
			}
			for _, after := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/after=%v", change, stage.name, after), func(t *testing.T) {
					f := newMountStoreFixture(t, change != "establish")
					command := f.command
					if change == "range" {
						command = f.updateCommand(t, 50)
					}
					if change == "revoke" {
						read, err := f.a.mounts.ReadMount(rangeStoreActor, f.anchor)
						if err != nil {
							t.Fatal(err)
						}
						command = corecontract.MountCommand{Kind: corecontract.MountRevoke, MountAnchorUUID: f.anchor, ExpectedMountRevision: read.MountRevision}
					}
					ordinal := stage.ordinal
					if change == "revoke" && stage.name == "close" {
						ordinal = 1
					}
					if change == "revoke" && stage.name == "settle" {
						ordinal = 2
					}
					owner := f.a.owner
					if stage.owner == "b" {
						owner = f.b.owner
					}
					failure := errors.New("mount durable fault")
					fault := commit.InjectMountStoreFailureForTest(owner, after, ordinal, failure)
					id := corecontract.OperationID("mount-store-fault-" + change)
					result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, id, command)
					if fault.Calls() != ordinal {
						t.Fatalf("fault did not reach its stage: calls=%d want=%d result=%#v err=%v", fault.Calls(), ordinal, result, err)
					}
					if stage.owner == "a" {
						if !errors.Is(err, corecontract.ErrStore) || !errors.Is(err, failure) || !reflect.DeepEqual(result, corecontract.MountControlResult{}) {
							t.Fatalf("local uncertain response: %#v %v", result, err)
						}
					} else if err != nil || result.BlockedBy != corecontract.MountBlockedPeerStore || result.Decision == corecontract.MountAborted {
						t.Fatalf("peer uncertain response must retain intent: %#v %v", result, err)
					}
					if err := commit.ValidateCompleteStateForTest(f.a.owner); err != nil {
						t.Fatalf("A after fault: %v", err)
					}
					if err := commit.ValidateCompleteStateForTest(f.b.owner); err != nil {
						t.Fatalf("B after fault: %v", err)
					}
					f.reopen(t)
					completed := f.apply(t, id, command)
					afterA, afterB := commit.CaptureCompleteStateForTest(f.a.owner), commit.CaptureCompleteStateForTest(f.b.owner)
					f.reopen(t)
					replayed := f.apply(t, id, command)
					if !reflect.DeepEqual(replayed, completed) || !commit.MatchesCompleteStateForTest(f.a.owner, afterA) || !commit.MatchesCompleteStateForTest(f.b.owner, afterB) {
						t.Fatal("exact retry changed result or state")
					}
				})
			}
		}
	}
}

func TestMountDurableAddressRepairWhilePreparedSurvivesReopen(t *testing.T) {
	f := newMountStoreFixture(t, false)
	f.aTransport.afterPrepare = func() { commit.InjectWriteFailureForTest(f.a.owner, false, errors.New("stop before A decision")) }
	_, err := f.a.mounts.ApplyMount(context.Background(), f.callers, "mount-store-prepared", f.command)
	if !errors.Is(err, corecontract.ErrStore) || mountStoreFacts(f.b).Mount.Incoming().Gate != mountfact.GatePreparing {
		t.Fatalf("prepare pause: %v", err)
	}
	request := mountStoreOperation(t, f.a, "mount-store-prepared").Request
	beforeA, beforeB := mountStoreFacts(f.a).Mount.Record(f.anchor), mountStoreFacts(f.b).Mount.Incoming()
	for _, core := range []*mountStoreOwner{f.a, f.b} {
		mount, err := core.mounts.ReadMount(rangeStoreActor, f.anchor)
		if err != nil {
			t.Fatal(err)
		}
		next := f.command.CarrierNextHop
		if core == f.a {
			next = "local:b-new"
		} else {
			next = "local:a-new"
		}
		if err := core.mounts.ConfigureMountTransport(nil); err != nil {
			t.Fatal(err)
		}
		result, err := core.mounts.UpdateMountNextHop(rangeStoreActor, "mount-store-address", f.anchor, mount.MountKey, mount.NextHopRevision, next)
		if err != nil || !result.Changed {
			t.Fatalf("offline address repair: %#v %v", result, err)
		}
	}
	if mountStoreFacts(f.a).Mount.Record(f.anchor).Revision != beforeA.Revision || mountStoreFacts(f.b).Mount.Incoming().Gate != beforeB.Gate || mountStoreOperation(t, f.a, "mount-store-prepared").Request != request {
		t.Fatal("address repair modified control intent or write gate")
	}
	f.reopen(t)
	delete(f.aTransport.endpoints, "local:b")
	delete(f.bTransport.endpoints, "local:a")
	f.apply(t, "mount-store-prepared", f.command)
	if mountStoreFacts(f.a).Mount.Record(f.anchor).Contact.NextHop != "local:b-new" || mountStoreFacts(f.b).Mount.Incoming().Contact.NextHop != "local:a-new" {
		t.Fatal("recovery overwrote explicit new addresses")
	}
}
