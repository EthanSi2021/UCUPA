package composition_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ucupa/composition"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
)

const (
	connectionRefA values.FactAuthorityRef = "00000000-0000-4000-8000-000000000101"
	connectionRefB values.FactAuthorityRef = "00000000-0000-4000-8000-000000000102"
	connectionID   values.ElementID        = "00000000-0000-4000-8000-000000000103"
)

var connectionActor = values.PrincipalRef{Realm: "connection-realm", Principal: "reader"}

type connectionPort struct {
	ref      values.FactAuthorityRef
	identity func() values.FactAuthorityRef
	read     func(values.PrincipalRef, values.ElementID, *corecontract.CoreReadCut) (corecontract.ElementRead, error)
}

func (port *connectionPort) AuthorityRef() values.FactAuthorityRef {
	if port.identity != nil {
		return port.identity()
	}
	return port.ref
}

func (port *connectionPort) ReadElement(actor values.PrincipalRef, id values.ElementID, at *corecontract.CoreReadCut) (corecontract.ElementRead, error) {
	return port.read(actor, id, at)
}

type connectionAuthorizer struct{}

func (connectionAuthorizer) Authorize(actor values.PrincipalRef, requirements ...values.Requirement) bool {
	if actor != connectionActor {
		return false
	}
	for _, requirement := range requirements {
		if !requirement.Valid() {
			return false
		}
	}
	return true
}

var connectionContact = corecontract.RealmInitialization{Realm: connectionActor.Realm, NextHop: "local:connection-realm"}

// connectionContacts adapts the fixed bool authorization stub to the Realm
// contact port. These tests exercise the local connection tables, not the
// public view path, so view evidence stays unavailable.
type connectionContacts struct{}

func (connectionContacts) Check(context.Context, values.NextHop, values.RealmID) error { return nil }

func (connectionContacts) AuthorizePrincipal(_ values.NextHop, _ values.RealmID, actor values.PrincipalRef, requirements ...values.Requirement) values.AuthorizationDecision {
	stub := connectionAuthorizer{}
	if stub.Authorize(actor, requirements...) {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}

func (connectionContacts) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return values.AuthorizationUnavailable, values.AuthorizationView{}
}

func (connectionContacts) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func (connectionContacts) CaptureDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationUnavailable, values.AuthorizationDenialView{}
}

func (connectionContacts) ValidateDenialView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationDenialView, values.Requirement) values.AuthorizationDecision {
	return values.AuthorizationUnavailable
}

func TestLocalCoreConnectionsValidateBindings(t *testing.T) {
	var connections composition.LocalCoreConnections
	valid := &connectionPort{ref: connectionRefA}
	var typedNil *connectionPort
	tests := []struct {
		name    string
		address values.NextHop
		peer    composition.CoreReadPort
	}{
		{name: "invalid address", address: "LOCAL:peer", peer: valid},
		{name: "nil port", address: "local:peer"},
		{name: "typed nil port", address: "local:peer", peer: typedNil},
		{name: "typed nil Core entry", address: "local:peer", peer: (*core.Core)(nil).Elements()},
		{name: "invalid identity", address: "local:peer", peer: &connectionPort{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := connections.BindEndpoint(test.address, composition.CoreEndpoint{Read: test.peer}); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
				t.Fatalf("BindEndpoint() = %v, want invalid endpoint", err)
			}
		})
	}
	if err := connections.UnbindEndpoint("local:"); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
		t.Fatalf("UnbindEndpoint(invalid) = %v", err)
	}
	if err := connections.UnbindEndpoint("local:absent"); !errors.Is(err, composition.ErrCoreEndpointUnavailable) {
		t.Fatalf("UnbindEndpoint(absent) = %v", err)
	}
	for _, test := range []struct {
		address values.NextHop
		ref     values.FactAuthorityRef
	}{
		{address: "peer", ref: connectionRefA},
		{address: "local:peer", ref: "invalid"},
	} {
		result, err := connections.ReadElement(test.address, test.ref, connectionActor, connectionID, nil)
		assertConnectionError(t, result, err, composition.ErrInvalidCoreEndpoint)
	}
}

