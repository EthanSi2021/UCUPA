package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// InstallTransferTargetLocked completes an existing durable obligation, not a
// new user write. The operation layer must fetch the exact decision from its
// designated authenticated control owner; caller-supplied bytes are not enough.
// Exact installed retries recover locally even after later ordinary writes.
func (o *Owner) InstallTransferTargetLocked(d transferfact.SourceDecision) (transferfact.TargetInstallation, error) {
	if !d.Acceptance.Fence.Input.Valid() {
		return transferfact.TargetInstallation{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.TargetInstallation{}, err
	}
	if o.store == nil {
		return transferfact.TargetInstallation{}, values.PublicUnavailable
	}
	r, ok := o.state.transfers.targets[d.Acceptance.Fence.Key()]
	if !ok {
		return transferfact.TargetInstallation{}, contract.ErrNotFound
	}
	if r.installation.Revision != 0 {
		if r.installation.Decision != d {
			return transferfact.TargetInstallation{}, contract.ErrOperationConflict
		}
		return r.installation, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferTargetInstalled, At: next.currentCut(), Decision: d}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.TargetInstallation{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.TargetInstallation{}, err
	}
	return o.state.transfers.targets[d.Acceptance.Fence.Key()].installation, nil
}

func (s *coreState) applyTransferInstallation(e transferencoding.TransferEvent) error {
	d, a := e.Decision, e.Decision.Acceptance
	if e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || e.Manifest != "" ||
		a.Fence.Input.Target != s.authority || d.Revision == 0 || !token.Valid(d.At) || token.Scope(d.At) != token.Scope(a.Fence.FinalCut) || token.Serial(d.At) <= token.Serial(a.Fence.FinalCut) {
		return contract.ErrInvalid
	}
	r, ok := s.transfers.targets[a.Fence.Key()]
	if !ok || r.receipt != a || r.installation.Revision != 0 {
		return contract.ErrConflict
	}
	if err := s.validateTargetParent(a.Fence, r.parent); err != nil {
		return err
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	accepted := s.transfers.events[a.Revision-1]
	slice, err := transferencoding.DecodeTransferManifest(a.Fence, accepted.Manifest)
	if err != nil {
		return err
	}
	slice, err = transferencoding.AcceptedEdges(slice, accepted)
	if err != nil {
		return err
	}
	// Placement holds have excluded conflicting intervening changes. Recheck
	// with the same pure rule, without rejecting this transfer's own holds.
	returns, err := s.transferReturns(slice)
	if err != nil {
		return err
	}
	if _, err := transferfact.Place(slice, s.authority, s.currentCut(), s.elements, s.mount.facts.View, a.Fence.Input.Final, transferencoding.MaxTransferManifestRecords, returns); err != nil {
		return err
	}
	serial, err := transferfact.Install(slice, &s.elements, a.Fence.Input.Final, s.scope, s.revisionSerial, returns)
	if err != nil {
		return err
	}
	s.revisionSerial, s.cutSerial = serial, s.cutSerial+1
	for _, member := range slice.Members() {
		if err := s.attachments.facts.ElementTransferred(member.ID(), s.currentCut(), member.Facet); err != nil {
			return err
		}
	}
	index := uint64(len(s.transfers.events)) + 1
	r.installation = transferfact.TargetInstallation{Decision: d, At: s.currentCut(), Revision: index}
	if err := s.installTransferredDelegations(r, slice); err != nil {
		return err
	}
	s.transfers.targets[a.Fence.Key()] = r
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: index})
	return validateCurrentSnapshot(s)
}

// Each admitted target has a bounded immutable membership list. Only installed
// batches contribute surplus revisions above one per cut. This cheap projection
// never decodes a manifest or walks ordinary operation history during a read.
func (s *coreState) transferRevisionSurplus() uint64 {
	var n uint64
	for _, r := range s.transfers.targets {
		if r.installed() && len(r.members) > 1 {
			n += uint64(len(r.members) - 1)
		}
	}
	return n
}
