package coresdk

import (
	"crypto/tls"
	"testing"

	"google.golang.org/protobuf/proto"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestDelegationShapeTargetActorAndContextAreBound(t *testing.T) {
	const target = "00000000-0000-4000-8000-000000000001"
	const actor = "00000000-0000-4000-8000-000000000002"
	base := &corepb.ReadDelegation{Target: proto.String(target), Actor: proto.String(actor), Token: make([]byte, 32), ExpiresInSeconds: proto.Uint64(5), Handle: &networkpb.Token{Owner: proto.String(target), Kind: networkpb.TokenKind_SPACE_HANDLE.Enum(), Value: make([]byte, 32)}}
	newClient := func() *Client {
		return &Client{config: Config{Expected: values.FactAuthorityRef(target), Actor: values.FactAuthorityRef(actor), ActorCertificate: &tls.Certificate{}}}
	}
	for name, change := range map[string]func(*corepb.ReadDelegation){
		"wrong target":       func(g *corepb.ReadDelegation) { g.Target = proto.String(actor) },
		"wrong actor":        func(g *corepb.ReadDelegation) { g.Actor = proto.String(target) },
		"missing actor":      func(g *corepb.ReadDelegation) { g.Actor = nil },
		"empty token":        func(g *corepb.ReadDelegation) { g.Token = nil },
		"short token":        func(g *corepb.ReadDelegation) { g.Token = []byte{1} },
		"missing handle":     func(g *corepb.ReadDelegation) { g.Handle = nil },
		"wrong handle owner": func(g *corepb.ReadDelegation) { g.Handle.Owner = proto.String(actor) },
		"wrong handle role":  func(g *corepb.ReadDelegation) { g.Handle.Kind = networkpb.TokenKind_OBSERVATION_CURSOR.Enum() },
		"zero remaining":     func(g *corepb.ReadDelegation) { g.ExpiresInSeconds = proto.Uint64(0) },
		"unknown field":      func(g *corepb.ReadDelegation) { g.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01}) },
	} {
		t.Run(name, func(t *testing.T) {
			grant := proto.Clone(base).(*corepb.ReadDelegation)
			change(grant)
			if newClient().UseReadDelegation(grant) == nil {
				t.Fatal("accepted malformed or misbound grant")
			}
		})
	}
	client := newClient()
	if err := client.UseReadDelegation(base); err != nil {
		t.Fatal(err)
	}
	if err := client.UseReadDelegation(base); err == nil {
		t.Fatal("replaced installed grant")
	}
	client = newClient()
	client.session = "existing-user-session"
	if client.UseReadDelegation(base) == nil {
		t.Fatal("mixed user and delegated sessions")
	}
	client = newClient()
	client.config.ActorCertificate = nil
	if client.UseReadDelegation(base) == nil {
		t.Fatal("accepted actor without certificate")
	}
}
