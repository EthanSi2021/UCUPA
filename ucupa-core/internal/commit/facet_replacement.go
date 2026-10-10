package commit

import (
	"math"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// The authenticated operation must authorize the original caller and, for
// peer milestones, verify the peer's own durable evidence before entering
// these private transactions. Milestones never authorize anything by themselves.

// BeginFacetReplacementLocked records the exact caller input and a fresh
// proposal for one element namespace replacement. It is not permission for
// the source to fence and not the target's content acceptance.
func (o *Owner) BeginFacetReplacementLocked(input transferfact.FacetReplacementIntent) (transferfact.FacetReplacementControl, error) {
	if _, err := transferencoding.EncodeFacetReplacementIntent(input); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	if o.store == nil {
		return transferfact.FacetReplacementControl{}, values.PublicUnavailable
	}
	if old, ok := o.state.transfers.replacements[input.Operation]; ok {
		if old.Input != input {
			return transferfact.FacetReplacementControl{}, contract.ErrOperationConflict
		}
		return old, nil
	}
	next := o.state.clone()
	if err := next.applyTransferEvent(transferencoding.TransferEvent{Kind: transferencoding.TransferFacetReserved, At: next.currentCut(), Facet: input}); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	return o.state.transfers.replacements[input.Operation], nil
}

// FacetReplacementControlLocked returns one value, never a table and never an
// authorization. Disclosure belongs to the calling operation.
func (o *Owner) FacetReplacementControlLocked(id contract.OperationID) (transferfact.FacetReplacementControl, error) {
	if len(id) == 0 || len(id) > 128 {
		return transferfact.FacetReplacementControl{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	c, ok := o.state.transfers.replacements[id]
	if !ok {
		return transferfact.FacetReplacementControl{}, contract.ErrNotFound
	}
	return c, nil
}

func (o *Owner) facetReplacementLocked(actor values.PrincipalRef, id contract.OperationID) (transferfact.FacetReplacementControl, error) {
	if !actor.Valid() || len(id) == 0 || len(id) > 128 {
		return transferfact.FacetReplacementControl{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	c, ok := o.state.transfers.replacements[id]
	if !ok {
		return transferfact.FacetReplacementControl{}, contract.ErrNotFound
	}
	if c.Input.Actor != actor {
		return transferfact.FacetReplacementControl{}, values.PublicDenied
	}
	return c, nil
}

// PermitFacetReplacementLocked closes the controller's free cancellation
// window. From this milestone on, only the source Facet's durable refusal can
// still end the replacement without its fence.
func (o *Owner) PermitFacetReplacementLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.FacetReplacementControl, error) {
	return o.advanceFacetReplacementLocked(transferencoding.TransferFacetPermitted, actor, id, expected,
		func(c transferfact.FacetReplacementControl) bool { return c.PermitAt != (contract.CoreReadCut{}) })
}

// RecordFacetReplacementAcceptanceLocked records the target Facet's durable
// acceptance of the complete fenced namespace. It does not itself install or
// verify any content.
func (o *Owner) RecordFacetReplacementAcceptanceLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.FacetReplacementControl, error) {
	return o.advanceFacetReplacementLocked(transferencoding.TransferFacetAccepted, actor, id, expected,
		func(c transferfact.FacetReplacementControl) bool { return c.AcceptedAt != (contract.CoreReadCut{}) })
}

// CommitFacetReplacementLocked performs the one atomic relationship switch.
// The already accepted target key becomes current exactly when the element's
// attachment still holds the original expected revision; the former key is
// then never current again.
func (o *Owner) CommitFacetReplacementLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.FacetReplacementControl, error) {
	return o.advanceFacetReplacementLocked(transferencoding.TransferFacetCommitted, actor, id, expected,
		func(c transferfact.FacetReplacementControl) bool { return c.CommittedAt != (contract.CoreReadCut{}) })
}

// RecordFacetReplacementInstallationLocked records the target Facet's durable
// installation of the accepted namespace at its own authority.
func (o *Owner) RecordFacetReplacementInstallationLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.FacetReplacementControl, error) {
	return o.advanceFacetReplacementLocked(transferencoding.TransferFacetInstalled, actor, id, expected,
		func(c transferfact.FacetReplacementControl) bool { return c.InstalledAt != (contract.CoreReadCut{}) })
}

// SettleFacetReplacementLocked records the source Facet's durable settlement
// and frees the element for further control. The journal retains everything.
func (o *Owner) SettleFacetReplacementLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.FacetReplacementControl, error) {
	return o.advanceFacetReplacementLocked(transferencoding.TransferFacetSettled, actor, id, expected,
		func(c transferfact.FacetReplacementControl) bool { return c.SettledAt != (contract.CoreReadCut{}) })
}

