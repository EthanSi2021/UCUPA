package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The retained manifest exists once in the journal. This bounded, derived
// reservation index is rebuilt through the same transition on Store reopen.
type acceptedTransfer struct {
	parent              transferfact.ParentHold
	receipt             transferfact.TargetAcceptance
	carrier             transferfact.CarrierAcceptance
	carrierInstallation transferfact.CarrierInstallation
	members             []values.ElementID
	closures            []transferfact.EdgeClosure
	installation        transferfact.TargetInstallation
	completion          transferfact.CoreCompletion
}

func (r acceptedTransfer) reservesWholeSpace() bool {
	return !r.complete() && (r.receipt.Fence.Input.Profile == transferfact.RootSuccessor || r.carrier.Revision != 0)
}

func (r acceptedTransfer) installed() bool {
	return r.installation.Revision != 0 || r.carrierInstallation.Revision != 0
}

func (r acceptedTransfer) complete() bool {
	return r.installed() && (len(r.closures) == 0 || r.completion.Revision != 0)
}

func (r acceptedTransfer) installedProof() transferfact.CoreInstallation {
	return transferfact.CoreInstallation{Source: r.installation, Carrier: r.carrierInstallation}
}

// AcceptTransferTargetLocked is a private construction-stage transaction.
// Its future caller must authenticate the source fence and every carrier hold,
// and authorize the original actor for this exact target placement.
// No network call or credential verification occurs under the owner lock.
func (o *Owner) AcceptTransferTargetLocked(actor values.PrincipalRef, f transferfact.SourceFence, manifest string, closures []transferfact.EdgeClosure, parent transferfact.ParentHold) (transferfact.TargetAcceptance, error) {
	if !actor.Valid() || !f.Input.Valid() {
		return transferfact.TargetAcceptance{}, contract.ErrInvalid
	}
	if actor != f.Input.Actor {
		return transferfact.TargetAcceptance{}, values.PublicDenied
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.TargetAcceptance{}, err
	}
	if o.store == nil {
		return transferfact.TargetAcceptance{}, values.PublicUnavailable
	}
	if f.Input.Target != o.authority {
		return transferfact.TargetAcceptance{}, contract.ErrConflict
	}
	closed, err := transferencoding.EncodeEdgeClosures(transferfact.EdgeFence{Source: f, Mount: f.Input.Boundary}, closures)
	if err != nil {
		return transferfact.TargetAcceptance{}, err
	}
	upstream, err := transferencoding.EncodeTargetParentHold(parent)
	if err != nil {
		return transferfact.TargetAcceptance{}, err
	}
	if old, ok := o.state.transfers.targets[f.Key()]; ok {
		if old.carrier.Revision != 0 {
			return transferfact.TargetAcceptance{}, contract.ErrOperationConflict
		}
		e := o.state.transfers.events[old.receipt.Revision-1]
		if old.receipt.Fence != f || e.Manifest != manifest || e.Closures != closed || e.TargetParent != upstream {
			return transferfact.TargetAcceptance{}, contract.ErrOperationConflict
		}
		return old.receipt, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferTargetAccepted, At: next.currentCut(), Fence: f, Manifest: manifest, Closures: closed, TargetParent: upstream}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.TargetAcceptance{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.TargetAcceptance{}, err
	}
	return o.state.transfers.targets[f.Key()].receipt, nil
}

func (s *coreState) applyTransferAcceptance(e transferencoding.TransferEvent) error {
	f := e.Fence
	if e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || f.Input.Target != s.authority {
		return contract.ErrInvalid
	}
	if _, exists := s.transfers.targets[f.Key()]; exists {
		return contract.ErrOperationConflict
	}
	slice, err := transferencoding.DecodeTransferManifest(f, e.Manifest)
	if err != nil {
		return err
	}
	slice, err = transferencoding.AcceptedEdges(slice, e)
	if err != nil {
		return err
	}
	parent, err := transferencoding.DecodeTargetParentHold(e.TargetParent)
	if err != nil {
		return err
	}
	if err := s.validateTargetParent(f, parent); err != nil {
		return err
	}
	if parent != (transferfact.ParentHold{}) && s.mount.facts.Incoming().Contact.NextHop != parent.Input.ParentAddress {
		return contract.ErrMountCoordinationRequired
	}
	if err := s.checkTransferTargetCapacity(e.Manifest, e.Closures, e.TargetParent, len(slice.ClosedEdges())); err != nil {
		return err
	}
	if err := s.validateReturningFacetProposals(slice); err != nil {
		return err
	}
	if _, err := s.planTransferTarget(slice, f.Input.Final, transferencoding.MaxTransferManifestRecords); err != nil {
		return err
	}
	members := slice.Members()
	ids := make([]values.ElementID, len(members))
	for index, member := range members {
		ids[index] = member.ID()
	}
	r := transferfact.TargetAcceptance{Fence: f, At: e.At, Revision: uint64(len(s.transfers.events)) + 1}
	s.transfers.targets[f.Key()] = acceptedTransfer{receipt: r, members: ids, closures: slice.ClosedEdges(), parent: parent}
	return nil
}

func (s *coreState) checkTransferTargetCapacity(manifest, closures, parent string, edges int) error {
	if len(s.transfers.targets) >= transferencoding.MaxTransferTargets {
		return values.PublicResourceExhausted
	}
	return s.checkTransferRetainedBytes(len(manifest) + len(closures) + len(parent) + edges*transferencoding.MaxEdgeResultBytes)
}

func (s *coreState) checkTransferRetainedCapacity(manifest, closures string) error {
	return s.checkTransferRetainedBytes(len(manifest) + len(closures))
}

func (s *coreState) checkTransferRetainedBytes(retained int) error {
	for _, old := range s.transfers.events {
		retained += len(old.Manifest) + len(old.Closures) + len(old.EdgeResults) + len(old.TargetParent)
	}
	for _, r := range s.transfers.targets {
		if !r.complete() {
			retained += len(r.closures) * transferencoding.MaxEdgeResultBytes
		}
	}
	if retained > transferencoding.MaxTransferRetainedBytes {
		return values.PublicResourceExhausted
	}
	return nil
}
