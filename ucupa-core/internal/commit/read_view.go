package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// readProjection is the finite, immutable read set shared by snapshot and
// connected callers. It deliberately has no Core state, operation/history
// tables, public key, or mutation methods. The two bounded maps contain only
// mount evidence needed by the current mount validator.
type ReadView struct {
	edgeClosure         transferfact.EdgeClosure
	edgeInstallation    transferfact.EdgeInstallation
	carrierInstallation transferfact.CarrierInstallation
	carrierFence        transferfact.CarrierFence
	Authority           values.FactAuthorityRef
	Scope               [32]byte
	CutSerial           uint64
	RevisionSerial      uint64
	extraRevisions      uint64 // bounded batch-install surplus, not retained control history
	Elements            elementfact.View
	Attachments         facetfact.View
	Mount               mountfact.View
	Transfers           transferfact.Visibility
	mountOperations     map[contract.OperationID]MountOperation
	mountAttempts       map[contract.MountAttemptKey]PeerAttempt
	mountInitialized,
	mountOperationsInitialized,
	mountAddressesInitialized,
	mountAttemptsInitialized bool
}

// RevisionsValid allows a single atomic cut to issue several distinct target
// revisions. The finite surplus derives only from durable installed receipts;
// full Store replay still proves the exact issuance of every revision.
func (view ReadView) RevisionsValid() bool {
	return validRevisionSequence(view.RevisionSerial, view.CutSerial, view.extraRevisions)
}

func validRevisionSequence(revision, cut, surplus uint64) bool {
	return cut != 0 && (revision < cut || revision-(cut-1) <= surplus)
}

// readProjectionCandidate keeps the full element history only between the
// reservation and the first validation. Once sealed, only its live View is
// retained by the registry.
type ReadCandidate struct {
	Projection ReadView
	elements   elementfact.State
}

func (candidate *ReadCandidate) Validate() error {
	if candidate == nil {
		return contract.ErrStore
	}
	if err := ValidateMountSnapshotProjection(candidate.Projection); err != nil {
		return err
	}
	return candidate.elements.Validate(candidate.Projection.Scope, candidate.Projection.RevisionSerial, ElementExternalParentForView(candidate.Projection.Mount))
}

func (candidate *ReadCandidate) Seal() ReadView {
	projection := candidate.Projection
	projection.Elements = candidate.elements.View
	candidate.elements = elementfact.State{}
	return projection
}

func (projection ReadView) CurrentCut() contract.CoreReadCut {
	return token.FromParts(projection.Scope, projection.CutSerial)
}

func (owner *Owner) CaptureReadLocked() (*ReadCandidate, bool) {
	source := owner.state
	if source == nil || !source.elements.Initialized() {
		return nil, false
	}
	attachmentBuilder := facetfact.NewProjectionBuilder()
	for id, record := range source.attachments.facts.Records() {
		attachmentBuilder.Add(id, facetfact.Record{Attachment: record.Attachment})
	}
	mount := mountfact.New()
	mount.ReplaceIncoming(source.mount.facts.Incoming())
	mountOperations := make(map[contract.OperationID]MountOperation)
	for id, record := range source.mount.facts.Outgoings() {
		mount.ReplaceOutgoing(id, record)
		if record.Slot != "" {
			if op, ok := source.mount.operations[record.Slot]; ok {
				mountOperations[record.Slot] = op
			}
		}
	}
	mountAttempts := make(map[contract.MountAttemptKey]PeerAttempt)
	if key := source.mount.facts.Incoming().Preparing; key != (contract.MountAttemptKey{}) {
		if attempt, ok := source.mount.attempts[key]; ok {
			mountAttempts[key] = attempt
		}
	}
	projection := ReadView{
		edgeClosure:         source.currentEdgeClosure(),
		edgeInstallation:    source.currentEdgeInstallation(),
		carrierInstallation: source.currentCarrierInstallation(),
		carrierFence:        source.currentCarrierFence(),
		Authority:           source.authority, Scope: source.scope, CutSerial: source.cutSerial,
		RevisionSerial: source.revisionSerial, Elements: source.elements.View,
		extraRevisions: source.transferRevisionSurplus(),
		Attachments:    attachmentBuilder.Build().View, Mount: mount.View,
		Transfers:       owner.SourceVisibilityLocked(),
		mountOperations: mountOperations, mountAttempts: mountAttempts,
		mountInitialized:           source.mount.facts.Initialized(),
		mountOperationsInitialized: source.mount.operations != nil,
		mountAddressesInitialized:  source.mount.addresses != nil,
		mountAttemptsInitialized:   source.mount.attempts != nil,
	}
	valid := source.authority == owner.authority && source.scope != ([32]byte{}) &&
		projection.RevisionsValid() &&
		uint64(len(source.commits)) == source.cutSerial-1 && source.mount.facts.Initialized() &&
		source.mount.operations != nil && source.mount.addresses != nil && source.mount.attempts != nil
	return &ReadCandidate{Projection: projection, elements: source.elements}, valid
}

// Facts is a borrowed view of the immutable current facts. It never contains
// claims, tombstones, operation journals, mutable tables or the Store.
type Facts struct {
	Authority                 values.FactAuthorityRef
	Scope                     [32]byte
	CutSerial, RevisionSerial uint64
	Elements                  elementfact.View
	Mount                     mountfact.View
	Attachments               facetfact.View
}

func (facts Facts) CurrentCut() contract.CoreReadCut {
	return token.FromParts(facts.Scope, facts.CutSerial)
}
func (owner *Owner) FactsLocked() Facts {
	if owner.state == nil {
		return Facts{}
	}
	s := owner.state
	return Facts{Authority: s.authority, Scope: s.scope, CutSerial: s.cutSerial, RevisionSerial: s.revisionSerial, Elements: s.elements.View, Mount: s.mount.facts.View, Attachments: s.attachments.facts.View}
}
func (owner *Owner) SigningLocked() token.PublicSigner {
	if owner.state == nil {
		return token.PublicSigner{}
	}
	return token.NewPublicSigner(owner.state.publicKey, owner.authority)
}
func (owner *Owner) ValidateCurrentLocked() error { return validateCurrentSnapshot(owner.state) }
func (owner *Owner) AnchorProjectionLocked() elementfact.View {
	return owner.state.elements.AnchorsOnly().View
}
