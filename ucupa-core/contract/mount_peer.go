package contract

import (
	"context"

	"ucupa/shared/values"
)

// These messages carry exact retained evidence. They are a trusted local
// profile, not remotely authenticated messages or public wire encodings.
type MountPrepareRequest struct {
	Key               MountKey
	OperationID       OperationID
	Command           MountCommand
	ParentActor       values.PrincipalRef
	CarrierActor      values.PrincipalRef
	PreviousAdmission MountAdmissionKey
	ReservedRevision  MountRevision
	Tuple             SpatialTuple
}

type MountPrepared struct {
	Request   MountPrepareRequest
	Prepared  bool
	Rejection MountAbortCause
	Cut       CoreReadCut
}

type MountDecisionEvidence struct {
	Request      MountPrepareRequest
	Decision     MountDecision
	DecisionCut  CoreReadCut
	AbortCause   MountAbortCause
	AdmissionKey MountAdmissionKey
}

type MountFinishRequest struct{ Evidence MountDecisionEvidence }

type MountClosed struct {
	Evidence MountDecisionEvidence
	Cut      CoreReadCut
}

// MountPeerPort is solely for trusted assembly, not an application protocol.
// The adapter establishes Source independently of the request payload.
type MountPeerPort interface {
	AuthorityRef() values.FactAuthorityRef
	Prepare(context.Context, values.FactAuthorityRef, MountPrepareRequest) (MountPrepared, error)
	Finish(context.Context, values.FactAuthorityRef, MountFinishRequest) (MountClosed, error)
	ReadDecision(context.Context, values.FactAuthorityRef, MountAttemptKey) (MountDecisionEvidence, error)
}
