// Package admission owns the single inbound session, grant, challenge and
// approval lifecycle. It has no fact Store, query graph or outbound clients.
package admission

import (
	"context"
	"sync"
	"time"

	"ucupa/internal/grpcbound"
	networkpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

// Owner is consumed only for the original Enter/Close grant binding.
type Owner interface {
	PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error)
}

// Authenticator is supplied by trusted composition. Failure values are limited
// to PublicDenied (bad evidence) and PublicUnavailable (dependency failure).
type Authenticator interface {
	Establish(context.Context, values.RealmID, [16]byte, [32]byte) (values.CallerContext, error)
}

// MountOwner is consumed by approval and authenticated peer preparation.
// It cannot inspect application state or obtain a Store or transaction callback.
type MountOwner interface {
	CommandCommitment(values.FactAuthorityRef, values.FactAuthorityRef, corecontract.OperationID, corecontract.MountCommand) ([32]byte, error)
	MountPeer() corecontract.MountPeerPort
	PrepareApprovedMountControl(context.Context, values.CallerContext, values.FactAuthorityRef, corecontract.MountPrepareRequest, values.AuthorizationView, [32]byte) (corecontract.MountPrepared, error)
	AuthorizeMountAcceptance(context.Context, values.CallerContext, values.ElementID) (values.AuthorizationView, error)
	CheckMountAcceptance(context.Context, values.CallerContext, values.ElementID, values.AuthorizationView) error
}

// ChallengeSource mints one single-use identity challenge through the serving
// Realm when it is remote, so the proof's challenge is also registered at the
// Realm that will verify it. Nil keeps the in-process mint; a delegated
// challenge is still registered in this service's request-bound registry and
// never extends the requested TTL bound.
type ChallengeSource interface {
	BeginChallenge(context.Context) ([32]byte, time.Time, error)
}

// Config is fixed trusted assembly, not a second live host configuration.
// Its pointer-valued policies were already validated and copied by the host.
type Config struct {
	Expected             values.FactAuthorityRef
	NextHop              values.NextHop
	SessionTTL           time.Duration
	RPCDeadline          time.Duration
	MaxSessions          int
	MaxSessionsPerCaller int
	MaxConcurrentRPCs    int
	MaxResponseBytes     int
	ReadDelegation       *ReadDelegationConfig
	IdentityProofs       IdentityProofIssuer
	ExternalIdentity     ExternalIdentity
	ChallengeSource      ChallengeSource
	MountApprovals       *MountApprovalConfig
	TransitionApprovals  *TransitionApprovalConfig
	TransitionImport     TransitionImportOwner
	TransitionTarget     TransitionAcceptanceOwner
	CarrierApprovals     *CarrierApprovalConfig
	CarrierApproval      CarrierApprovalOwner
	CarrierTarget        CarrierAcceptanceOwner
	MountControl         MountOwner
	Federation           FederationSource
}

type Service struct {
	networkpb.UnimplementedSessionServiceServer
	pb.UnimplementedReadDelegationServiceServer
	networkpb.UnimplementedIdentityProofServiceServer
	pb.UnimplementedMountApprovalServiceServer
	pb.UnimplementedTransitionApprovalServiceServer
	pb.UnimplementedCarrierApprovalServiceServer
	pb.UnimplementedMountPeerServiceServer
	pb.UnimplementedFederationReadServiceServer
	owner               Owner
	auth                Authenticator
	config              Config
	mu                  sync.Mutex
	sessions            map[[32]byte]session
	stopped             bool
	active              chan struct{}
	running             sync.WaitGroup
	listener            *grpcbound.Listener
	grants              map[[32]byte]readGrant
	pending             map[values.PrincipalRef]int
	challenges          map[[32]byte]identityChallenge
	approvals           map[[32]byte]mountApproval
	transitionApprovals map[[32]byte]transitionApproval
	carrierApprovals    map[[32]byte]carrierApproval
}

// New receives the already validated host policies and creates each registry
// once. RPC implementations operate on this owner directly, without proxies.
func New(owner Owner, auth Authenticator, config Config) *Service {
	return &Service{owner: owner, auth: auth, config: config,
		sessions: make(map[[32]byte]session), active: make(chan struct{}, config.MaxConcurrentRPCs),
		grants: make(map[[32]byte]readGrant), pending: make(map[values.PrincipalRef]int),
		challenges: make(map[[32]byte]identityChallenge), approvals: make(map[[32]byte]mountApproval), transitionApprovals: make(map[[32]byte]transitionApproval),
		carrierApprovals: make(map[[32]byte]carrierApproval)}
}
