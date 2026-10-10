package realmcontact

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

type testOwner struct {
	mu      sync.RWMutex
	contact contract.RealmAttachment
	results map[contract.OperationID]contract.RealmContactResult
	applies int
}

func (o *testOwner) AuthorityRef() values.FactAuthorityRef { return o.contact.Owner }
func (o *testOwner) RLock()                                { o.mu.RLock() }
func (o *testOwner) RUnlock()                              { o.mu.RUnlock() }
func (o *testOwner) Lock()                                 { o.mu.Lock() }
func (o *testOwner) Unlock()                               { o.mu.Unlock() }
func (o *testOwner) RealmContactLocked() (contract.RealmAttachment, error) {
	return o.contact, nil
}
func (o *testOwner) RealmContactOutcomeLocked(id contract.OperationID) (contract.RealmContactResult, bool) {
	result, found := o.results[id]
	return result, found
}
func (o *testOwner) ApplyRealmContactLocked(_ values.PrincipalRef, request contract.RealmContactUpdate) (contract.RealmContactResult, error) {
	o.applies++
	result := contract.RealmContactResult{OperationID: request.OperationID, Contact: o.contact}
	if request.Expected != o.contact.Revision {
		result.Decision = contract.RealmContactRejected
	} else if request.NextHop == o.contact.NextHop {
		result.Decision = contract.RealmContactUnchanged
	} else {
		result.Decision = contract.RealmContactCommitted
		o.contact.NextHop = request.NextHop
		o.contact.Revision[0]++
		result.Contact = o.contact
	}
	o.results[request.OperationID] = result
	return result, nil
}

type testConnections struct {
	live     map[values.NextHop]bool
	realm    map[values.NextHop]values.RealmID
	decision map[values.NextHop]values.AuthorizationDecision
	used     []values.NextHop
	requests []values.Requirement
	after    func()
}

func (c *testConnections) Check(ctx context.Context, address values.NextHop, expected values.RealmID) error {
	if !c.live[address] || c.realm[address] != expected {
		return errors.New("endpoint offline")
	}
	return nil
}
func (*testConnections) AuthorizePrincipal(values.NextHop, values.RealmID, values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}
func (c *testConnections) AuthorizeView(_ context.Context, address values.NextHop, _ values.RealmID, _ values.CallerContext, requirements ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	c.used = append(c.used, address)
	c.requests = append(c.requests, requirements...)
	if c.after != nil {
		c.after()
		c.after = nil
	}
	return c.decision[address], values.AuthorizationView{}
}
func (*testConnections) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}
func (*testConnections) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}
func (*testConnections) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func testControl() (*Operations, *testOwner, *testConnections, values.CallerContext) {
	contact := contract.RealmAttachment{Owner: values.FactAuthorityRef("11111111-1111-4111-8111-111111111111"), Realm: "realm-a", NextHop: "local:old"}
	contact.Revision[0] = 1
	owner := &testOwner{contact: contact, results: map[contract.OperationID]contract.RealmContactResult{}}
	connections := &testConnections{live: map[values.NextHop]bool{"local:old": true},
		realm:    map[values.NextHop]values.RealmID{"local:old": "realm-a"},
		decision: map[values.NextHop]values.AuthorizationDecision{"local:old": values.AuthorizationValid}}
	caller := values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-a", Principal: "operator"}, Credential: [16]byte{1}, Establishment: [32]byte{2}, Proof: [32]byte{3}}
	return &Operations{owner: owner, connections: connections}, owner, connections, caller
}

func TestControlUsesCurrentRealmAndRetainsOriginalResult(t *testing.T) {
	control, owner, connections, caller := testControl()
	contact, err := control.Inspect(context.Background(), caller, "")
	if err != nil || contact != owner.contact || len(connections.requests) != 1 ||
		connections.requests[0] != (values.Requirement{Permission: contract.PermissionRealmContactRead, Scope: values.AuthorityScope(contact.Owner)}) {
		t.Fatalf("read permission/contact = %+v, %v, %+v", contact, err, connections.requests)
	}
	request := contract.RealmContactUpdate{OperationID: "move", Owner: contact.Owner, Realm: contact.Realm,
		Expected: contact.Revision, NextHop: "local:new"}
	result, err := control.Update(context.Background(), caller, request)
	if err != nil || result.Decision != contract.RealmContactCommitted || result.Contact.NextHop != "local:new" || owner.applies != 1 {
		t.Fatalf("current-address update to offline new address = %+v, %v", result, err)
	}
	if connections.used[len(connections.used)-1] != "local:old" ||
		connections.requests[len(connections.requests)-1].Permission != contract.PermissionRealmContactUpdate {
		t.Fatal("update did not authorize through current address with independent permission")
	}
	if _, err := control.Recover(context.Background(), caller, request.OperationID, ""); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("offline new current address must not disclose retained result: %v", err)
	}
	connections.live["local:new"] = true
	connections.realm["local:new"] = "realm-a"
	connections.decision["local:new"] = values.AuthorizationValid
	recovered, err := control.Recover(context.Background(), caller, request.OperationID, "")
	if err != nil || recovered != result || owner.applies != 1 {
		t.Fatalf("recover must return original result without replay: %+v, %v", recovered, err)
	}
}

