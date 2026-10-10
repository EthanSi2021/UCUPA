package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// InstallTransferredEdgeLocked is a carrier transaction, not a new transfer
// of its content. The operation layer must fetch the actual installation from
// the designated authenticated target. The retained original closure already
// binds the actor, target and promised range; client-supplied bytes do not.
func (o *Owner) InstallTransferredEdgeLocked(parent transferfact.CoreInstallation, mount values.ElementID) (transferfact.EdgeInstallation, error) {
	if !parent.Valid() || !mount.Valid() {
		return transferfact.EdgeInstallation{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.EdgeInstallation{}, err
	}
	if o.store == nil {
		return transferfact.EdgeInstallation{}, values.PublicUnavailable
	}
	key := parent.Fence(mount).Key()
	r, ok := o.state.transfers.edges[key]
	if !ok {
		return transferfact.EdgeInstallation{}, contract.ErrNotFound
	}
	if r.installation.Revision != 0 {
		if r.installation.Parent != parent {
			return transferfact.EdgeInstallation{}, contract.ErrOperationConflict
		}
		return r.installation, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferEdgeInstalled, At: next.currentCut(), EdgeParent: parent, EdgeMount: mount}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.EdgeInstallation{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.EdgeInstallation{}, err
	}
	return o.state.transfers.edges[key].installation, nil
}

func (s *coreState) applyEdgeInstallation(e transferencoding.TransferEvent) error {
	if !e.EdgeParent.Valid() || !e.EdgeMount.Valid() || e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || e.Manifest != "" {
		return contract.ErrInvalid
	}
	key := e.EdgeParent.Fence(e.EdgeMount).Key()
	r, exists := s.transfers.edges[key]
	if !exists || r.installation.Revision != 0 {
		return contract.ErrConflict
	}
	c := r.closure
	admission, err := e.EdgeParent.Delegation(c)
	if err != nil {
		return err
	}
	in := s.mount.facts.Incoming()
	if admission.MountKey.CarrierAuthority != s.authority || in.Gate != mountfact.GateReparenting || in.Admission != c.Previous || in.Key != c.Previous.Key.MountKey || in.Detached || in.Preparing != (contract.MountAttemptKey{}) {
		return contract.ErrConflict
	}
	inside, err := s.elements.RangeContainsDirectContent(e.EdgeMount, c.Range)
	if err != nil {
		return err
	}
	if !inside {
		return contract.ErrConflict
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	s.cutSerial++
	r.installation = transferfact.EdgeInstallation{Parent: e.EdgeParent, Closure: c, At: s.currentCut(), Revision: uint64(len(s.transfers.events)) + 1}
	if !r.installation.Valid() {
		return contract.ErrConflict
	}
	s.mount.facts.ReplaceIncoming(mountfact.Incoming{Key: admission.MountKey, AcceptedAt: c.At, Gate: mountfact.GateOpen,
		Admission: mountfact.Admission{Key: admission, RangeValue: c.Range, DecisionCut: e.EdgeParent.Cut()}, Contact: s.newContact(admission.MountKey, e.EdgeParent.Address(), true)})
	s.transfers.edges[key] = r
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: r.installation.Revision})
	return validateCurrentSnapshot(s)
}

func (s *coreState) currentEdgeInstallation() transferfact.EdgeInstallation {
	in := s.mount.facts.Incoming()
	for _, r := range s.transfers.edges {
		if r.installation.Revision == 0 {
			continue
		}
		key, err := r.installation.Parent.Delegation(r.closure)
		if err == nil && key.MountKey == in.Key {
			return r.installation
		}
	}
	return transferfact.EdgeInstallation{}
}
