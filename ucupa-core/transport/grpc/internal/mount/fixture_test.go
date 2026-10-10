package mount

import (
	"bytes"
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	grpcsession "ucupa/ucupa-core/transport/grpc/internal/session"
	"ucupa/ucupa-core/transport/grpc/internal/wire"
	"ucupa/ucupa-core/transport/grpc/mountconn"
)

const (
	testOwner   values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
	testTarget  values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	testElement values.ElementID        = "00000000-0000-4000-8000-000000000003"
	testOther   values.ElementID        = "00000000-0000-4000-8000-000000000004"
	testCarrier values.NextHop          = "local:carrier"
	testParent  values.NextHop          = "local:parent"
)

var testCaller = values.CallerContext{
	Subject:       values.PrincipalRef{Realm: "mount-realm", Principal: "mount-caller"},
	Credential:    [16]byte{1},
	Establishment: [32]byte{2},
	Proof:         [32]byte{3},
}

type ownerStub struct {
	authority values.FactAuthorityRef
	calls     []string

	inspectCaller  values.CallerContext
	inspectElement values.ElementID
	inspectResult  corecontract.PublicMountInspection
	inspectErr     error

	resolveCaller  values.CallerContext
	resolveID      corecontract.OperationID
	resolvePublic  corecontract.PublicMountCommand
	resolveCommand corecontract.MountCommand
	resolveView    values.AuthorizationView
	resolveErr     error
	releases       int
	releasedEarly  bool

	describeCaller values.CallerContext
	describeID     corecontract.OperationID
	describePublic corecontract.PublicMountCommand
	describeTarget values.FactAuthorityRef
	describeDigest [32]byte
	describeNeeded bool
	describeErr    error

	applyContext   context.Context
	applyCaller    values.CallerContext
	applyView      values.AuthorizationView
	applyID        corecontract.OperationID
	applyCommand   corecontract.MountCommand
	applyTransport corecontract.MountControlTransport
	applyResult    corecontract.MountControlResult
	applyErr       error

	contactCaller   values.CallerContext
	contactElement  values.ElementID
	contactIncoming bool
	contactResult   corecontract.PublicMountContact
	contactErr      error

	repairCaller   values.CallerContext
	repairID       corecontract.OperationID
	repairElement  values.ElementID
	repairIncoming bool
	repairExpected values.PublicToken
	repairNextHop  values.NextHop
	repairResult   corecontract.PublicMountContact
	repairErr      error

	commitmentDigest [32]byte
	commitmentErr    error
}

func (owner *ownerStub) AuthorityRef() values.FactAuthorityRef { return owner.authority }

func (owner *ownerStub) CommandCommitment(values.FactAuthorityRef, values.FactAuthorityRef, corecontract.OperationID, corecontract.MountCommand) ([32]byte, error) {
	return owner.commitmentDigest, owner.commitmentErr
}

func (owner *ownerStub) InspectMountControl(_ context.Context, caller values.CallerContext, element values.ElementID) (corecontract.PublicMountInspection, error) {
	owner.calls = append(owner.calls, "inspect")
	owner.inspectCaller, owner.inspectElement = caller, element
	return owner.inspectResult, owner.inspectErr
}

func (owner *ownerStub) ResolveMountControl(_ context.Context, caller values.CallerContext, id corecontract.OperationID, command corecontract.PublicMountCommand) (corecontract.MountCommand, values.AuthorizationView, func(), error) {
	owner.calls = append(owner.calls, "resolve")
	owner.resolveCaller, owner.resolveID, owner.resolvePublic = caller, id, command
	if owner.resolveErr != nil {
		return owner.resolveCommand, owner.resolveView, nil, owner.resolveErr
	}
	return owner.resolveCommand, owner.resolveView, func() { owner.releases++ }, nil
}

func (owner *ownerStub) DescribeMountApproval(_ context.Context, caller values.CallerContext, id corecontract.OperationID, command corecontract.PublicMountCommand) (values.FactAuthorityRef, [32]byte, bool, error) {
	owner.calls = append(owner.calls, "describe")
	owner.describeCaller, owner.describeID, owner.describePublic = caller, id, command
	return owner.describeTarget, owner.describeDigest, owner.describeNeeded, owner.describeErr
}

