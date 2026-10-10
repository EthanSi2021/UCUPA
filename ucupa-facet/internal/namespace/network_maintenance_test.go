package namespace

import (
	"context"
	"errors"
	"testing"

	"ucupa/ucupa-facet/contract"
)

func TestNetworkContactMaintenanceCanceledWaitNeverDisclosesOrCommits(t *testing.T) {
	for _, operation := range []string{"inspect", "update", "recover"} {
		t.Run(operation, func(t *testing.T) {
			x, p := networkAcceptanceFixture(t)
			contact, err := x.f.RealmContact()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.authorized = cancel
			switch operation {
			case "inspect":
				_, err = x.f.InspectRealmContact(ctx, x.caller, "")
			case "update":
				_, err = x.f.UpdateRealmContact(ctx, x.caller, contract.RealmContactUpdate{OperationID: "cancelled-contact", Owner: contact.Owner, Realm: contact.Realm, Expected: contact.Revision, NextHop: "https://localhost:34568"})
			case "recover":
				_, err = x.f.RecoverRealmContact(ctx, x.caller, "cancelled-contact", "")
			}
			if !errors.Is(err, contract.ErrUnavailable) || p.locked.Load() || p.released.Load() != 1 || len(x.f.state.RealmContact.Events) != 0 {
				t.Fatalf("err=%v locked=%v released=%d contact events=%d", err, p.locked.Load(), p.released.Load(), len(x.f.state.RealmContact.Events))
			}
		})
	}
}

func TestNetworkContactUpdateRetainsExactReceiptAndReplay(t *testing.T) {
	x, p := networkAcceptanceFixture(t)
	before, err := x.f.RealmContact()
	if err != nil {
		t.Fatal(err)
	}
	result, err := x.f.UpdateRealmContact(context.Background(), x.caller, contract.RealmContactUpdate{OperationID: "network-contact", Owner: before.Owner, Realm: before.Realm, Expected: before.Revision, NextHop: "https://localhost:34568"})
	if err != nil || result.Decision != contract.RealmContactCommitted || result.Contact.NextHop != "https://localhost:34568" {
		t.Fatalf("update=%+v %v", result, err)
	}
	recovered, err := x.f.RecoverRealmContact(context.Background(), x.caller, "network-contact", "")
	if err != nil || recovered != result || p.locked.Load() || p.released.Load() != 2 {
		t.Fatalf("recover=%+v %v", recovered, err)
	}
	if err := x.f.state.RealmContact.validateRealmContactHistory(x.f.authority, x.f.scope); err != nil {
		t.Fatalf("network contact history does not replay: %v", err)
	}
}
