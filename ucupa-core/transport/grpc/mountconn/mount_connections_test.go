package mountconn

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	coresdk "ucupa/ucupa-core/sdk/go"
)

type testCommitment struct {
	authority values.FactAuthorityRef
	parent    values.FactAuthorityRef
	carrier   values.FactAuthorityRef
	id        corecontract.OperationID
	command   corecontract.MountCommand
	digest    [32]byte
}

func (c *testCommitment) AuthorityRef() values.FactAuthorityRef { return c.authority }
func (c *testCommitment) CommandCommitment(parent, carrier values.FactAuthorityRef, id corecontract.OperationID, command corecontract.MountCommand) ([32]byte, error) {
	if parent != c.parent || carrier != c.carrier || id != c.id || command != c.command {
		return [32]byte{}, corecontract.ErrInvalid
	}
	return c.digest, nil
}

const (
	testSource    values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	testTarget    values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	testElement   values.ElementID        = "00000000-0000-4000-8000-000000000003"
	testOther     values.ElementID        = "00000000-0000-4000-8000-000000000004"
	testCarrier   values.NextHop          = "local:carrier"
	testRemoteHop values.NextHop          = "https://127.0.0.1:1"
	testParent    values.NextHop          = "local:parent"
)

func testMountIntent(t *testing.T) (corecontract.OperationID, corecontract.MountCommand, [32]byte, *testCommitment) {
	t.Helper()
	var scope [32]byte
	scope[0] = 1
	revision, err := (corecontract.ServiceRevision{Incarnation: scope, Element: testElement, Sequence: 1}).ElementRevision()
	if err != nil {
		t.Fatal(err)
	}
	id := corecontract.OperationID("mountconn-test")
	command := corecontract.MountCommand{
		Kind:                    corecontract.MountEstablish,
		MountAnchorUUID:         testElement,
		ExpectedElementRevision: revision,
		CarrierAuthority:        testTarget,
		CarrierNextHop:          testCarrier,
		ParentNextHop:           testParent,
	}
	digest := [32]byte{0x71, 0x29}
	return id, command, digest, &testCommitment{authority: testSource, parent: testSource, carrier: testTarget, id: id, command: command, digest: digest}
}

func testApproval(t *testing.T, id corecontract.OperationID, digest [32]byte) *pb.MountApproval {
	t.Helper()
	return &pb.MountApproval{
		Target: proto.String(string(testTarget)),
		Binding: &pb.MountApprovalBinding{
			Actor:         proto.String(string(testSource)),
			Element:       proto.String(string(testElement)),
			OperationId:   []byte(id),
			CommandDigest: append([]byte(nil), digest[:]...),
		},
		Token:            bytes.Repeat([]byte{0x51}, 32),
		ExpiresInSeconds: proto.Uint64(30),
	}
}

