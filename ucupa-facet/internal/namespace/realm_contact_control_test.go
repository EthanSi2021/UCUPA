package namespace

import (
	"context"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// contactPort is a controllable trusted-local connection port: it maps
// addresses to live/Realm/decision answers and records what control used.
type contactPort struct {
	live     map[values.NextHop]bool
	realm    map[values.NextHop]values.RealmID
	decision map[values.NextHop]values.AuthorizationDecision
	used     []values.NextHop
	requests []values.Requirement
	hook     func()
}

func (p *contactPort) Check(ctx context.Context, address values.NextHop, expected values.RealmID) error {
	if !p.live[address] || p.realm[address] != expected {
		return errors.New("endpoint offline")
	}
	return nil
}

func (p *contactPort) AuthorizePrincipal(address values.NextHop, _ values.RealmID, _ values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	p.used = append(p.used, address)
	p.requests = append(p.requests, requirements...)
	if p.hook != nil {
		hook := p.hook
		p.hook = nil
		hook()
	}
	return p.decision[address]
}

func (p *contactPort) AuthorizeView(_ context.Context, address values.NextHop, _ values.RealmID, _ values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	p.used = append(p.used, address)
	p.requests = append(p.requests, requirements...)
	if p.hook != nil {
		hook := p.hook
		p.hook = nil
		hook()
	}
	if p.decision[address] == values.AuthorizationValid {
		return values.AuthorizationValid, values.AuthorizationView{1}
	}
	return p.decision[address], values.AuthorizationView{}
}

func (p *contactPort) ValidateView(_ context.Context, address values.NextHop, _ values.RealmID, _ values.CallerContext, view values.AuthorizationView, _ ...values.Requirement) values.AuthorizationDecision {
	if p.hook != nil {
		hook := p.hook
		p.hook = nil
		hook()
	}
	if p.decision[address] != values.AuthorizationValid || view != (values.AuthorizationView{1}) {
		return values.AuthorizationChanged
	}
	return values.AuthorizationValid
}

func (*contactPort) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}
func (*contactPort) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func testContactControl() (*Owner, *contactPort, contract.RealmAttachment, values.CallerContext) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		panic(err)
	}
	scope := [32]byte{12}
	contact, err := newRealmContact(contract.RealmInitialization{Realm: "realm-a", NextHop: "local:old"})
	if err != nil {
		panic(err)
	}
	facts := emptyState()
	facts.PublicKey = [32]byte{5}
	facts.RealmContact = &contact
	port := &contactPort{live: map[values.NextHop]bool{"local:old": true},
		realm:    map[values.NextHop]values.RealmID{"local:old": "realm-a"},
		decision: map[values.NextHop]values.AuthorizationDecision{"local:old": values.AuthorizationValid}}
	f := &Owner{ownerState: &ownerState{authority: owner, scope: scope, realmPort: port, state: facts, operations: map[byteString]int{}}}
	return f, port, contact.attachment(owner, scope), values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-a", Principal: "operator"}}
}

func TestContactControlUsesCurrentRealmAndRetainsOriginalResult(t *testing.T) {
	f, port, attachment, actor := testContactControl()
	contact, err := f.InspectRealmContact(context.Background(), actor, "")
	if err != nil || contact != attachment || len(port.requests) != 1 ||
		port.requests[0] != (values.Requirement{Permission: contract.PermissionRealmContactRead, Scope: values.AuthorityScope(attachment.Owner)}) {
		t.Fatalf("read permission/contact = %+v, %v, %+v", contact, err, port.requests)
	}
	request := contract.RealmContactUpdate{OperationID: "move", Owner: attachment.Owner, Realm: attachment.Realm,
		Expected: attachment.Revision, NextHop: "local:new"}
	result, err := f.UpdateRealmContact(context.Background(), actor, request)
	if err != nil || result.Decision != contract.RealmContactCommitted || result.Contact.NextHop != "local:new" ||
		len(f.state.RealmContact.Events) != 1 {
		t.Fatalf("current-address update to offline new address = %+v, %v", result, err)
	}
	if port.used[len(port.used)-1] != "local:old" ||
		port.requests[len(port.requests)-1].Permission != contract.PermissionRealmContactUpdate {
		t.Fatal("update did not authorize through current address with independent permission")
	}
	if _, err := f.RecoverRealmContact(context.Background(), actor, request.OperationID, ""); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("offline new current address must not disclose retained result: %v", err)
	}
	port.live["local:new"] = true
	port.realm["local:new"] = "realm-a"
	port.decision["local:new"] = values.AuthorizationValid
	recovered, err := f.RecoverRealmContact(context.Background(), actor, request.OperationID, "")
	if err != nil || recovered != result || len(f.state.RealmContact.Events) != 1 {
		t.Fatalf("recover must return original result without replay: %+v, %v", recovered, err)
	}
	after, err := f.currentRealmContact()
	if err != nil || after != result.Contact {
		t.Fatalf("recover changed current contact: %+v, %v", after, err)
	}

	// A→B→A does not reuse the first generation, and a later Recover still
	// returns the original receipt after the contact moved on.
	back := contract.RealmContactUpdate{OperationID: "back", Owner: result.Contact.Owner, Realm: result.Contact.Realm,
		Expected: result.Contact.Revision, NextHop: "local:old"}
	backResult, err := f.UpdateRealmContact(context.Background(), actor, back)
	if err != nil || backResult.Decision != contract.RealmContactCommitted || backResult.Contact.Revision == attachment.Revision ||
		backResult.Contact.Revision == result.Contact.Revision {
		t.Fatalf("A to B to A reused an old revision: %+v, %v", backResult, err)
	}
	if again, err := f.RecoverRealmContact(context.Background(), actor, request.OperationID, ""); err != nil || again != result || again.Contact.NextHop != "local:new" {
		t.Fatalf("original receipt after later updates = %+v, %v", again, err)
	}
}