func (owner *ownerStub) ApplyMountControl(ctx context.Context, caller values.CallerContext, view values.AuthorizationView, id corecontract.OperationID, command corecontract.MountCommand, transport corecontract.MountControlTransport) (corecontract.MountControlResult, error) {
	owner.releasedEarly = owner.releases != 0
	owner.calls = append(owner.calls, "apply")
	owner.applyContext, owner.applyCaller, owner.applyView = ctx, caller, view
	owner.applyID, owner.applyCommand, owner.applyTransport = id, command, transport
	return owner.applyResult, owner.applyErr
}

func (owner *ownerStub) InspectMountContact(_ context.Context, caller values.CallerContext, element values.ElementID, incoming bool) (corecontract.PublicMountContact, error) {
	owner.calls = append(owner.calls, "inspect-contact")
	owner.contactCaller, owner.contactElement, owner.contactIncoming = caller, element, incoming
	return owner.contactResult, owner.contactErr
}

func (owner *ownerStub) RepairMountContact(_ context.Context, caller values.CallerContext, id corecontract.OperationID, element values.ElementID, incoming bool, expected values.PublicToken, nextHop values.NextHop) (corecontract.PublicMountContact, error) {
	owner.calls = append(owner.calls, "repair-contact")
	owner.repairCaller, owner.repairID, owner.repairElement = caller, id, element
	owner.repairIncoming, owner.repairExpected, owner.repairNextHop = incoming, expected, nextHop
	return owner.repairResult, owner.repairErr
}

type serviceFixture struct {
	owner       *ownerStub
	connections *mountconn.MountConnections
	service     *Service
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	owner := &ownerStub{authority: testOwner, commitmentDigest: [32]byte{0x41}}
	connections, err := mountconn.NewMountConnections(testOwner, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &serviceFixture{owner: owner, connections: connections, service: New(owner, testOwner, connections)}
}

func (fixture *serviceFixture) appliedWithLocalApprovalCopy() bool {
	connection, ok := fixture.owner.applyTransport.(*mountconn.MountConnections)
	return ok && connection != fixture.connections && fixture.connections.ConfiguredFor(testOwner) && !connection.ConfiguredFor(testOwner)
}

func callerContext() context.Context {
	return grpcsession.WithCaller(context.Background(), testCaller)
}

func grantedContext(withCaller bool) context.Context {
	ctx := context.Background()
	if withCaller {
		ctx = grpcsession.WithCaller(ctx, testCaller)
	}
	return grpcsession.WithGrant(ctx, [32]byte{9})
}

func testToken(t *testing.T, kind string, seed byte) values.PublicToken {
	t.Helper()
	raw := [32]byte{seed}
	token, err := values.PublicTokenFromBytes(testOwner, kind, raw)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func testPublicCommand(t *testing.T) (*pb.PublicMountCommand, corecontract.PublicMountCommand) {
	t.Helper()
	expected := testToken(t, "record-revision", 1)
	command := corecontract.PublicMountCommand{
		Kind:            corecontract.MountEstablish,
		Element:         testElement,
		ExpectedElement: expected,
		Carrier:         testTarget,
		CarrierNextHop:  testCarrier,
		ParentNextHop:   testParent,
	}
	return &pb.PublicMountCommand{
		Kind:            pb.MountCommandKind_MOUNT_ESTABLISH.Enum(),
		Element:         proto.String(string(testElement)),
		ExpectedElement: wire.ControlToken(expected, networkpb.TokenKind_RECORD_REVISION),
		CarrierCore:     proto.String(string(testTarget)),
		CarrierNextHop:  proto.String(string(testCarrier)),
		ParentNextHop:   proto.String(string(testParent)),
	}, command
}

func testResolvedCommand(t *testing.T) corecontract.MountCommand {
	t.Helper()
	revision, err := (corecontract.ServiceRevision{Incarnation: [32]byte{3}, Element: testElement, Sequence: 4}).ElementRevision()
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.MountCommand{
		Kind:                    corecontract.MountEstablish,
		MountAnchorUUID:         testElement,
		ExpectedElementRevision: revision,
		CarrierAuthority:        testTarget,
		CarrierNextHop:          testCarrier,
		ParentNextHop:           testParent,
	}
}

func testApproval(operation []byte, digest [32]byte) *pb.MountApproval {
	return &pb.MountApproval{
		Target: proto.String(string(testTarget)),
		Binding: &pb.MountApprovalBinding{
			Actor:         proto.String(string(testOwner)),
			Element:       proto.String(string(testElement)),
			OperationId:   append([]byte(nil), operation...),
			CommandDigest: append([]byte(nil), digest[:]...),
		},
		Token:            bytes.Repeat([]byte{0x51}, 32),
		ExpiresInSeconds: proto.Uint64(30),
	}
}
