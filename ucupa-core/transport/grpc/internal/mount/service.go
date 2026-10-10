// Package mount implements the application-side mount control and contact RPCs.
// Approval and peer admission share a separate transport lifetime; this group
// consumes the already authenticated context and never owns an approval table.
package mount

import (
	"context"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/transport/grpc/mountconn"
)

// Owner is the six-method boundary consumed by this five-RPC group.
// The host binds it to the same fact owner before constructing the service.
type Owner interface {
	InspectMountControl(context.Context, values.CallerContext, values.ElementID) (corecontract.PublicMountInspection, error)
	ResolveMountControl(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicMountCommand) (corecontract.MountCommand, values.AuthorizationView, func(), error)
	DescribeMountApproval(context.Context, values.CallerContext, corecontract.OperationID, corecontract.PublicMountCommand) (values.FactAuthorityRef, [32]byte, bool, error)
	ApplyMountControl(context.Context, values.CallerContext, values.AuthorizationView, corecontract.OperationID, corecontract.MountCommand, corecontract.MountControlTransport) (corecontract.MountControlResult, error)
	InspectMountContact(context.Context, values.CallerContext, values.ElementID, bool) (corecontract.PublicMountContact, error)
	RepairMountContact(context.Context, values.CallerContext, corecontract.OperationID, values.ElementID, bool, values.PublicToken, values.NextHop) (corecontract.PublicMountContact, error)
}

type Service struct {
	pb.UnimplementedMountControlServiceServer
	pb.UnimplementedMountContactServiceServer
	owner       Owner
	expected    values.FactAuthorityRef
	connections *mountconn.MountConnections
}

// New retains the already validated, fixed host assembly. A missing owner or
// connection remains RPC-time Unimplemented, not a new construction failure.
func New(owner Owner, expected values.FactAuthorityRef, connections *mountconn.MountConnections) *Service {
	return &Service{owner: owner, expected: expected, connections: connections}
}
