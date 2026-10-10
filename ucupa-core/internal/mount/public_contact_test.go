package mount

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestPublicMountContactRepairWhilePending(t *testing.T) {
	f := newMountPublicFixture(t, false)
	f.transport.afterPrepare = func() { f.a.auth.validate = values.AuthorizationUnavailable }
	_, _ = f.a.mounts.ApplyMountControl(context.Background(), f.caller, f.view, "paused", f.command, f.transport)
	f.a.owner.RLock()
	parentOp, _ := f.a.owner.MountOperationLocked("paused")
	f.a.owner.RUnlock()
	f.b.owner.RLock()
	incoming := f.b.owner.FactsLocked().Mount.Incoming()
	f.b.owner.RUnlock()
	if parentOp.Decision != corecontract.MountPending || !commit.ValidMountAttemptKey(incoming.Preparing) {
		t.Fatal("fixture not paused")
	}
	f.a.auth.validate = values.AuthorizationValid
	for _, direction := range []struct {
		owner    *mountTestCore
		incoming bool
		address  values.NextHop
	}{{f.a, false, "local:new-b"}, {f.b, true, "local:new-a"}} {
		o := direction.owner
		contact, err := o.mounts.InspectMountContact(context.Background(), f.caller, f.anchor, direction.incoming)
		if err != nil {
			t.Fatal(err)
		}
		o.owner.RLock()
		cut := o.owner.FactsLocked().CurrentCut()
		o.owner.RUnlock()
		changed, err := o.mounts.RepairMountContact(context.Background(), f.caller, "repair", contact.Element, direction.incoming, contact.Revision, direction.address)
		o.owner.RLock()
		after := o.owner.FactsLocked().CurrentCut()
		o.owner.RUnlock()
		if err != nil || changed.Revision == contact.Revision || changed.NextHop != direction.address || after == cut {
			t.Fatalf("repair: %+v %v", changed, err)
		}
		retained := mountProbe(o.owner, "repair", f.anchor)
		replayed, err := o.mounts.RepairMountContact(context.Background(), f.caller, "repair", contact.Element, direction.incoming, contact.Revision, direction.address)
		if err != nil || replayed != changed || !reflectEqual(retained, mountProbe(o.owner, "repair", f.anchor)) {
			t.Fatalf("replay: %+v %v", replayed, err)
		}
		if _, err := o.mounts.RepairMountContact(context.Background(), f.caller, "stale", contact.Element, direction.incoming, contact.Revision, "local:bad"); !errors.Is(err, values.PublicConflict) {
			t.Fatalf("old contact: %v", err)
		}
		if _, err := o.mounts.RepairMountContact(context.Background(), f.caller, "repair", contact.Element, direction.incoming, contact.Revision, "local:bad"); !errors.Is(err, values.PublicConflict) {
			t.Fatalf("modified replay: %v", err)
		}
		if _, err := o.mounts.RepairMountContact(context.Background(), f.caller, "repair", contact.Element, !direction.incoming, contact.Revision, direction.address); !errors.Is(err, values.PublicConflict) {
			t.Fatalf("direction replay: %v", err)
		}
		other := f.caller
		other.Subject.Principal = "other"
		if _, err := o.mounts.RepairMountContact(context.Background(), other, "repair", contact.Element, direction.incoming, contact.Revision, direction.address); !errors.Is(err, values.PublicConflict) {
			t.Fatalf("other caller: %v", err)
		}
		if !reflectEqual(retained, mountProbe(o.owner, "repair", f.anchor)) {
			t.Fatal("rejected repair mutated facts")
		}
	}
	f.a.owner.RLock()
	parentOp, _ = f.a.owner.MountOperationLocked("paused")
	f.a.owner.RUnlock()
	f.b.owner.RLock()
	incoming = f.b.owner.FactsLocked().Mount.Incoming()
	f.b.owner.RUnlock()
	if parentOp.Decision != corecontract.MountPending || !commit.ValidMountAttemptKey(incoming.Preparing) {
		t.Fatal("repair changed pending decision")
	}
}

func TestPublicMountContactAuthorizationBeforeMutation(t *testing.T) {
	f := newMountPublicFixture(t, false)
	f.apply(t, "active", f.command)
	r, err := f.a.mounts.InspectMountContact(context.Background(), f.caller, f.anchor, false)
	if err != nil {
		t.Fatal(err)
	}
	before := mountProbe(f.a.owner, "aba", f.anchor)
	f.a.auth.validate = values.AuthorizationChanged
	if _, err := f.a.mounts.RepairMountContact(context.Background(), f.caller, "aba", r.Element, false, r.Revision, "local:replacement"); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal(err)
	}
	if !reflectEqual(before, mountProbe(f.a.owner, "aba", f.anchor)) {
		t.Fatal("ABA repair wrote")
	}
}