// CancelFacetReplacementLocked is valid before the permit without any source
// promise, or after the permit only once the source Facet's durable refusal
// has been recorded. Network silence is never that refusal.
func (o *Owner) CancelFacetReplacementLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut) (transferfact.FacetReplacementControl, error) {
	return o.advanceFacetReplacementLocked(transferencoding.TransferFacetCancelled, actor, id, expected,
		func(c transferfact.FacetReplacementControl) bool { return c.CancelledAt != (contract.CoreReadCut{}) })
}

func (o *Owner) advanceFacetReplacementLocked(kind transferencoding.TransferEventKind, actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut, repeat func(transferfact.FacetReplacementControl) bool) (transferfact.FacetReplacementControl, error) {
	c, err := o.facetReplacementLocked(actor, id)
	if err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	if o.store == nil {
		return transferfact.FacetReplacementControl{}, values.PublicUnavailable
	}
	if expected != c.At {
		return transferfact.FacetReplacementControl{}, contract.ErrRevisionConflict
	}
	if repeat(c) {
		return c, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: kind, At: next.currentCut(), Operation: id, Actor: actor}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	return o.state.transfers.replacements[id], nil
}

// RecordFacetReplacementFenceLocked retains the source Facet's durable final
// cut, the exact manifest digest and the full immutable manifest bytes. The
// controller passes the bytes it obtained from the authenticated source peer;
// it cannot rederive the Facet's digest and never parses them.
func (o *Owner) RecordFacetReplacementFenceLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut, fence transferfact.FacetReplacementFence, manifest string) (transferfact.FacetReplacementFence, string, error) {
	c, err := o.facetReplacementLocked(actor, id)
	if err != nil {
		return transferfact.FacetReplacementFence{}, "", err
	}
	if o.store == nil {
		return transferfact.FacetReplacementFence{}, "", values.PublicUnavailable
	}
	if expected != c.At {
		return transferfact.FacetReplacementFence{}, "", contract.ErrRevisionConflict
	}
	if c.FencedAt != (contract.CoreReadCut{}) {
		if c.Fence != fence {
			return transferfact.FacetReplacementFence{}, "", contract.ErrOperationConflict
		}
		// The retained journal bytes stay authoritative for every later retry.
		return o.state.facetReplacementFence(c)
	}
	if !fence.Valid() || len(manifest) > transferencoding.MaxTransferManifestBytes {
		return transferfact.FacetReplacementFence{}, "", contract.ErrInvalid
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferFacetFenced, At: next.currentCut(), Operation: id, Actor: actor, FacetFence: fence, Manifest: manifest}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.FacetReplacementFence{}, "", err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.FacetReplacementFence{}, "", err
	}
	return o.state.facetReplacementFence(o.state.transfers.replacements[id])
}

// FacetReplacementFenceLocked returns the retained fence evidence and the
// exact manifest bytes for the designated downstream peer steps.
func (o *Owner) FacetReplacementFenceLocked(actor values.PrincipalRef, id contract.OperationID) (transferfact.FacetReplacementFence, string, error) {
	c, err := o.facetReplacementLocked(actor, id)
	if err != nil {
		return transferfact.FacetReplacementFence{}, "", err
	}
	return o.state.facetReplacementFence(c)
}