func TestControlCandidateOnlyRepairsUnavailableCurrent(t *testing.T) {
	control, owner, connections, caller := testControl()
	request := contract.RealmContactUpdate{OperationID: "repair", Owner: owner.contact.Owner,
		Realm: owner.contact.Realm, Expected: owner.contact.Revision, NextHop: "local:repair"}
	connections.live["local:repair"] = true
	connections.realm["local:repair"] = "realm-a"
	connections.decision["local:repair"] = values.AuthorizationValid
	connections.decision["local:old"] = values.AuthorizationDenied
	if _, err := control.Update(context.Background(), caller, request); !errors.Is(err, contract.ErrDenied) || owner.applies != 0 || len(connections.used) != 1 {
		t.Fatalf("explicit denial must not switch to candidate: %v, calls=%v", err, connections.used)
	}
	connections.live["local:old"] = false
	connections.realm["local:repair"] = "other-realm"
	if _, err := control.Update(context.Background(), caller, request); !errors.Is(err, contract.ErrUnavailable) || owner.applies != 0 {
		t.Fatalf("wrong-Realm candidate must not repair contact: %v", err)
	}
	connections.realm["local:repair"] = "realm-a"
	result, err := control.Update(context.Background(), caller, request)
	if err != nil || result.Decision != contract.RealmContactCommitted || connections.used[len(connections.used)-1] != "local:repair" {
		t.Fatalf("unavailable current address permits same-Realm candidate: %+v, %v", result, err)
	}
}

func TestControlRejectsContactChangeDuringAuthorization(t *testing.T) {
	control, owner, connections, caller := testControl()
	request := contract.RealmContactUpdate{OperationID: "stale", Owner: owner.contact.Owner,
		Realm: owner.contact.Realm, Expected: owner.contact.Revision, NextHop: "local:new"}
	connections.after = func() {
		owner.mu.Lock()
		owner.contact.Revision[0]++
		owner.mu.Unlock()
	}
	if _, err := control.Update(context.Background(), caller, request); !errors.Is(err, contract.ErrUnavailable) || owner.applies != 0 {
		t.Fatalf("old approval crossed contact generation: %v, commits=%d", err, owner.applies)
	}
}

// Authorization may settle while the request is live, then the request dies
// waiting for the fact lock. Every final maintenance boundary must recheck it.
func TestControlRejectsCanceledFinalBoundary(t *testing.T) {
	for _, operation := range []string{"inspect", "update", "recover"} {
		for _, expired := range []bool{false, true} {
			name := operation + "/canceled"
			if expired {
				name = operation + "/expired"
			}
			t.Run(name, func(t *testing.T) {
				control, owner, connections, caller := testControl()
				original := owner.contact
				owner.results["retained"] = contract.RealmContactResult{OperationID: "retained", Decision: contract.RealmContactCommitted, Contact: original}
				ctx, cancel := context.WithCancel(context.Background())
				if expired {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 25*time.Millisecond)
				}
				defer cancel()
				authorized := make(chan struct{})
				connections.after = func() { owner.mu.Lock(); close(authorized) }
				done := make(chan error, 1)
				go func() {
					var err error
					switch operation {
					case "inspect":
						_, err = control.Inspect(ctx, caller, "")
					case "update":
						_, err = control.Update(ctx, caller, contract.RealmContactUpdate{OperationID: "canceled", Owner: original.Owner, Realm: original.Realm, Expected: original.Revision, NextHop: "local:new"})
					case "recover":
						_, err = control.Recover(ctx, caller, "retained", "")
					}
					done <- err
				}()
				select {
				case <-authorized:
				case <-time.After(time.Second):
					t.Fatal("authorization did not settle")
				}
				if expired {
					<-ctx.Done()
				} else {
					cancel()
				}
				owner.mu.Unlock()
				select {
				case err := <-done:
					if !errors.Is(err, contract.ErrUnavailable) || owner.applies != 0 || owner.contact != original || len(owner.results) != 1 {
						t.Fatalf("dead request crossed final boundary: err=%v applies=%d contact=%+v results=%d", err, owner.applies, owner.contact, len(owner.results))
					}
				case <-time.After(time.Second):
					t.Fatal("maintenance stayed blocked")
				}
				if _, err := control.Inspect(context.Background(), caller, ""); err != nil {
					t.Fatalf("fresh request failed: %v", err)
				}
			})
		}
	}
}
