package coregrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	attachmenttransport "ucupa/ucupa-core/transport/grpc/internal/attachment"
	"ucupa/ucupa-core/transport/grpc/mountconn"
)

const attachmentServerTarget values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const attachmentServerFacet values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"

type attachmentServerOwner struct{}

var _ Owner = (*attachmentServerOwner)(nil)

func (attachmentServerOwner) AuthorityRef() values.FactAuthorityRef { return attachmentServerTarget }
func (attachmentServerOwner) PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error) {
	return nil, nil
}

type attachmentServerAuth struct{}

var _ Authenticator = (*attachmentServerAuth)(nil)

func (attachmentServerAuth) Establish(context.Context, values.RealmID, [16]byte, [32]byte) (values.CallerContext, error) {
	return values.CallerContext{}, nil
}
func (attachmentServerAuth) ForkCaller(_ context.Context, c values.CallerContext) (values.CallerContext, error) {
	return c, nil
}

func attachmentServerConfig() Config {
	return Config{
		Expected:             attachmentServerTarget,
		NextHop:              values.NextHop("https://core.example:443"),
		Certificate:          tls.Certificate{Certificate: [][]byte{{1}}, PrivateKey: struct{}{}},
		SessionTTL:           time.Second,
		RPCDeadline:          time.Second,
		HandshakeTimeout:     time.Second,
		ConnectionLifetime:   time.Second,
		ShutdownTimeout:      time.Second,
		MaxSessions:          1,
		MaxSessionsPerCaller: 1,
		MaxConnections:       1,
		MaxConcurrentRPCs:    1,
		MaxStreams:           1,
		MaxMetadataBytes:     256,
		MaxResponseBytes:     1 << 20,
	}
}

func TestNewWithEmptyFacetServicesAcceptsNarrowOwner(t *testing.T) {
	s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, attachmentServerConfig())
	if err != nil {
		t.Fatalf("empty FacetServices rejected narrow owner: %v", err)
	}
	s.Stop()
}

type attachmentControlServerOwner struct {
	attachmenttransport.Owner
	identity values.FactAuthorityRef
}

func (o *attachmentControlServerOwner) AuthorityRef() values.FactAuthorityRef { return o.identity }

// attachmentFallbackServerOwner deliberately also satisfies the control
// methods. New must still require Config.AttachmentControl explicitly instead
// of rediscovering them through its broad public owner.
type attachmentFallbackServerOwner struct {
	attachmentServerOwner
	attachmenttransport.Owner
}

func TestNewWithFixedFacetServicesRequiresMatchingAttachmentOwner(t *testing.T) {
	for _, tc := range []struct {
		name  string
		port  AttachmentControlOwner
		valid bool
	}{
		{"missing", nil, false},
		{"same owner", &attachmentControlServerOwner{identity: attachmentServerTarget}, true},
		{"other owner", &attachmentControlServerOwner{identity: attachmentServerFacet}, false},
		{"typed nil", (*attachmentControlServerOwner)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := attachmentServerConfig()
			c.FacetServices = map[values.PrincipalRef]values.FactAuthorityRef{
				{Realm: "realm", Principal: "facet"}: attachmentServerFacet,
			}
			c.AttachmentControl = tc.port
			s, err := New(&attachmentFallbackServerOwner{}, &attachmentServerAuth{}, c)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				s.Stop()
			} else if err == nil || err.Error() != "attachment control owner required" {
				if s != nil {
					s.Stop()
				}
				t.Fatalf("invalid attachment owner was not rejected: %v", err)
			}
		})
	}
}

type facetControlServerOwner struct{ identity values.FactAuthorityRef }

func (o *facetControlServerOwner) AuthorityRef() values.FactAuthorityRef { return o.identity }
func (*facetControlServerOwner) InspectFacetControl(context.Context, values.CallerContext, values.ElementID) (corecontract.PublicFacetInspection, error) {
	return corecontract.PublicFacetInspection{}, nil
}
func (*facetControlServerOwner) ApplyFacetControl(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicFacetCommand) (corecontract.MutationStatus, corecontract.PublicFacetInspection, error) {
	return 0, corecontract.PublicFacetInspection{}, nil
}
func (*facetControlServerOwner) BeginFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicFacetReplacementCommand) (corecontract.PublicFacetReplacementStatus, error) {
	return corecontract.PublicFacetReplacementStatus{}, nil
}
func (*facetControlServerOwner) InspectFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID) (corecontract.PublicFacetReplacementStatus, error) {
	return corecontract.PublicFacetReplacementStatus{}, nil
}
func (*facetControlServerOwner) CancelFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID, values.PublicToken) (corecontract.PublicFacetReplacementStatus, error) {
	return corecontract.PublicFacetReplacementStatus{}, nil
}
func (*facetControlServerOwner) RecoverFacetReplacement(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID, replacement.FacetPeers) (corecontract.PublicFacetReplacementStatus, error) {
	return corecontract.PublicFacetReplacementStatus{}, nil
}