// RefuseFacetReplacementLocked records the source Facet's durable refusal of
// this exact permitted operation. The peer read must have established it;
// these bytes alone are not a Facet authentication.
func (o *Owner) RefuseFacetReplacementLocked(actor values.PrincipalRef, id contract.OperationID, expected contract.CoreReadCut, refusal transferfact.FacetReplacementRefusal) (transferfact.FacetReplacementControl, error) {
	c, err := o.facetReplacementLocked(actor, id)
	if err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	if o.store == nil {
		return transferfact.FacetReplacementControl{}, values.PublicUnavailable
	}
	if expected != c.At {
		return transferfact.FacetReplacementControl{}, contract.ErrRevisionConflict
	}
	if c.Refusal != (transferfact.FacetReplacementRefusal{}) {
		if c.Refusal != refusal {
			return transferfact.FacetReplacementControl{}, contract.ErrOperationConflict
		}
		return c, nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferFacetRefused, At: next.currentCut(), Operation: id, Actor: actor, FacetRefusal: refusal}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.FacetReplacementControl{}, err
	}
	return o.state.transfers.replacements[id], nil
}

func (s *coreState) facetReplacementFence(c transferfact.FacetReplacementControl) (transferfact.FacetReplacementFence, string, error) {
	if c.FencedAt == (contract.CoreReadCut{}) || c.FenceRevision == 0 || c.FenceRevision > uint64(len(s.transfers.events)) {
		return transferfact.FacetReplacementFence{}, "", contract.ErrStore
	}
	e := s.transfers.events[c.FenceRevision-1]
	// The journal event admits at the last pre-increment cut; the milestone
	// then consumes exactly one further cut of the same scope.
	if e.Kind != transferencoding.TransferFacetFenced || e.Operation != c.Input.Operation || e.FacetFence != c.Fence ||
		token.Scope(e.At) != token.Scope(c.FencedAt) || token.Serial(e.At)+1 != token.Serial(c.FencedAt) {
		return transferfact.FacetReplacementFence{}, "", contract.ErrStore
	}
	return c.Fence, e.Manifest, nil
}