func TestContactControlCandidateOnlyRepairsUnavailableCurrent(t *testing.T) {
	f, port, attachment, actor := testContactControl()
	request := contract.RealmContactUpdate{OperationID: "repair", Owner: attachment.Owner,
		Realm: attachment.Realm, Expected: attachment.Revision, NextHop: "local:repair"}
	port.live["local:repair"] = true
	port.realm["local:repair"] = "realm-a"
	port.decision["local:repair"] = values.AuthorizationValid
	port.decision["local:old"] = values.AuthorizationDenied
	if _, err := f.UpdateRealmContact(context.Background(), actor, request); !errors.Is(err, contract.ErrDenied) || len(port.used) != 1 {
		t.Fatalf("explicit denial must not switch to candidate: %v, calls=%v", err, port.used)
	}
	port.live["local:old"] = false
	port.realm["local:repair"] = "other-realm"
	if _, err := f.UpdateRealmContact(context.Background(), actor, request); !errors.Is(err, contract.ErrUnavailable) || f.state.RealmContact.Current != "local:old" {
		t.Fatalf("wrong-Realm candidate must not repair contact: %v", err)
	}
	port.realm["local:repair"] = "realm-a"
	result, err := f.UpdateRealmContact(context.Background(), actor, request)
	if err != nil || result.Decision != contract.RealmContactCommitted || port.used[len(port.used)-1] != "local:repair" {
		t.Fatalf("unavailable current address permits same-Realm candidate: %+v, %v", result, err)
	}
}

func TestContactControlRejectsContactChangeDuringAuthorization(t *testing.T) {
	f, port, attachment, actor := testContactControl()
	request := contract.RealmContactUpdate{OperationID: "stale", Owner: attachment.Owner,
		Realm: attachment.Realm, Expected: attachment.Revision, NextHop: "local:new"}
	port.hook = func() {
		f.mu.Lock()
		moved := f.state.RealmContact.clone()
		moved.Serial++
		f.state.RealmContact = &moved
		f.refreshContactLocked()
		f.mu.Unlock()
	}
	if _, err := f.UpdateRealmContact(context.Background(), actor, request); !errors.Is(err, contract.ErrUnavailable) || len(f.state.RealmContact.Events) != 0 {
		t.Fatalf("old approval crossed contact generation: %v, events=%d", err, len(f.state.RealmContact.Events))
	}
}

func TestContactControlValidatesInputAndUnknownOperation(t *testing.T) {
	f, _, attachment, actor := testContactControl()
	shape := contract.RealmContactUpdate{OperationID: "move", Owner: attachment.Owner, Realm: attachment.Realm,
		Expected: attachment.Revision, NextHop: "local:other-realm"}
	shape.Realm = "other-realm"
	if _, err := f.UpdateRealmContact(context.Background(), actor, shape); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("cross-Realm update shape = %v", err)
	}
	if _, err := f.UpdateRealmContact(context.Background(), values.CallerContext{}, shape); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("invalid actor = %v", err)
	}
	if _, err := f.RecoverRealmContact(context.Background(), actor, "", ""); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("empty operation = %v", err)
	}
	if _, err := f.RecoverRealmContact(context.Background(), actor, "unknown", ""); !errors.Is(err, contract.ErrOperationNotFound) {
		t.Fatalf("unknown operation receipt = %v", err)
	}
}

func TestContactControlRetainsCasDecisionsThroughPublicChain(t *testing.T) {
	f, port, attachment, actor := testContactControl()
	port.live["local:new"] = true
	port.realm["local:new"] = "realm-a"
	port.decision["local:new"] = values.AuthorizationValid
	moved, err := f.UpdateRealmContact(context.Background(), actor, contract.RealmContactUpdate{OperationID: "first", Owner: attachment.Owner,
		Realm: attachment.Realm, Expected: attachment.Revision, NextHop: "local:new"})
	if err != nil || moved.Decision != contract.RealmContactCommitted {
		t.Fatalf("first move = %+v, %v", moved, err)
	}

	// A stale CAS expectation is an authorized, retained rejection — never an
	// unauthorized fallback and never a silent retry.
	stale := contract.RealmContactUpdate{OperationID: "stale-cas", Owner: attachment.Owner,
		Realm: attachment.Realm, Expected: attachment.Revision, NextHop: "local:old"}
	rejected, err := f.UpdateRealmContact(context.Background(), actor, stale)
	if err != nil || rejected.Decision != contract.RealmContactRejected || rejected.Contact.NextHop != "local:new" ||
		rejected.Contact.Revision != moved.Contact.Revision {
		t.Fatalf("stale CAS = %+v, %v", rejected, err)
	}
	unchanged := contract.RealmContactUpdate{OperationID: "same-address", Owner: attachment.Owner,
		Realm: attachment.Realm, Expected: moved.Contact.Revision, NextHop: "local:new"}
	same, err := f.UpdateRealmContact(context.Background(), actor, unchanged)
	if err != nil || same.Decision != contract.RealmContactUnchanged || same.Contact != moved.Contact {
		t.Fatalf("same-address update = %+v, %v", same, err)
	}
	current, err := f.currentRealmContact()
	if err != nil || current != moved.Contact {
		t.Fatalf("unchanged decision advanced generation: %+v, %v", current, err)
	}
	for _, id := range []contract.OperationID{"first", "stale-cas", "same-address"} {
		if _, err := f.RecoverRealmContact(context.Background(), actor, id, ""); err != nil {
			t.Fatalf("retained decision %q not recoverable: %v", id, err)
		}
	}
}