func TestLocalCoreConnectionsNeverScanAlternativeAddresses(t *testing.T) {
	var connections composition.LocalCoreConnections
	readCalls, alternateIdentityCalls := 0, 0
	read := func(values.PrincipalRef, values.ElementID, *corecontract.CoreReadCut) (corecontract.ElementRead, error) {
		readCalls++
		return corecontract.ElementRead{}, nil
	}
	wrong := &connectionPort{ref: connectionRefB, read: read}
	alternate := &connectionPort{read: read, identity: func() values.FactAuthorityRef {
		alternateIdentityCalls++
		return connectionRefA
	}}
	mustBindConnection(t, &connections, "local:wrong", wrong)
	mustBindConnection(t, &connections, "local:alternate", alternate)
	alternateIdentityCalls = 0
	result, err := connections.ReadElement("local:wrong", connectionRefA, connectionActor, connectionID, nil)
	assertConnectionError(t, result, err, composition.ErrCoreTargetMismatch)
	result, err = connections.ReadElement("local:missing", connectionRefA, connectionActor, connectionID, nil)
	assertConnectionError(t, result, err, composition.ErrCoreEndpointUnavailable)
	if readCalls != 0 || alternateIdentityCalls != 0 {
		t.Fatalf("unexpected peer calls: reads=%d alternate identities=%d", readCalls, alternateIdentityCalls)
	}
}

func TestLocalCoreConnectionsForwardExactReadAndPreserveErrors(t *testing.T) {
	var connections composition.LocalCoreConnections
	at := &corecontract.CoreReadCut{}
	want := corecontract.ElementRead{State: corecontract.ExistenceNotFound}
	var outcomeErr error
	peer := &connectionPort{ref: connectionRefA, read: func(actor values.PrincipalRef, id values.ElementID, cut *corecontract.CoreReadCut) (corecontract.ElementRead, error) {
		if actor != connectionActor || id != connectionID || cut != at {
			t.Errorf("read arguments changed: actor=%#v id=%q cut=%p", actor, id, cut)
		}
		return want, outcomeErr
	}}
	mustBindConnection(t, &connections, "local:peer", peer)
	result, err := connections.ReadElement("local:peer", connectionRefA, connectionActor, connectionID, at)
	if err != nil || !reflect.DeepEqual(result, want) {
		t.Fatalf("ReadElement() = (%#v, %v), want exact result", result, err)
	}
	for _, outcomeErr = range []error{corecontract.ErrDenied, corecontract.ErrStore, corecontract.ErrReadCutExpired} {
		result, err = connections.ReadElement("local:peer", connectionRefA, connectionActor, connectionID, at)
		assertConnectionError(t, result, err, outcomeErr)
	}
	if err := connections.UnbindEndpoint("local:peer"); err != nil {
		t.Fatal(err)
	}
	result, err = connections.ReadElement("local:peer", connectionRefA, connectionActor, connectionID, at)
	assertConnectionError(t, result, err, composition.ErrCoreEndpointUnavailable)
}

func TestLocalCoreConnectionsRejectIdentityChangeDuringRead(t *testing.T) {
	for _, readErr := range []error{nil, corecontract.ErrDenied} {
		var connections composition.LocalCoreConnections
		peer := &connectionPort{ref: connectionRefA}
		peer.read = func(values.PrincipalRef, values.ElementID, *corecontract.CoreReadCut) (corecontract.ElementRead, error) {
			peer.ref = connectionRefB
			return corecontract.ElementRead{State: corecontract.ExistenceNotFound}, readErr
		}
		mustBindConnection(t, &connections, "local:peer", peer)
		result, err := connections.ReadElement("local:peer", connectionRefA, connectionActor, connectionID, nil)
		assertConnectionError(t, result, err, composition.ErrCoreTargetMismatch)
		if readErr != nil && !errors.Is(err, readErr) {
			t.Fatalf("identity mismatch hid peer error: %v", err)
		}
	}
}