// applyFacetReplacement is shared by live milestones and the original Core
// replay. Every milestone binds the reservation's actor and commits its own
// cut; the fact handler derives all further preconditions from the retained
// control record, never from the caller.
func (s *coreState) applyFacetReplacement(e transferencoding.TransferEvent) error {
	if e.Input != (transferfact.Intent{}) || e.Expected != 0 || len(e.Operation) == 0 && e.Kind != transferencoding.TransferFacetReserved {
		return contract.ErrInvalid
	}
	if e.Kind != transferencoding.TransferFacetFenced && e.Manifest != "" {
		return contract.ErrInvalid
	}
	if e.Kind == transferencoding.TransferFacetReserved {
		return s.reserveFacetReplacement(e)
	}
	c, ok := s.transfers.replacements[e.Operation]
	if !ok || !c.Valid() || !e.Actor.Valid() || e.Actor != c.Input.Actor || s.cutSerial == math.MaxUint64 {
		return contract.ErrConflict
	}
	switch e.Kind {
	case transferencoding.TransferFacetPermitted:
		if c.PermitAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
			return contract.ErrConflict
		}
		s.cutSerial++
		c.PermitAt = s.currentCut()
	case transferencoding.TransferFacetRefused:
		if c.PermitAt == (contract.CoreReadCut{}) || c.FencedAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) || c.Refusal != (transferfact.FacetReplacementRefusal{}) || !e.FacetRefusal.Matches(c) {
			return contract.ErrConflict
		}
		s.cutSerial++
		c.Refusal = e.FacetRefusal
	case transferencoding.TransferFacetFenced:
		if c.PermitAt == (contract.CoreReadCut{}) || c.FencedAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) || c.Refusal != (transferfact.FacetReplacementRefusal{}) || !e.FacetFence.Valid() || e.Manifest == "" {
			return contract.ErrConflict
		}
		if err := s.checkTransferRetainedCapacity(e.Manifest, ""); err != nil {
			return err
		}
		s.cutSerial++
		c.Fence, c.FenceRevision = e.FacetFence, uint64(len(s.transfers.events))+1
		c.FencedAt = s.currentCut()
	case transferencoding.TransferFacetAccepted:
		if c.FencedAt == (contract.CoreReadCut{}) || c.AcceptedAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
			return contract.ErrConflict
		}
		s.cutSerial++
		c.AcceptedAt = s.currentCut()
	case transferencoding.TransferFacetCommitted:
		if c.FencedAt == (contract.CoreReadCut{}) || c.AcceptedAt == (contract.CoreReadCut{}) || c.CommittedAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
			return contract.ErrConflict
		}
		record, exists := s.attachments.facts.Record(c.Input.Element)
		if !exists || record.Pending.Valid() || record.Attachment.State != contract.FacetActive ||
			record.Attachment.Key.Core != s.authority || record.Attachment.Key.Facet != c.Input.Source || record.Attachment.Revision != c.Input.Expected {
			return contract.ErrStore
		}
		s.cutSerial++
		key := values.FacetAcceptanceKey{Core: s.authority, Facet: c.Input.Target, Element: c.Input.Element, Proposal: c.Proposal}
		s.attachments.facts.TransferNamespace(c.Input.Element, key, s.currentCut(), c.Input.TargetAddress)
		c.CommittedAt = s.currentCut()
	case transferencoding.TransferFacetInstalled:
		if c.CommittedAt == (contract.CoreReadCut{}) || c.InstalledAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
			return contract.ErrConflict
		}
		s.cutSerial++
		c.InstalledAt = s.currentCut()
	case transferencoding.TransferFacetSettled:
		if c.InstalledAt == (contract.CoreReadCut{}) || c.SettledAt != (contract.CoreReadCut{}) || c.CancelledAt != (contract.CoreReadCut{}) {
			return contract.ErrConflict
		}
		s.cutSerial++
		c.SettledAt = s.currentCut()
	case transferencoding.TransferFacetCancelled:
		if c.CancelledAt != (contract.CoreReadCut{}) || c.SettledAt != (contract.CoreReadCut{}) {
			return contract.ErrConflict
		}
		// After the fence the replacement can only run to its committed
		// outcome; a dispatched permit additionally requires the source's
		// durable refusal, never inferred from network silence.
		if c.FencedAt != (contract.CoreReadCut{}) || c.PermitAt != (contract.CoreReadCut{}) && !c.Refusal.Matches(c) {
			return values.PublicUnavailable
		}
		s.cutSerial++
		c.CancelledAt = s.currentCut()
	default:
		return contract.ErrInvalid
	}
	if !c.Valid() {
		return contract.ErrStore
	}
	s.transfers.replacements[e.Operation] = c
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: uint64(len(s.transfers.events)) + 1})
	return validateCurrentSnapshot(s)
}

func (s *coreState) reserveFacetReplacement(e transferencoding.TransferEvent) error {
	i := e.Facet
	if !i.Valid() || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Manifest != "" {
		return contract.ErrInvalid
	}
	if _, err := transferencoding.EncodeFacetReplacementIntent(i); err != nil {
		return err
	}
	if s.transferIntentCount() >= transferencoding.MaxTransferIntents || s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	if _, ok := s.operations[i.Operation]; ok || s.mountOperationClaimed(i.Operation) {
		return contract.ErrOperationConflict
	}
	if err := s.guardTransferElement(i.Element); err != nil {
		return err
	}
	record, ok := s.attachments.facts.Record(i.Element)
	if !ok || record.Attachment.State != contract.FacetActive || record.Pending.Valid() ||
		record.Attachment.Key.Core != s.authority || record.Attachment.Key.Facet != i.Source {
		return contract.ErrConflict
	}
	// The retained source snapshot must be the exact current relation; a later
	// switch ends that relation, so peer steps after the commit switch can
	// only recover from this retained fact, never from the live record.
	if record.Attachment.Key != i.SourceKey || record.NextHop != i.SourceNextHop {
		return contract.ErrConflict
	}
	if record.Attachment.Revision != i.Expected {
		return contract.ErrRevisionConflict
	}
	s.cutSerial++
	c := transferfact.FacetReplacementControl{Input: i, At: s.currentCut(), Proposal: facetfact.AttachmentRevisionForCut(i.Element, s.currentCut())}
	if !c.Valid() {
		return contract.ErrStore
	}
	s.transfers.replacements[i.Operation] = c
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: uint64(len(s.transfers.events)) + 1})
	return validateCurrentSnapshot(s)
}
