package commit_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestMountStoreRejectsMalformedOrImpossibleHistoryWithoutWriting(t *testing.T) {
	cases := []string{
		"missing journal",
		"journal sequence",
		"nested journal bucket",
		"event gap",
		"unknown event",
		"missing before cut",
		"wrong after cut",
		"out of order",
		"commit references another control",
		"commit duplicates ordinary outcome",
		"ordinary and control operation collision",
		"undeclared event field",
		"invalid specified address",
		"forged own expected revision",
		"changed decision evidence",
		"new decision without authorization",
		"success altered to abort",
		"unknown JSON field",
	}
	for _, test := range cases {
		t.Run(test, func(t *testing.T) {
			f := newMountStoreFixture(t, true)
			closeMountStore(t, f.a)
			commit.CorruptMountJournalForTest(t, f.aPath, test)
			before, err := os.ReadFile(f.aPath)
			if err != nil {
				t.Fatal(err)
			}
			opened, err := commit.OpenDurable(f.aPath, nil)
			if opened != nil {
				_ = opened.CloseStore()
			}
			if opened != nil || !errors.Is(err, corecontract.ErrStore) {
				t.Fatalf("accepted impossible mounted Store: %v", err)
			}
			after, err := os.ReadFile(f.aPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("failed validation modified Store")
			}
		})
	}
}

func TestMountHistoryRejectsCurrentSnapshotWithoutReachableEvidence(t *testing.T) {
	f := newMountStoreFixture(t, true)
	for _, fault := range []string{"lost contact", "changed address", "original result altered", "lost original operation", "lost final event"} {
		t.Run(fault, func(t *testing.T) {
			same, err := commit.CheckMountHistoryFaultForTest(f.a.owner, f.anchor, fault)
			if same {
				t.Fatal("complete equality ignored control evidence")
			}
			if err == nil {
				t.Fatal("unreachable control snapshot accepted")
			}
		})
	}
	// A valid but different third Store result is not a permissible resolution
	// of the caller's uncertain previous/next transaction.
	read, err := f.a.mounts.ReadMount(rangeStoreActor, f.anchor)
	if err != nil {
		t.Fatal(err)
	}
	commit.InjectThirdMountStoreResultForTest(f.a.owner, rangeStoreActor, f.anchor, read.MountKey)
	result, err := f.a.mounts.UpdateMountNextHop(rangeStoreActor, "uncertain-address", f.anchor, read.MountKey, read.NextHopRevision, "local:b-new")
	if !errors.Is(err, corecontract.ErrStore) || result != (corecontract.NextHopUpdateResult{}) || mountStoreAvailable(f.a) {
		t.Fatalf("third result did not poison: %#v %v", result, err)
	}
	if _, err := f.a.mounts.ReadMount(rangeStoreActor, f.anchor); !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("poisoned read: %v", err)
	}
	if _, err := f.a.mounts.RecoverMounts(context.Background()); !errors.Is(err, corecontract.ErrStore) {
		t.Fatalf("poisoned recovery: %v", err)
	}
	closeMountStore(t, f.a)
	f.a = openMountStore(t, f.aPath)
	if !commit.MountAddressRetainedForTest(f.a.owner, "third-outcome") {
		t.Fatal("fresh reopen did not restore durable third result")
	}
}
