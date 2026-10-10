package coregrpc

import (
	"context"
	"crypto/x509"
	"testing"
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	port "ucupa/ucupa-core/contract/transition"
	"ucupa/ucupa-core/transport/grpc/internal/transition"
	"ucupa/ucupa-core/transport/grpc/transitionconn"
)

type transitionImportServerOwner struct {
	id values.FactAuthorityRef
}

type transitionSourceOwner struct {
	port.SourcePeer
	id values.FactAuthorityRef
}

type transitionTargetOwner struct {
	port.TargetPeer
	id values.FactAuthorityRef
}

type transitionParentOwner struct {
	port.CarrierParentPeer
	id values.FactAuthorityRef
}

func (o *transitionParentOwner) AuthorityRef() values.FactAuthorityRef { return o.id }

func TestTransitionParentIsExplicitAndMatching(t *testing.T) {
	for _, tc := range []struct {
		name  string
		owner port.CarrierParentPeer
		valid bool
	}{
		{"absent", nil, true}, {"same owner", &transitionParentOwner{id: attachmentServerTarget}, true},
		{"other owner", &transitionParentOwner{id: attachmentServerFacet}, false}, {"typed nil", (*transitionParentOwner)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := attachmentServerConfig()
			c.TransitionParent = tc.owner
			s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c)
			if s != nil {
				s.Stop()
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func (o *transitionTargetOwner) AuthorityRef() values.FactAuthorityRef { return o.id }

func TestTransitionTargetRequiresMatchingOwnerConnectionsAndTrust(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-owner", "typed-nil", "missing-owner", "missing-contacts", "wrong-contacts", "missing-trust"} {
		t.Run(mode, func(t *testing.T) {
			c := attachmentServerConfig()
			c.TransitionTarget = &transitionTargetOwner{id: attachmentServerTarget}
			c.TransitionConnections, _ = transitionconn.NewConnections(attachmentServerTarget, nil, 1, time.Second)
			c.ReadDelegation = &ReadDelegationConfig{ClientRoots: x509.NewCertPool(), Peers: map[values.FactAuthorityRef][32]byte{attachmentServerFacet: {1}}, MaxTTL: time.Second, MaxGrants: 2, MaxPerCaller: 1}
			switch mode {
			case "wrong-owner":
				c.TransitionTarget = &transitionTargetOwner{id: attachmentServerFacet}
			case "typed-nil":
				c.TransitionTarget = (*transitionTargetOwner)(nil)
			case "missing-owner":
				c.TransitionTarget = nil
			case "missing-contacts":
				c.TransitionConnections = nil
			case "wrong-contacts":
				c.TransitionConnections, _ = transitionconn.NewConnections(attachmentServerFacet, nil, 1, time.Second)
			case "missing-trust":
				c.ReadDelegation = nil
			}
			s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c)
			if s != nil {
				s.Stop()
			}
			if (err == nil) != (mode == "valid") {
				t.Fatalf("host error: %v", err)
			}
		})
	}
}

func (o *transitionSourceOwner) AuthorityRef() values.FactAuthorityRef { return o.id }

func TestTransitionSourceIsExplicitAndMatching(t *testing.T) {
	for _, tc := range []struct {
		name  string
		owner port.SourcePeer
		valid bool
	}{
		{"absent", nil, true}, {"same owner", &transitionSourceOwner{id: attachmentServerTarget}, true},
		{"other owner", &transitionSourceOwner{id: attachmentServerFacet}, false}, {"typed nil", (*transitionSourceOwner)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := attachmentServerConfig()
			c.TransitionSource = tc.owner
			s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c)
			if s != nil {
				s.Stop()
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func (o *transitionImportServerOwner) AuthorizeTransitionImport(context.Context, values.CallerContext, contract.TransitionApprovalBinding) (values.AuthorizationView, error) {
	return values.AuthorizationView{}, values.PublicDenied
}
func (o *transitionImportServerOwner) CheckTransitionImport(context.Context, values.CallerContext, contract.TransitionApprovalBinding, values.AuthorizationView) error {
	return values.PublicDenied
}

func (o *transitionImportServerOwner) AuthorityRef() values.FactAuthorityRef { return o.id }

func TestTransitionImportHostBindingAndPolicyCopy(t *testing.T) {
	for _, mode := range []string{"valid", "absent owner", "wrong owner", "typed nil", "no trust", "zero ttl"} {
		t.Run(mode, func(t *testing.T) {
			c := attachmentServerConfig()
			c.TransitionImport = &transitionImportServerOwner{id: attachmentServerTarget}
			c.TransitionApprovals = &TransitionApprovalConfig{MaxTTL: time.Second, MaxApprovals: 2, MaxPerCaller: 1}
			c.ReadDelegation = &ReadDelegationConfig{ClientRoots: x509.NewCertPool(), Peers: map[values.FactAuthorityRef][32]byte{attachmentServerFacet: {1}}, MaxTTL: time.Second, MaxGrants: 2, MaxPerCaller: 1}
			switch mode {
			case "absent owner":
				c.TransitionImport = nil
			case "wrong owner":
				c.TransitionImport = &transitionImportServerOwner{id: attachmentServerFacet}
			case "typed nil":
				c.TransitionImport = (*transitionImportServerOwner)(nil)
			case "no trust":
				c.ReadDelegation = nil
			case "zero ttl":
				c.TransitionApprovals.MaxTTL = 0
			}
			s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("host error: %v", err)
			}
			if s != nil {
				defer s.Stop()
				c.TransitionApprovals.MaxTTL = time.Hour
				if s.config.TransitionApprovals.MaxTTL != time.Second {
					t.Fatal("caller mutated live policy")
				}
			}
		})
	}
}

type transitionServerOwner struct {
	transition.Owner
	id values.FactAuthorityRef
}

func (o *transitionServerOwner) AuthorityRef() values.FactAuthorityRef { return o.id }

func TestTransitionOwnerIsExplicitAndMatching(t *testing.T) {
	for _, tc := range []struct {
		name  string
		owner transition.Owner
		valid bool
	}{
		{"absent", nil, true}, {"same owner", &transitionServerOwner{id: attachmentServerTarget}, true},
		{"other owner", &transitionServerOwner{id: attachmentServerFacet}, false}, {"typed nil", (*transitionServerOwner)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := attachmentServerConfig()
			c.TransitionControl = tc.owner
			s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c)
			if s != nil {
				s.Stop()
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