func TestConfiguredFor(t *testing.T) {
	_, _, _, commitment := testMountIntent(t)
	tests := []struct {
		name     string
		owner    *MountConnections
		expected values.FactAuthorityRef
		want     bool
	}{
		{name: "nil", owner: nil, expected: testSource},
		{name: "missing commitment", owner: &MountConnections{source: testSource}, expected: testSource},
		{name: "matching source", owner: &MountConnections{source: testSource, commitment: commitment}, expected: testSource, want: true},
		{name: "source mismatch", owner: &MountConnections{source: testTarget, commitment: commitment}, expected: testSource},
		{name: "preloaded approval", owner: &MountConnections{source: testSource, commitment: commitment, approval: &pb.MountApproval{}}, expected: testSource},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.owner.ConfiguredFor(tc.expected); got != tc.want {
				t.Fatalf("ConfiguredFor() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWithApprovalClonesInputAndLeavesBaseUnchanged(t *testing.T) {
	id, command, digest, commitment := testMountIntent(t)
	base := &MountConnections{source: testSource, commitment: commitment, peers: make(map[values.NextHop]*coresdk.Client)}
	approval := testApproval(t, id, digest)
	derived, err := base.WithApproval(approval)
	if err != nil {
		t.Fatal(err)
	}
	if base.approval != nil {
		t.Fatal("WithApproval mutated the base connector")
	}
	if derived == base || derived.approval == approval || derived.approval.Binding == approval.Binding {
		t.Fatal("WithApproval did not clone the approval message")
	}
	if derived.commitment != commitment {
		t.Fatal("WithApproval did not preserve the configured commitment owner")
	}
	wantToken := append([]byte(nil), approval.Token...)

	approval.Target = proto.String(string(testSource))
	approval.Binding.Actor = proto.String(string(testTarget))
	approval.Binding.CommandDigest[0] ^= 0xff
	approval.Token[0] ^= 0xff
	approval.ExpiresInSeconds = proto.Uint64(0)

	if derived.approval.GetTarget() != string(testTarget) ||
		derived.approval.Binding.GetActor() != string(testSource) ||
		!bytes.Equal(derived.approval.Token, wantToken) ||
		derived.approval.GetExpiresInSeconds() != 30 {
		t.Fatal("mutating the input approval contaminated the derived connector")
	}
	if _, err := derived.exactApproval(testTarget, id, command); err != nil {
		t.Fatalf("cloned approval no longer binds the original commitment: %v", err)
	}
}

func TestWithApprovalRejectsInvalidBindingAndExpiry(t *testing.T) {
	id, _, digest, commitment := testMountIntent(t)
	base := &MountConnections{source: testSource, commitment: commitment}
	approval := testApproval(t, id, digest)
	if _, err := base.WithApproval(nil); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("nil approval error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*pb.MountApproval)
	}{
		{name: "wrong actor", mutate: func(a *pb.MountApproval) { a.Binding.Actor = proto.String(string(testTarget)) }},
		{name: "empty operation", mutate: func(a *pb.MountApproval) { a.Binding.OperationId = nil }},
		{name: "zero digest", mutate: func(a *pb.MountApproval) { a.Binding.CommandDigest = make([]byte, 32) }},
		{name: "short token", mutate: func(a *pb.MountApproval) { a.Token = []byte{1} }},
		{name: "self target", mutate: func(a *pb.MountApproval) { a.Target = proto.String(string(testSource)) }},
		{name: "zero expiry", mutate: func(a *pb.MountApproval) { a.ExpiresInSeconds = proto.Uint64(0) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := proto.Clone(approval).(*pb.MountApproval)
			tc.mutate(candidate)
			if _, err := base.WithApproval(candidate); !errors.Is(err, corecontract.ErrInvalid) {
				t.Fatalf("invalid approval error = %v", err)
			}
		})
	}
}

func TestCheckApprovalPreflightRetainsCommitmentAndBindingRejection(t *testing.T) {
	id, command, digest, commitment := testMountIntent(t)
	base := &MountConnections{source: testSource, commitment: commitment}
	approval := testApproval(t, id, digest)
	derived, err := base.WithApproval(approval)
	if err != nil {
		t.Fatal(err)
	}
	changed := command
	changed.ParentNextHop = "local:other"
	if _, err := derived.exactApproval(testTarget, id, changed); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("changed commitment error = %v", err)
	}
	derived.approval.Binding.Element = proto.String(string(testOther))
	if _, err := derived.exactApproval(testTarget, id, command); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("changed binding error = %v", err)
	}

	// Use a real, lazily connected SDK client. The unreachable endpoint is
	// intentional: CheckApproval must reject the invalid target/commitment
	// before it attempts a remote RPC.
	client, err := coresdk.New(coresdk.Config{
		NextHop: testRemoteHop, Expected: testTarget, Roots: x509.NewCertPool(),
		SPKISHA256: [32]byte{1}, RPCDeadline: time.Second, ConnectTimeout: time.Second,
		MaxResponseBytes: 1 << 20, MaxMetadataBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	derived.peers = map[values.NextHop]*coresdk.Client{testRemoteHop: client}
	if _, err := derived.CheckApproval(context.Background(), testRemoteHop, testTarget, id, changed); !errors.Is(err, corecontract.ErrInvalid) {
		t.Fatalf("invalid CheckApproval preflight error = %v", err)
	}

	noApproval := &MountConnections{source: testSource, commitment: commitment, peers: map[values.NextHop]*coresdk.Client{testRemoteHop: client}}
	if _, err := noApproval.CheckApproval(context.Background(), testRemoteHop, testTarget, id, command); !errors.Is(err, corecontract.ErrUnavailable) {
		t.Fatalf("missing approval error = %v", err)
	}
}

func TestNewMountConnectionsRequiresSameOwnerCommitment(t *testing.T) {
	_, _, _, commitment := testMountIntent(t)
	if _, err := NewMountConnections(testSource, commitment, nil); err != nil {
		t.Fatalf("same-owner commitment rejected: %v", err)
	}
	for name, candidate := range map[string]CommandCommitment{
		"nil":       nil,
		"typed nil": (*testCommitment)(nil),
		"wrong owner": &testCommitment{
			authority: testTarget,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewMountConnections(testSource, candidate, nil); !errors.Is(err, corecontract.ErrInvalid) {
				t.Fatalf("invalid commitment error = %v", err)
			}
		})
	}
}

func TestExactApprovalRejectsCommitmentOwnerDrift(t *testing.T) {
	id, command, digest, commitment := testMountIntent(t)
	base := &MountConnections{source: testSource, commitment: commitment}
	derived, err := base.WithApproval(testApproval(t, id, digest))
	if err != nil {
		t.Fatal(err)
	}
	commitment.authority = testTarget
	if _, err := derived.exactApproval(testTarget, id, command); !errors.Is(err, corecontract.ErrOwnerMismatch) {
		t.Fatalf("drifted commitment owner error = %v", err)
	}
}
