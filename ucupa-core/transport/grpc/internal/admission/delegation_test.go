package admission

import (
	"crypto/x509"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"ucupa/shared/values"
)

func TestDelegationAdmissionIncludesPendingAcrossSubjectsAndCapsExpiry(t *testing.T) {
	a := values.CallerContext{Subject: values.PrincipalRef{Realm: "r", Principal: "a"}}
	b := values.CallerContext{Subject: values.PrincipalRef{Realm: "r", Principal: "b"}}
	ka, kb := [32]byte{1}, [32]byte{2}
	expires := time.Now().Add(5 * time.Second)
	s := &Service{config: Config{ReadDelegation: &ReadDelegationConfig{MaxGrants: 2, MaxPerCaller: 2}},
		sessions: map[[32]byte]session{ka: {caller: a, expires: expires}, kb: {caller: b, expires: expires}},
		grants:   make(map[[32]byte]readGrant), pending: make(map[values.PrincipalRef]int)}
	got, err := s.reserveGrant(a, ka, time.Hour)
	if err != nil || got != expires {
		t.Fatalf("parent expiry not retained: %v %v", got, err)
	}
	if _, err := s.reserveGrant(b, kb, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := s.reserveGrant(a, ka, time.Hour); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("pending permits exceeded global capacity: %v", err)
	}
	s.releaseGrantReservationLocked(a.Subject)
	if _, ok := s.pending[a.Subject]; ok {
		t.Fatal("empty subject reservation leaked")
	}
	s.grants[[32]byte{3}] = readGrant{caller: a, expires: time.Now().Add(-time.Second)}
	if _, err := s.reserveGrant(a, ka, time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(s.grants) != 0 {
		t.Fatal("expired grants occupied admission")
	}
	s.releaseGrantReservationLocked(a.Subject)
	delete(s.sessions, ka)
	if _, err := s.reserveGrant(a, ka, time.Hour); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("released session issued grant: %v", err)
	}
	s.stopped = true
	if s.grantAlive([32]byte{3}, a) {
		t.Fatal("stopped registry still valid")
	}
}

func TestDelegationConfigurationIsExplicitAndCopied(t *testing.T) {
	const actor values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	good := &ReadDelegationConfig{ClientRoots: x509.NewCertPool(), Peers: map[values.FactAuthorityRef][32]byte{actor: {1}}, MaxTTL: time.Second, MaxGrants: 4, MaxPerCaller: 2}
	if out, err := CopyReadDelegationConfig(nil); err != nil || out != nil {
		t.Fatal("omission enabled delegation")
	}
	out, err := CopyReadDelegationConfig(good)
	if err != nil {
		t.Fatal(err)
	}
	good.Peers[actor] = [32]byte{2}
	if out.Peers[actor] != ([32]byte{1}) || out.ClientRoots == good.ClientRoots {
		t.Fatal("live trust aliases caller config")
	}
	for _, change := range []func(*ReadDelegationConfig){
		func(c *ReadDelegationConfig) { c.ClientRoots = nil },
		func(c *ReadDelegationConfig) { c.Peers = nil },
		func(c *ReadDelegationConfig) { c.Peers = map[values.FactAuthorityRef][32]byte{"bad": {1}} },
		func(c *ReadDelegationConfig) { c.Peers = map[values.FactAuthorityRef][32]byte{actor: {}} },
		func(c *ReadDelegationConfig) { c.MaxTTL = 0 },
		func(c *ReadDelegationConfig) { c.MaxTTL = time.Millisecond },
		func(c *ReadDelegationConfig) { c.MaxGrants = 0 },
		func(c *ReadDelegationConfig) { c.MaxPerCaller = 0 },
		func(c *ReadDelegationConfig) { c.MaxPerCaller = 5 },
	} {
		c := *good
		change(&c)
		if _, err := CopyReadDelegationConfig(&c); err == nil {
			t.Fatal("invalid trust or resource policy accepted")
		}
	}
}
