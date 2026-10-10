// Package encoding owns bounded transfer evidence and journal representations.
// It has no live owner, transaction, database, network, signer or shared state.
// Both normal execution and replay consume this one canonical encoding.
package encoding

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// These bounds reserve terminal control room for each admitted intent. The
// separately bounded manifests cannot consume cancellation headroom. A full
// journal never evicts an original outcome or frees its operation ID.
const MaxTransferIntents = 1024

const MaxTransferIntentBytes = 4096

// 6144 bounded control envelopes plus 1024 compact completion envelopes,
// and <=8 MiB retained manifests, closure proofs and results (base64 <11 MiB).
// Reserve completion bytes as well as event slots; settlement cannot get stuck
// merely because other admissible tasks consumed its terminal journal room.
const MaxTransferJournalBytes = 64 << 20

type TransferEventKind uint8

const (
	TransferPending TransferEventKind = iota + 1
	TransferCancelled
	TransferFenced
	TransferTargetAccepted
	TransferSuccessorDecided
	TransferTargetInstalled
	TransferSourceSettled
	TransferCarrierReserved
	TransferCarrierPermitted
	TransferCarrierCancelled
	TransferCarrierRefused
	TransferCarrierAborted
	TransferCarrierFenced
	TransferCarrierAccepted
	TransferCarrierBound
	TransferCarrierInstalled
	TransferCarrierParentSettled
	TransferCarrierSourceSettled
	TransferEdgeClosed
	TransferEdgeInstalled
	TransferTargetCompleted
	TransferParentReserved
	TransferParentReleased
	TransferFacetReserved
	TransferFacetPermitted
	TransferFacetFenced
	TransferFacetAccepted
	TransferFacetCommitted
	TransferFacetInstalled
	TransferFacetSettled
	TransferFacetCancelled
	TransferFacetRefused
)

const MaxTransferTargets = 1024

const MaxTransferEvents = 4*MaxTransferIntents + 3*MaxTransferTargets

type TransferEvent struct {
	TargetParent        string // canonical retained upstream proof, target acceptance only
	Parent              transferfact.ParentIntent
	ParentRelease       transferfact.ParentRelease
	EdgeResults         string
	Completion          transferfact.CoreCompletion
	EdgeParent          transferfact.CoreInstallation
	EdgeMount           values.ElementID
	EdgeFence           transferfact.EdgeFence
	CarrierInstallation transferfact.CarrierInstallation
	CarrierBinding      transferfact.CarrierBinding
	CarrierAcceptance   transferfact.CarrierAcceptance
	CarrierFence        transferfact.CarrierFence
	CarrierPermit       transferfact.CarrierControl
	CarrierRefusal      transferfact.CarrierRefusal
	Carrier             transferfact.CarrierIntent
	CarrierExpected     contract.CoreReadCut
	Facet               transferfact.FacetReplacementIntent  // populated only by the initial facet reservation
	FacetFence          transferfact.FacetReplacementFence   // populated only by the source Facet fence
	FacetRefusal        transferfact.FacetReplacementRefusal // populated only by the source Facet refusal
	Kind                TransferEventKind
	At                  contract.CoreReadCut
	Input               transferfact.Intent  // populated only by the initial intent
	Operation           contract.OperationID // populated by cancellation or source fence
	Actor               values.PrincipalRef
	Expected            uint64
	Manifest            string                          // immutable source bytes: source fence or private target acceptance
	Closures            string                          // canonical carrier holds, target acceptance only
	Fence               transferfact.SourceFence        // target acceptance only
	Acceptance          transferfact.TargetAcceptance   // source successor decision only
	Decision            transferfact.SourceDecision     // target installation only
	Installation        transferfact.TargetInstallation // source settlement only
}
