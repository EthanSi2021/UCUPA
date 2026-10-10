package contract

import (
	"context"
	"errors"

	"ucupa/shared/values"
	coretoken "ucupa/ucupa-core/internal/token"
)

var (
	ErrMountBusy                 = errors.New("Core mount control or content write gate is occupied")
	ErrMountCoordinationRequired = errors.New("Core mount range change requires coordination")
	ErrMountContextRequired      = errors.New("Core read requires a mount context")
	ErrUnavailable               = errors.New("Core peer is unavailable")
	ErrOwnerMismatch             = errors.New("Core peer identity does not match")
)

const (
	PermissionMountControl values.PermissionID = "ucupa.core.anchor.mount-control"
	PermissionMountAccept  values.PermissionID = "ucupa.core.anchor.mount-accept"
	PermissionMountRead    values.PermissionID = "ucupa.core.anchor.mount-read"
)

// LocalCallers contains independently established callers, not credentials.
// The trusted local boundary supplies an entry for every required owner.
type LocalCallers map[values.FactAuthorityRef]values.PrincipalRef

type MountCommandKind uint8

const (
	MountEstablish MountCommandKind = iota + 1
	MountUpdate
	MountRevoke
)

// MountCommand is a closed tagged intent. Fields irrelevant to Kind must be zero.
type MountCommand struct {
	Kind                    MountCommandKind
	MountAnchorUUID         values.ElementID
	ExpectedElementRevision Revision
	ExpectedMountRevision   MountRevision
	CarrierAuthority        values.FactAuthorityRef
	CarrierNextHop          values.NextHop
	ParentNextHop           values.NextHop
	Final                   SpatialTuple
}

type MountKey = coretoken.MountKey

type MountAttemptKey struct {
	ParentAuthority values.FactAuthorityRef
	OperationID     OperationID
}

type MountAdmissionKey struct {
	MountKey    MountKey
	OperationID OperationID
}

// NextHopRevision belongs to one direction of one exact relationship.
type NextHopRevision = coretoken.NextHopRevision

type MountState uint8

const (
	MountNone MountState = iota
	MountEstablishing
	MountActive
	MountRevoking
	MountDetached
)

type MountDecision uint8

const (
	MountPending MountDecision = iota + 1
	MountCommitted
	MountAborted
)

type MountAbortCause uint8

const (
	MountAbortNone MountAbortCause = iota
	MountAbortDenied
	MountAbortNotFound
	MountAbortConflict
	MountAbortRevisionConflict
)

type MountBlockedBy uint8

const (
	MountNotBlocked MountBlockedBy = iota
	MountBlockedUnavailable
	MountBlockedOwnerMismatch
	MountBlockedPeerStore
)

type MountControlResult struct {
	OperationID     OperationID
	MountAnchorUUID values.ElementID
	MountKey        MountKey
	Decision        MountDecision
	Settled         bool
	DecisionCut     CoreReadCut
	Record          *ElementRecord
	Changed         bool
	AbortCause      MountAbortCause
	BlockedBy       MountBlockedBy
}

type MountRead struct {
	State           MountState
	MountRevision   MountRevision
	MountKey        MountKey
	AdmissionKey    MountAdmissionKey
	Range           BoundedSphere
	NextHop         values.NextHop
	NextHopRevision NextHopRevision
	ControlSlot     OperationID
	Cut             CoreReadCut
	Incoming        bool
	WriteGateOpen   bool
}

type NextHopUpdateResult struct {
	OperationID     OperationID
	MountKey        MountKey
	NextHop         values.NextHop
	NextHopRevision NextHopRevision
	Changed         bool
	Cut             CoreReadCut
}

type MountRecoveryItem struct {
	Attempt MountAttemptKey
	Result  MountControlResult
	Err     error
}

type MountRecoveryReport struct{ Items []MountRecoveryItem }

// MountTransport is consumed by Core. Its implementation is bound to this
// source Core by trusted local assembly; it must not trust caller-supplied
// source strings, scan by Ref, or return raw Core/Store objects.
type MountTransport interface {
	Prepare(context.Context, values.NextHop, values.FactAuthorityRef, MountPrepareRequest) (MountPrepared, error)
	Finish(context.Context, values.NextHop, values.FactAuthorityRef, MountFinishRequest) (MountClosed, error)
	ReadDecision(context.Context, values.NextHop, values.FactAuthorityRef, MountAttemptKey) (MountDecisionEvidence, error)
}

// PublicMountCommand carries opaque comparisons, not service revisions or
// requester identities. Resolving it is not authorization to execute it later.
type PublicMountCommand struct {
	Kind            MountCommandKind
	Element         values.ElementID
	ExpectedElement values.PublicToken
	ExpectedMount   values.PublicToken
	Carrier         values.FactAuthorityRef
	CarrierNextHop  values.NextHop
	ParentNextHop   values.NextHop
	Final           SpatialTuple
}

// PublicMountInspection is the parent-side control projection. It deliberately
// omits cut, incarnation, sequence, operation subjects and peer admission keys.
type PublicMountInspection struct {
	Element         values.ElementID
	ElementRevision values.PublicToken
	MountRevision   values.PublicToken
	State           MountState
	Carrier         values.FactAuthorityRef
	NextHop         values.NextHop
}

// PublicMountContact describes one direction of one exact mount. Its token is
// an opaque comparison, not a permission or a locator registry entry.
type PublicMountContact struct {
	Element  values.ElementID
	Incoming bool
	Peer     values.FactAuthorityRef
	NextHop  values.NextHop
	Revision values.PublicToken
}

// MountControlTransport is an invocation-bound peer adapter. CheckApproval
// verifies the target-issued exact approval using the specified contact and
// returns its authenticated original subject, never a request's subject field.
// The same adapter carries that approval on Prepare. Recovery transports must
// not invent a replacement approval when no authenticated invocation exists.
type MountControlTransport interface {
	MountTransport
	CheckApproval(context.Context, values.NextHop, values.FactAuthorityRef, OperationID, MountCommand) (values.PrincipalRef, error)
}
