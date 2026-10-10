package composition

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"math"
	"testing"
	"time"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
)

func testNetworkEndpointConfig() NetworkRealmEndpointConfig {
	return NetworkRealmEndpointConfig{
		Roots:            x509.NewCertPool(),
		SPKISHA256:       [32]byte{1},
		RPCDeadline:      time.Second,
		ConnectTimeout:   time.Second,
		MaxResponseBytes: 1 << 20,
		MaxMetadataBytes: 8192,
	}
}

func TestNetworkRealmConnectionsBindingValidation(t *testing.T) {
	const address values.NextHop = "https://realm-a.example:443"
	connections := &NetworkRealmConnections{}
	if err := connections.BindNetworkEndpoint("local:realm", "realm-a", testNetworkEndpointConfig()); !errors.Is(err, ErrInvalidNetworkRealmEndpoint) {
		t.Fatalf("local address accepted by network binding = %v", err)
	}
	invalid := []struct {
		name   string
		mutate func(*NetworkRealmEndpointConfig)
	}{
		{"zero roots", func(c *NetworkRealmEndpointConfig) { c.Roots = nil }},
		{"zero pin", func(c *NetworkRealmEndpointConfig) { c.SPKISHA256 = [32]byte{} }},
		{"zero rpc deadline", func(c *NetworkRealmEndpointConfig) { c.RPCDeadline = 0 }},
		{"zero connect timeout", func(c *NetworkRealmEndpointConfig) { c.ConnectTimeout = 0 }},
		{"zero response ceiling", func(c *NetworkRealmEndpointConfig) { c.MaxResponseBytes = 0 }},
		{"metadata below floor", func(c *NetworkRealmEndpointConfig) { c.MaxMetadataBytes = 255 }},
	}
	for _, test := range invalid {
		config := testNetworkEndpointConfig()
		test.mutate(&config)
		if err := connections.BindNetworkEndpoint(address, "realm-a", config); !errors.Is(err, ErrInvalidNetworkRealmEndpoint) {
			t.Fatalf("%s accepted = %v", test.name, err)
		}
	}
	if err := connections.BindNetworkEndpoint(address, "", testNetworkEndpointConfig()); !errors.Is(err, ErrInvalidNetworkRealmEndpoint) {
		t.Fatalf("empty expected Realm accepted = %v", err)
	}
	if err := connections.BindNetworkEndpoint(address, "realm-a", testNetworkEndpointConfig()); err != nil {
		t.Fatal(err)
	}
	endpoint, err := connections.selectEndpoint(address, "realm-a")
	if err != nil {
		t.Fatal(err)
	}
	if nextHop, realmID := endpoint.client.Target(); nextHop != address || realmID != "realm-a" {
		t.Fatalf("bound target = %v, %v", nextHop, realmID)
	}
	if _, err := connections.selectEndpoint(address, "realm-b"); !errors.Is(err, ErrNetworkRealmTargetMismatch) {
		t.Fatalf("wrong expected Realm = %v", err)
	}
	if _, err := connections.selectEndpoint("https://missing.example:443", "realm-a"); !errors.Is(err, ErrNetworkRealmUnavailable) {
		t.Fatalf("unbound address = %v", err)
	}
	if err := connections.UnbindNetworkEndpoint("https://missing.example:443"); !errors.Is(err, ErrNetworkRealmUnavailable) {
		t.Fatalf("unbinding a missing address = %v", err)
	}
	if err := connections.UnbindNetworkEndpoint(address); err != nil {
		t.Fatal(err)
	}
	if _, err := connections.selectEndpoint(address, "realm-a"); !errors.Is(err, ErrNetworkRealmUnavailable) {
		t.Fatalf("removed endpoint reused = %v", err)
	}
}

