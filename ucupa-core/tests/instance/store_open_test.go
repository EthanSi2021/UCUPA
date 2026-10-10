package instance_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

var storeOpenActor = values.PrincipalRef{Realm: "store-realm", Principal: "store-writer"}

func TestDurableStoreIsExclusiveAndCreateNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock.db")
	spatial := createDurable(t, path)
	rootID := values.ElementID("20000000-0000-4000-8000-000000000014")
	root, err := spatial.Elements().CreateRoot(storeOpenActor, rootID, "locked-root")
	if err != nil || root.Status != corecontract.MutationCommitted || root.Record == nil {
		t.Fatalf("seed locked Store: %#v err=%v", root, err)
	}

	opened, err := core.OpenDurable(instancePolicyContact{durableAllow{}}, path, corecontract.Config{})
	if !errors.Is(err, corecontract.ErrStore) || opened != nil {
		t.Fatalf("concurrent OpenDurable: core=%#v err=%v", opened, err)
	}
	created, err := core.CreateDurable(instancePolicyContact{durableAllow{}}, instanceContact, path, corecontract.Config{})
	if !errors.Is(err, corecontract.ErrStore) || created != nil {
		t.Fatalf("CreateDurable over existing Store: core=%#v err=%v", created, err)
	}
	closeDurable(t, spatial)

	reopened := openDurable(t, path)
	defer closeDurable(t, reopened)
	read, err := reopened.Elements().ReadElement(storeOpenActor, rootID, nil)
	if err != nil || read.State != corecontract.ExistencePresent || read.Record == nil ||
		!read.Record.Anchor.Revision.Equal(root.Record.Anchor.Revision) {
		t.Fatalf("existing Store was changed: %#v err=%v", read, err)
	}
}

func TestOpenDurableNeverCreatesOrAcceptsEmptyFile(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "missing.db")
	opened, err := core.OpenDurable(instancePolicyContact{durableAllow{}}, missing, corecontract.Config{})
	if !errors.Is(err, corecontract.ErrStore) || opened != nil {
		t.Fatalf("missing Store: core=%#v err=%v", opened, err)
	}
	if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("OpenDurable created missing path: %v", statErr)
	}

	empty := filepath.Join(directory, "empty.db")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err = core.OpenDurable(instancePolicyContact{durableAllow{}}, empty, corecontract.Config{})
	if !errors.Is(err, corecontract.ErrStore) || opened != nil {
		t.Fatalf("empty Store: core=%#v err=%v", opened, err)
	}
	info, statErr := os.Stat(empty)
	if statErr != nil {
		t.Fatalf("stat empty Store: %v", statErr)
	}
	if info.Size() != 0 {
		t.Fatalf("OpenDurable changed empty Store: size=%d", info.Size())
	}
}
