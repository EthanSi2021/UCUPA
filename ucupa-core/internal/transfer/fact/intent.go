package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	"ucupa/ucupa-core/internal/token"
)

type Profile uint8

const (
	SliceTransfer Profile = iota + 1
	RootSuccessor
	CarrierReplacement
)

// Intent is exact caller input. RootSuccessor has no final bounded tuple;
// its original root UUID and unbounded, parentless shape cannot be rewritten.
// It is not a source fence, target approval, placement reservation or credential.
// Contacts are designated addresses; neither may be resolved from an owner Ref.
type Intent struct {
	Profile       Profile
	Operation     contract.OperationID
	Actor         values.PrincipalRef
	Source        values.FactAuthorityRef
	Target        values.FactAuthorityRef
	Boundary      values.ElementID
	Expected      contract.Revision
	Final         contract.SpatialTuple
	SourceAddress values.NextHop
	TargetAddress values.NextHop
}

func (i Intent) Valid() bool {
	placement := i.Profile == SliceTransfer && i.Final.Parent.Valid() && geometry.CanonicalPose(i.Final.LocalPose) && geometry.ValidSphere(i.Final.Range)
	if i.Profile == RootSuccessor {
		placement = i.Final == (contract.SpatialTuple{})
	}
	return len(i.Operation) > 0 && len(i.Operation) <= 128 && i.Actor.Valid() &&
		i.Source.Valid() && i.Target.Valid() && i.Source != i.Target && i.Boundary.Valid() &&
		token.ValidRevision(i.Expected) && token.RevisionElement(i.Expected) == i.Boundary &&
		placement &&
		i.SourceAddress.Valid() && i.TargetAddress.Valid()
}

// IntentProgress has no implied target acceptance or installation state.
// Fenced certifies only source withdrawal; it is not a target installation token.
// Cancellation can never be inferred from network silence.
type IntentProgress uint8

const (
	IntentPending IntentProgress = iota + 1
	IntentCancelled
	IntentFenced
	IntentCommitted
	IntentSettled
)

type IntentRecord struct {
	Input            Intent
	Progress         IntentProgress
	Revision         uint64 // owner's control-journal sequence, not an Element Revision
	Admission        contract.CoreReadCut
	FinalCut         contract.CoreReadCut // source's last active cut, only after its durable fence
	FenceRevision    uint64               // immutable fence event; later decisions never replace its result
	DecisionRevision uint64               // immutable unique successor event, retained after settlement
	// Origin is derived from the fenced manifest, never caller-supplied.
	// Until source settlement it anchors the source's ancestor protection and
	// unresolved coverage. It is not a second copy of the transferred closure.
	Origin contract.SpatialTuple
}

// SourceClosed remains true after the successor decision: neither commitment
// nor receipt recovery is evidence that the target installed the facts.
func (r IntentRecord) SourceClosed() bool {
	return r.SourcePending() || r.Progress == IntentSettled
}

// SourcePending governs unresolved coverage and ancestor protection, not the
// lifetime of the source fence or its original result.
func (r IntentRecord) SourcePending() bool {
	return r.Progress == IntentFenced || r.Progress == IntentCommitted
}

// SourceFence binds one durable source withdrawal. It is evidence data, not a
// credential: the consumer must obtain it from the designated, authenticated
// source. A self-reported matching digest is never permission to accept/install.
type SourceFence struct {
	Input          Intent
	FinalCut       contract.CoreReadCut
	ManifestDigest [32]byte
}

// TransferKey qualifies an operation by its actual decision owner. A carrier
// replacement is parent-controlled, so its source is not its control owner.
type TransferKey struct {
	Controller values.FactAuthorityRef
	Operation  contract.OperationID
}

func (k TransferKey) Valid() bool {
	return k.Controller.Valid() && len(k.Operation) > 0 && len(k.Operation) <= 128
}

// Both profiles described by Intent are source-controlled.
func (i Intent) Key() TransferKey { return TransferKey{Controller: i.Source, Operation: i.Operation} }

func (f SourceFence) Key() TransferKey { return f.Input.Key() }

type TargetAcceptance struct {
	Fence    SourceFence
	At       contract.CoreReadCut
	Revision uint64 // target control journal sequence, not an Element Revision
}

// SourceDecision is the unique successor decision for an owner-local slice.
// It binds the exact durable target acceptance, not merely its target address.
// Like SourceFence it must be obtained from the authenticated control owner;
// these bytes alone do not grant a caller permission to install anything.
type SourceDecision struct {
	Acceptance TargetAcceptance
	At         contract.CoreReadCut
	Revision   uint64
}

type TargetInstallation struct {
	Decision SourceDecision
	At       contract.CoreReadCut // target cut after the single atomic installation
	Revision uint64               // target control journal sequence
}

// SourceSettlement binds the installed result and the source's departed
// adjacency in one durable cut. It does not grant access to the successor.
type SourceSettlement struct {
	Installation TargetInstallation
	At           contract.CoreReadCut
	Revision     uint64
}

// SuccessorStatement is the source's durable successor disclosure for one
// departed element. It exists only after the unique successor decision, so a
// fenced-only, cancelled or carrier-controlled departure, and an ordinary
// deletion, are all proven absences. Like SourceFence it is evidence data that
// must be obtained from the designated, authenticated source; it binds no
// manifest content and grants no access to the successor.
type SuccessorStatement struct {
	Operation contract.OperationID
	Source    values.FactAuthorityRef
	Target    values.FactAuthorityRef
	FinalCut  contract.CoreReadCut
	Settled   bool
}