func TestNetworkRealmConnectionsRebindRetiresCapturedEndpoint(t *testing.T) {
	const address values.NextHop = "https://realm-a.example:443"
	connections := &NetworkRealmConnections{}
	if err := connections.BindNetworkEndpoint(address, "realm-a", testNetworkEndpointConfig()); err != nil {
		t.Fatal(err)
	}
	first, err := connections.selectEndpoint(address, "realm-a")
	if err != nil {
		t.Fatal(err)
	}
	if !connections.stillBound(address, "realm-a", first) {
		t.Fatal("fresh endpoint must be current")
	}
	if err := connections.UnbindNetworkEndpoint(address); err != nil {
		t.Fatal(err)
	}
	if connections.stillBound(address, "realm-a", first) {
		t.Fatal("unbound endpoint stayed current")
	}
	if err := connections.BindNetworkEndpoint(address, "realm-a", testNetworkEndpointConfig()); err != nil {
		t.Fatal(err)
	}
	if connections.stillBound(address, "realm-a", first) {
		t.Fatal("A→B→A rebinding revived a captured endpoint")
	}
	second, err := connections.selectEndpoint(address, "realm-a")
	if err != nil {
		t.Fatal(err)
	}
	if second == first || second.serial <= first.serial {
		t.Fatalf("rebind must mint a fresh serial: %d, %d", second.serial, first.serial)
	}
	if !connections.stillBound(address, "realm-a", second) {
		t.Fatal("rebound endpoint must be current")
	}
}

func TestNetworkRealmConnectionsRejectSerialExhaustion(t *testing.T) {
	const address values.NextHop = "https://realm-a.example:443"
	connections := &NetworkRealmConnections{serial: math.MaxUint64}
	if err := connections.BindNetworkEndpoint(address, "realm-a", testNetworkEndpointConfig()); !errors.Is(err, ErrInvalidNetworkRealmEndpoint) {
		t.Fatalf("exhausted serial accepted = %v", err)
	}
	if _, err := connections.selectEndpoint(address, "realm-a"); !errors.Is(err, ErrNetworkRealmUnavailable) {
		t.Fatalf("exhausted serial must not create a binding: %v", err)
	}
}

func TestNetworkRealmConnectionsPrincipalAndLocalFacesFailClosed(t *testing.T) {
	const address values.NextHop = "https://realm-a.example:443"
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "operator"}
	requirement := values.Requirement{Permission: "ucupa.core.realm-contact-read", Scope: values.AuthorityScope(owner)}
	connections := &NetworkRealmConnections{}
	if err := connections.BindNetworkEndpoint(address, "realm-a", testNetworkEndpointConfig()); err != nil {
		t.Fatal(err)
	}
	if got := connections.AuthorizePrincipal(address, "realm-a", actor, requirement); got != values.AuthorizationUnavailable {
		t.Fatalf("network profile principal authorization must fail closed: %v", got)
	}
	if _, err := connections.TrustLocalCaller(address, "realm-a", actor); !errors.Is(err, realm.ErrUnauthenticated) {
		t.Fatalf("network profile trusted local subject must fail closed: %v", err)
	}
	if decision, view := connections.AuthorizeView(context.Background(), address, "realm-a", values.CallerContext{}, requirement); decision != values.AuthorizationDenied || view != (values.AuthorizationView{}) {
		t.Fatalf("unestablished caller authorize = %v, %v", decision, view)
	}
	if got := connections.ValidateView(context.Background(), address, "realm-a", values.CallerContext{}, values.AuthorizationView{1}, requirement); got != values.AuthorizationChanged {
		t.Fatalf("unestablished caller validate = %v", got)
	}
	if decision, denial := connections.CaptureDenialView(context.Background(), address, "realm-a", values.CallerContext{}, requirement); decision != values.AuthorizationDenied || denial != (values.AuthorizationDenialView{}) {
		t.Fatalf("unestablished caller denial capture = %v, %v", decision, denial)
	}
	if got := connections.ValidateDenialView(context.Background(), address, "realm-a", values.CallerContext{}, values.AuthorizationDenialView{1}, requirement); got != values.AuthorizationChanged {
		t.Fatalf("unestablished caller denial validate = %v", got)
	}
	if decision, _ := connections.AuthorizeView(context.Background(), address, "realm-a", values.CallerContext{Subject: actor, Establishment: [32]byte{9}}, values.Requirement{Permission: "ucupa.core.realm-contact-read"}); decision != values.AuthorizationDenied {
		t.Fatalf("unencodable requirement must be a denial, not a request: %v", decision)
	}
}