func TestLocalCoreConnectionsAllowReentrantBindingAndKeepCapturedPeer(t *testing.T) {
	var connections composition.LocalCoreConnections
	oldResult := corecontract.ElementRead{State: corecontract.ExistenceNotFound}
	newResult := corecontract.ElementRead{State: corecontract.ExistencePresent}
	replacement := &connectionPort{ref: connectionRefB, read: func(values.PrincipalRef, values.ElementID, *corecontract.CoreReadCut) (corecontract.ElementRead, error) {
		return newResult, nil
	}}
	old := &connectionPort{identity: func() values.FactAuthorityRef {
		if err := connections.BindEndpoint("local:identity-callback", composition.CoreEndpoint{Read: replacement}); err != nil {
			t.Errorf("reentrant identity binding: %v", err)
		}
		return connectionRefA
	}, read: func(values.PrincipalRef, values.ElementID, *corecontract.CoreReadCut) (corecontract.ElementRead, error) {
		if err := connections.BindEndpoint("local:peer", composition.CoreEndpoint{Read: replacement}); err != nil {
			return corecontract.ElementRead{}, err
		}
		return oldResult, nil
	}}
	mustBindConnection(t, &connections, "local:peer", old)
	type outcome struct {
		read corecontract.ElementRead
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := connections.ReadElement("local:peer", connectionRefA, connectionActor, connectionID, nil)
		done <- outcome{read: result, err: err}
	}()
	select {
	case result := <-done:
		if result.err != nil || !reflect.DeepEqual(result.read, oldResult) {
			t.Fatalf("in-flight read mixed rebound port: %#v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("peer callback deadlocked while binding an endpoint")
	}
	result, err := connections.ReadElement("local:peer", connectionRefA, connectionActor, connectionID, nil)
	assertConnectionError(t, result, err, composition.ErrCoreTargetMismatch)
	result, err = connections.ReadElement("local:peer", connectionRefB, connectionActor, connectionID, nil)
	if err != nil || !reflect.DeepEqual(result, newResult) {
		t.Fatalf("next read did not use rebound port: (%#v, %v)", result, err)
	}
}

func TestLocalCoreConnectionsDurableReopenAndNewCoreIdentity(t *testing.T) {
	var connections composition.LocalCoreConnections
	path := filepath.Join(t.TempDir(), "core.db")
	spatial, err := core.CreateDurable(connectionContacts{}, connectionContact, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spatial.Close() })
	ref := spatial.AuthorityRef()
	created, err := spatial.Elements().CreateRoot(connectionActor, connectionID, "connection-create-root")
	if err != nil {
		t.Fatal(err)
	}
	mustBindConnection(t, &connections, "local:peer", spatial.Elements())
	result, err := connections.ReadElement("local:peer", ref, connectionActor, connectionID, &created.OutcomeCut)
	if err != nil || result.State != corecontract.ExistencePresent || !reflect.DeepEqual(result.Record, created.Record) {
		t.Fatalf("durable read = (%#v, %v)", result, err)
	}
	denied := values.PrincipalRef{Realm: connectionActor.Realm, Principal: "denied"}
	result, err = connections.ReadElement("local:peer", ref, denied, connectionID, nil)
	assertConnectionError(t, result, err, corecontract.ErrDenied)
	if err := spatial.Close(); err != nil {
		t.Fatal(err)
	}
	result, err = connections.ReadElement("local:peer", ref, connectionActor, connectionID, nil)
	assertConnectionError(t, result, err, corecontract.ErrStore)
	reopened, err := core.OpenDurable(connectionContacts{}, path, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if reopened.AuthorityRef() != ref {
		t.Fatal("normal durable reopen changed Core identity")
	}
	mustBindConnection(t, &connections, "local:peer", reopened.Elements())
	result, err = connections.ReadElement("local:peer", ref, connectionActor, connectionID, &created.OutcomeCut)
	if err != nil || !reflect.DeepEqual(result.Record, created.Record) {
		t.Fatalf("reopened read = (%#v, %v)", result, err)
	}
	replacement, err := core.New(connectionContacts{}, connectionContact, corecontract.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if replacement.AuthorityRef() == ref {
		t.Fatal("new Core reused the durable identity")
	}
	mustBindConnection(t, &connections, "local:peer", replacement.Elements())
	result, err = connections.ReadElement("local:peer", ref, connectionActor, connectionID, nil)
	assertConnectionError(t, result, err, composition.ErrCoreTargetMismatch)
}

func mustBindConnection(t *testing.T, connections *composition.LocalCoreConnections, address values.NextHop, peer composition.CoreReadPort) {
	t.Helper()
	if err := connections.BindEndpoint(address, composition.CoreEndpoint{Read: peer}); err != nil {
		t.Fatalf("BindEndpoint(%q): %v", address, err)
	}
}

func assertConnectionError(t *testing.T, result corecontract.ElementRead, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || !reflect.DeepEqual(result, corecontract.ElementRead{}) {
		t.Fatalf("ReadElement() = (%#v, %v), want (zero, %v)", result, err, want)
	}
}

type publicConnectionPort struct {
	ref  values.FactAuthorityRef
	call func(context.Context, values.CallerContext, []byte) ([]byte, error)
}

func (port *publicConnectionPort) AuthorityRef() values.FactAuthorityRef { return port.ref }
func (port *publicConnectionPort) PublicCall(ctx context.Context, caller values.CallerContext, raw []byte) ([]byte, error) {
	return port.call(ctx, caller, raw)
}

func TestLocalCoreEndpointExplicitPublicBinding(t *testing.T) {
	var connections composition.LocalCoreConnections
	read := &connectionPort{ref: connectionRefA}
	var typedNil *publicConnectionPort
	for _, public := range []composition.CorePublicPort{typedNil, &publicConnectionPort{ref: connectionRefB}} {
		if err := connections.BindEndpoint("local:peer", composition.CoreEndpoint{Read: read, Public: public}); !errors.Is(err, composition.ErrInvalidCoreEndpoint) {
			t.Fatalf("invalid public owner bound: %v", err)
		}
	}
	if err := connections.BindEndpoint("local:peer", composition.CoreEndpoint{Read: read}); err != nil {
		t.Fatal(err)
	}
	if raw, err := connections.PublicCall(context.Background(), "local:peer", connectionRefA, values.CallerContext{}, nil); !errors.Is(err, values.PublicUnavailable) || raw != nil {
		t.Fatalf("missing public entry acquired capability: %q %v", raw, err)
	}
}

func TestLocalCorePublicCallKeepsCapturedPairAndExactEnvelope(t *testing.T) {
	var connections composition.LocalCoreConnections
	ctx := context.Background()
	caller := values.CallerContext{Subject: connectionActor}
	request := []byte("exact-request")
	read := &connectionPort{ref: connectionRefA}
	replacement := &publicConnectionPort{ref: connectionRefA, call: func(context.Context, values.CallerContext, []byte) ([]byte, error) { return []byte("next"), nil }}
	public := &publicConnectionPort{ref: connectionRefA, call: func(got context.Context, who values.CallerContext, raw []byte) ([]byte, error) {
		if got != ctx || !reflect.DeepEqual(who, caller) || !reflect.DeepEqual(raw, request) {
			t.Error("connection changed the public envelope")
		}
		if err := connections.BindEndpoint("local:peer", composition.CoreEndpoint{Read: read, Public: replacement}); err != nil {
			t.Error(err)
		}
		return []byte("captured"), nil
	}}
	if err := connections.BindEndpoint("local:peer", composition.CoreEndpoint{Read: read, Public: public}); err != nil {
		t.Fatal(err)
	}
	type response struct {
		raw []byte
		err error
	}
	done := make(chan response, 1)
	go func() {
		raw, err := connections.PublicCall(ctx, "local:peer", connectionRefA, caller, request)
		done <- response{raw, err}
	}()
	select {
	case out := <-done:
		if out.err != nil || string(out.raw) != "captured" {
			t.Fatalf("mixed in-flight entry: %q %v", out.raw, out.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("public call held the connection lock")
	}
	if raw, err := connections.PublicCall(ctx, "local:peer", connectionRefA, caller, request); err != nil || string(raw) != "next" {
		t.Fatalf("next call ignored rebound entry: %q %v", raw, err)
	}
}

func TestLocalCorePublicCallRechecksBothOwners(t *testing.T) {
	for _, change := range []string{"read", "public"} {
		t.Run(change, func(t *testing.T) {
			var connections composition.LocalCoreConnections
			read := &connectionPort{ref: connectionRefA}
			public := &publicConnectionPort{ref: connectionRefA}
			public.call = func(context.Context, values.CallerContext, []byte) ([]byte, error) {
				if change == "read" {
					read.ref = connectionRefB
				} else {
					public.ref = connectionRefB
				}
				return []byte("must not disclose"), nil
			}
			if err := connections.BindEndpoint("local:peer", composition.CoreEndpoint{Read: read, Public: public}); err != nil {
				t.Fatal(err)
			}
			if raw, err := connections.PublicCall(context.Background(), "local:peer", connectionRefA, values.CallerContext{}, nil); !errors.Is(err, values.PublicUnavailable) || raw != nil {
				t.Fatalf("changed %s identity disclosed response: %q %v", change, raw, err)
			}
		})
	}
}
