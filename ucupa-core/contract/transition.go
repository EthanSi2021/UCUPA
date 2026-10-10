package contract

import "ucupa/shared/values"

const (
	// Export authorizes the exact boundary and its owner-local closure. It is
	// not implied by CRUD, query, mount or writer-control permissions.
	PermissionTransitionExport  values.PermissionID = "ucupa.core.transition.export"
	PermissionTransitionImport  values.PermissionID = "ucupa.core.transition.import"
	PermissionTransitionCommit  values.PermissionID = "ucupa.core.transition.commit"
	PermissionTransitionInspect values.PermissionID = "ucupa.core.transition.inspect"
)

type TransitionKind uint8

const (
	TransitionSlice TransitionKind = iota + 1
	TransitionRoot
)

// TransitionCommand describes one source-controlled transfer. The receiving
// Core supplies Source and the authenticated caller supplies the actor; neither
// is a caller assertion in this value. Expected is the source's public Element
// revision, not a private Store sequence. Addresses are explicit, not Ref routes.
type TransitionCommand struct {
	Kind          TransitionKind
	Target        values.FactAuthorityRef
	Boundary      values.ElementID
	Expected      values.PublicToken
	Final         SpatialTuple
	SourceAddress values.NextHop
	TargetAddress values.NextHop
}

// TransitionApprovalBinding limits target-side authorization to one source,
// operation and exact command. It is a description, never authentication or
// proof that the source has fenced or that the target has accepted content.
type TransitionApprovalBinding struct {
	Source    values.FactAuthorityRef
	Operation OperationID
	Command   TransitionCommand
}

type TransitionProgress uint8

const (
	TransitionIntent TransitionProgress = iota + 1
	TransitionCancelled
	TransitionFenced
	TransitionCommitted
	TransitionSettled
)

// TransitionStatus discloses this source's durable progress only. INTENT is
// not target approval, FENCED is not acceptance, and COMMITTED is not proof of
// current target availability. Target acceptance/installation remain distinct
// obligations; a source status is never an installation credential.
type TransitionStatus struct {
	Source    values.FactAuthorityRef
	Operation OperationID
	Kind      TransitionKind
	Target    values.FactAuthorityRef
	Boundary  values.ElementID
	Progress  TransitionProgress
	Revision  values.PublicToken
}

// TransitionTerminal is the one terminal transition evidence a source
// observation discloses when its old scope provably ended under one unique
// committed successor decision. FinalCut is that decision's public cut serial.
// It binds no manifest content and grants no access to the successor.
type TransitionTerminal struct {
	Operation OperationID
	Source    values.FactAuthorityRef
	Target    values.FactAuthorityRef
	FinalCut  uint64
	Settled   bool
	NextHop   values.NextHop
}