// The identity faces fail closed before any dial: a wrong expected Realm or
// an unbound address never mints or forwards a proof, and a malformed foreign
// proof is rejected without reaching the Realm.
func TestNetworkRealmConnectionsIdentityFacesFailClosed(t *testing.T) {
	const address values.NextHop = "https://realm-a.example:443"
	connections := &NetworkRealmConnections{}
	if err := connections.BindNetworkEndpoint(address, "realm-a", testNetworkEndpointConfig()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connections.BeginChallenge(context.Background(), address, "realm-b"); !errors.Is(err, ErrNetworkRealmTargetMismatch) {
		t.Fatalf("wrong Realm challenge mint = %v", err)
	}
	if _, _, err := connections.BeginChallenge(context.Background(), "https://missing.example:443", "realm-a"); !errors.Is(err, ErrNetworkRealmUnavailable) {
		t.Fatalf("unbound challenge mint = %v", err)
	}
	if _, _, err := connections.EstablishForeignCaller(context.Background(), address, "realm-b", "00000000-0000-4000-8000-000000000001", []byte("payload"), make([]byte, 64), [32]byte{1}); !errors.Is(err, ErrNetworkRealmTargetMismatch) {
		t.Fatalf("wrong Realm foreign caller = %v", err)
	}
	if _, _, err := connections.EstablishForeignCaller(context.Background(), "https://missing.example:443", "realm-a", "00000000-0000-4000-8000-000000000001", []byte("payload"), make([]byte, 64), [32]byte{1}); !errors.Is(err, ErrNetworkRealmUnavailable) {
		t.Fatalf("unbound foreign caller = %v", err)
	}
	malformed := []struct {
		name      string
		payload   []byte
		signature []byte
		challenge [32]byte
	}{
		{"empty payload", nil, make([]byte, 64), [32]byte{1}},
		{"oversized payload", make([]byte, 4097), make([]byte, 64), [32]byte{1}},
		{"short signature", []byte("payload"), make([]byte, 63), [32]byte{1}},
		{"zero challenge", []byte("payload"), make([]byte, 64), [32]byte{}},
	}
	for _, test := range malformed {
		if _, _, err := connections.EstablishForeignCaller(context.Background(), address, "realm-a", "00000000-0000-4000-8000-000000000001", test.payload, test.signature, test.challenge); !errors.Is(err, realm.ErrUnauthenticated) {
			t.Fatalf("%s accepted = %v", test.name, err)
		}
	}
}

func TestNetworkRealmConnectionsRequirementEncoding(t *testing.T) {
	owner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	element := values.ElementID("00000000-0000-4000-8000-000000000001")
	facet, err := values.NewFacetTypeRef(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		scope     values.OperationScope
		kind      realmpb.ScopeKind
		authority string
		element   string
		facet     []byte
		realm     string
	}{
		{"core element", values.CoreElementScope(owner, element), realmpb.ScopeKind_CORE_ELEMENT, string(owner), string(element), nil, ""},
		{"facet element", values.FacetElementScope(owner, element), realmpb.ScopeKind_FACET_ELEMENT, string(owner), string(element), nil, ""},
		{"facet key", values.FacetKeyScope(owner, element, facet), realmpb.ScopeKind_FACET_KEY, string(owner), string(element), facet.Bytes(), ""},
		{"realm", values.RealmScope("realm-a"), realmpb.ScopeKind_REALM, "", "", nil, "realm-a"},
		{"authority", values.AuthorityScope(owner), realmpb.ScopeKind_AUTHORITY, string(owner), "", nil, ""},
		{"anchor closure", values.AnchorClosureScope(owner, element), realmpb.ScopeKind_ANCHOR_CLOSURE, string(owner), string(element), nil, ""},
	}
	for _, test := range cases {
		wire, ok := requirementToWire(values.Requirement{Permission: "ucupa.core.test", Scope: test.scope})
		if !ok {
			t.Fatalf("%s: valid requirement refused", test.name)
		}
		if wire.GetPermission() != "ucupa.core.test" {
			t.Fatalf("%s: permission = %q", test.name, wire.GetPermission())
		}
		scope := wire.GetScope()
		if scope.GetKind() != test.kind {
			t.Fatalf("%s: kind = %v", test.name, scope.GetKind())
		}
		if scope.GetAuthority() != test.authority {
			t.Fatalf("%s: authority = %q", test.name, scope.GetAuthority())
		}
		if scope.GetElement() != test.element {
			t.Fatalf("%s: element = %q", test.name, scope.GetElement())
		}
		if !bytes.Equal(scope.GetFacet(), test.facet) {
			t.Fatalf("%s: facet = %v", test.name, scope.GetFacet())
		}
		if scope.GetRealm() != test.realm {
			t.Fatalf("%s: realm = %q", test.name, scope.GetRealm())
		}
	}
	if _, ok := requirementToWire(values.Requirement{Permission: "", Scope: values.AuthorityScope(owner)}); ok {
		t.Fatal("empty permission encoded")
	}
	if _, ok := requirementToWire(values.Requirement{Permission: "ucupa.core.test"}); ok {
		t.Fatal("zero scope encoded")
	}
	valid := values.Requirement{Permission: "ucupa.core.test", Scope: values.CoreElementScope(owner, element)}
	set := make([]values.Requirement, maxNetworkRequirements)
	for i := range set {
		set[i] = valid
	}
	wire, ok := requirementsToWire(set)
	if !ok || len(wire) != maxNetworkRequirements {
		t.Fatalf("ceiling-sized set refused: %v, %d", ok, len(wire))
	}
	if _, ok := requirementsToWire(append(set, valid)); ok {
		t.Fatal("over-ceiling set encoded")
	}
	if _, ok := requirementsToWire(nil); ok {
		t.Fatal("empty set encoded")
	}
	broken := make([]values.Requirement, 3)
	for i := range broken {
		broken[i] = valid
	}
	broken[1] = values.Requirement{Permission: "ucupa.core.test"}
	if _, ok := requirementsToWire(broken); ok {
		t.Fatal("invalid member encoded")
	}
}

func TestNetworkRealmWireMappings(t *testing.T) {
	if wireDecision(realmpb.Decision_VALID) != values.AuthorizationValid {
		t.Fatal("valid decision mismatch")
	}
	if wireDecision(realmpb.Decision_CHANGED) != values.AuthorizationChanged {
		t.Fatal("changed decision mismatch")
	}
	if wireDecision(realmpb.Decision_DENIED) != values.AuthorizationDenied {
		t.Fatal("denied decision mismatch")
	}
	if wireDecision(realmpb.Decision_UNAVAILABLE) != values.AuthorizationUnavailable {
		t.Fatal("unavailable decision mismatch")
	}
	if wireDecision(realmpb.Decision(0)) != values.AuthorizationUnavailable {
		t.Fatal("unknown decision must stay unavailable")
	}
	if _, ok := networkHandle([]byte{1, 2, 3}); ok {
		t.Fatal("short handle accepted")
	}
	if _, ok := networkHandle(make([]byte, 33)); ok {
		t.Fatal("long handle accepted")
	}
	raw := bytes.Repeat([]byte{9}, 32)
	expected := [32]byte(bytes.Repeat([]byte{9}, 32))
	handle, ok := networkHandle(raw)
	if !ok || handle != expected {
		t.Fatal("exact handle rejected")
	}
	view, ok := networkView(raw)
	if !ok || view != values.AuthorizationView(expected) {
		t.Fatal("view conversion mismatch")
	}
	denial, ok := networkDenial(raw)
	if !ok || denial != values.AuthorizationDenialView(expected) {
		t.Fatal("denial conversion mismatch")
	}
}