func TestNewBindsExplicitFacetControlToTheSameOwner(t *testing.T) {
	for _, tc := range []struct {
		name  string
		port  FacetControlOwner
		valid bool
	}{
		{"absent", nil, true},
		{"same owner", &facetControlServerOwner{attachmentServerTarget}, true},
		{"other owner", &facetControlServerOwner{attachmentServerFacet}, false},
		{"typed nil", (*facetControlServerOwner)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := attachmentServerConfig()
			c.FacetControl = tc.port
			s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				s.Stop()
			} else if err == nil {
				s.Stop()
				t.Fatal("accepted a mismatched Facet control entry")
			}
		})
	}
}

type mountControlServerOwner struct {
	MountControl
	identity values.FactAuthorityRef
}

func (o *mountControlServerOwner) AuthorityRef() values.FactAuthorityRef { return o.identity }

func TestNewBindsExplicitMountControlToTheSameOwner(t *testing.T) {
	for _, tc := range []struct {
		name  string
		port  MountControl
		valid bool
	}{
		{"absent while disabled", nil, true},
		{"same owner", &mountControlServerOwner{identity: attachmentServerTarget}, true},
		{"other owner", &mountControlServerOwner{identity: attachmentServerFacet}, false},
		{"typed nil", (*mountControlServerOwner)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := attachmentServerConfig()
			c.MountControl = tc.port
			s, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				s.Stop()
			} else if err == nil {
				s.Stop()
				t.Fatal("accepted a mismatched Mount control entry")
			}
		})
	}
}

type federationSourceServerOwner struct {
	FederationSource
	identity values.FactAuthorityRef
}

func (o *federationSourceServerOwner) AuthorityRef() values.FactAuthorityRef { return o.identity }
func (*federationSourceServerOwner) PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error) {
	panic("assembly-only Federation source must not execute ordinary public operations")
}

func TestNewBindsExplicitFederationSourceWithoutOwnerFallback(t *testing.T) {
	actual := &federationSourceServerOwner{identity: attachmentServerTarget}
	for _, tc := range []struct {
		name  string
		port  FederationSource
		valid bool
	}{
		{"absent while disabled", nil, true},
		{"same owner", actual, true},
		{"other owner", &federationSourceServerOwner{FederationSource: actual, identity: attachmentServerFacet}, false},
		{"typed nil", (*federationSourceServerOwner)(nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := attachmentServerConfig()
			c.Federation = tc.port
			// The broad public owner deliberately also implements FederationSource.
			// A nil Config port must not be rediscovered from it.
			s, err := New(actual, &attachmentServerAuth{}, c)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				defer s.Stop()
				if tc.port == nil {
					if _, err := s.admission.ProjectFederationReadPart(context.Background(), &corepb.ProjectFederationReadPartRequest{}); status.Code(err) != codes.Unimplemented {
						t.Fatalf("nil Federation fell back to Owner: %v", err)
					}
				}
			} else if err == nil || err.Error() != "invalid Federation source owner" {
				if s != nil {
					s.Stop()
				}
				t.Fatalf("invalid Federation source was not rejected: %v", err)
			}
		})
	}
}

func TestNewRequiresMountControlWhenMountFeatureEnabled(t *testing.T) {
	control := &mountControlServerOwner{identity: attachmentServerTarget}
	connections, err := mountconn.NewMountConnections(attachmentServerTarget, control, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("connections", func(t *testing.T) {
		c := attachmentServerConfig()
		c.MountConnections = connections
		if _, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c); err == nil || err.Error() != "authenticated mount control owner required" {
			t.Fatalf("missing MountControl error = %v", err)
		}
	})
	t.Run("approvals", func(t *testing.T) {
		c := attachmentServerConfig()
		c.ReadDelegation = &ReadDelegationConfig{
			ClientRoots: x509.NewCertPool(),
			Peers: map[values.FactAuthorityRef][32]byte{
				attachmentServerFacet: {1},
			},
			MaxTTL: time.Second, MaxGrants: 1, MaxPerCaller: 1,
		}
		c.MountApprovals = &MountApprovalConfig{MaxTTL: time.Second, MaxApprovals: 1, MaxPerCaller: 1}
		if _, err := New(&attachmentServerOwner{}, &attachmentServerAuth{}, c); err == nil || err.Error() != "authenticated mount owner required" {
			t.Fatalf("missing MountControl error = %v", err)
		}
	})
}
