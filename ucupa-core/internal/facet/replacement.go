package facet

import (
	"context"
	"errors"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// BeginFacetReplacement reserves one element namespace replacement from the
// caller's exact public comparison. It grants the source no permission to
// fence and the target no content acceptance; every later step advances only
// through the fixed peer evidence retained by the commit journal.
func (core *Operations) BeginFacetReplacement(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, command corecontract.PublicFacetReplacementCommand) (corecontract.PublicFacetReplacementStatus, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
	}
	if ctx == nil || !caller.Subject.Valid() || !commitbinding.ValidOperationID(id) || !validReplacementCommand(command, core.owner.AuthorityRef()) {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicInvalid
	}
	if err := core.admission.Acquire(); err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	defer core.admission.Release()
	reqs := []values.Requirement{core.requirement(corecontract.PermissionFacetReplacementBegin, command.Element)}
	view, permit, err := core.admission.Authorize(ctx, caller, reqs)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
	}
	if err = core.admission.Validate(ctx, caller, view, reqs, false, permit); err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	if ctx.Err() != nil {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
	}
	input, err := core.resolveReplacementLocked(caller.Subject, id, command)
	if err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	c, err := core.owner.BeginFacetReplacementLocked(input)
	if err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	return core.replacementStatusLocked(c)
}

// InspectFacetReplacement is an exact original-caller lookup, never a task
// enumeration and never a disclosure of manifest bytes or private cuts.
func (core *Operations) InspectFacetReplacement(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, element values.ElementID) (corecontract.PublicFacetReplacementStatus, error) {
	_, status, err := core.readReplacement(ctx, caller, corecontract.PermissionFacetReplacementRead, id, element)
	return status, err
}

// CancelFacetReplacement is the caller's own bounded window: free before the
// permit is dispatched, and after the permit only once the source Facet's
// durable refusal has been recorded from its own evidence.
func (core *Operations) CancelFacetReplacement(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, element values.ElementID, expected values.PublicToken) (corecontract.PublicFacetReplacementStatus, error) {
	if core == nil || !expected.Valid() || expected.Kind != "facet-replacement-revision" || expected.Owner != core.owner.AuthorityRef() {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicInvalid
	}
	c, _, err := core.readReplacement(ctx, caller, corecontract.PermissionFacetReplacementCancel, id, element)
	if err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if ctx.Err() != nil || core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
	}
	if core.owner.SigningLocked().FacetReplacementRevision(c.At, string(id), element) != expected {
		return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrRevisionConflict
	}
	final, err := core.owner.CancelFacetReplacementLocked(caller.Subject, id, c.At)
	if err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	return core.replacementStatusLocked(final)
}

// RecoverFacetReplacement advances one exact replacement to its committed
// outcome. I/O never owns the commit lock: every peer call runs lock-free,
// and every local milestone revalidates the original caller view and compares
// the retained reservation cut before it commits. A failed call leaves its
// durable milestones recoverable by the same ID; callers never orchestrate,
// roll back or peek at intermediate peer steps.
func (core *Operations) RecoverFacetReplacement(ctx context.Context, caller values.CallerContext, id corecontract.OperationID, element values.ElementID, peers replacement.FacetPeers) (corecontract.PublicFacetReplacementStatus, error) {
	if core == nil || !core.admission.Enabled() {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
	}
	if ctx == nil || !caller.Subject.Valid() || !commitbinding.ValidOperationID(id) || !element.Valid() || peers.Source == nil || peers.Target == nil {
		return corecontract.PublicFacetReplacementStatus{}, values.PublicInvalid
	}
	if err := core.admission.Acquire(); err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	defer core.admission.Release()
	reqs := []values.Requirement{core.requirement(corecontract.PermissionFacetReplacementRecover, element)}
	view, permit, err := core.admission.Authorize(ctx, caller, reqs)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return corecontract.PublicFacetReplacementStatus{}, err
	}
	advance := func(apply func() error) error {
		core.owner.Lock()
		defer core.owner.Unlock()
		if ctx.Err() != nil || core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
			return values.PublicUnavailable
		}
		if err := core.admission.Validate(ctx, caller, view, reqs, false, permit); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return values.PublicUnavailable
		}
		return apply()
	}
	// Every cycle re-reads the durable control record and advances exactly one
	// step, so a concurrent recovery either idempotently repeats the step or
	// fails its own stale comparison without corrupting the milestone chain.
	for step := 0; step < 12; step++ {
		if ctx.Err() != nil {
			return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
		}
		c, status, err := core.readReplacement(ctx, caller, corecontract.PermissionFacetReplacementRecover, id, element)
		if err != nil {
			return corecontract.PublicFacetReplacementStatus{}, err
		}
		if c.SettledAt != (corecontract.CoreReadCut{}) || c.CancelledAt != (corecontract.CoreReadCut{}) {
			return status, nil
		}
		switch {
		case c.PermitAt == (corecontract.CoreReadCut{}):
			if err := advance(func() error {
				_, err := core.owner.PermitFacetReplacementLocked(caller.Subject, id, c.At)
				return err
			}); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, err
			}
		case c.FencedAt == (corecontract.CoreReadCut{}):
			state, err := peers.Source.CheckSourceExport(ctx, c.Input.SourceNextHop, c.Input.SourceKey, id)
			if err != nil {
				return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
			}
			if ctx.Err() != nil {
				return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
			}
			if state.Refused {
				// The source's durable refusal is discovered evidence, never a
				// Core command; recording it reopens only the caller's cancel.
				if err := advance(func() error {
					_, err := core.owner.RefuseFacetReplacementLocked(caller.Subject, id, c.At,
						transferfact.FacetReplacementRefusal{Source: c.Input.Source})
					return err
				}); err != nil {
					return corecontract.PublicFacetReplacementStatus{}, err
				}
				return core.replacementFinish(ctx, caller, corecontract.PermissionFacetReplacementRecover, id, element)
			}
			fence := transferfact.FacetReplacementFence{}
			manifest := ""
			if state.Fenced {
				fence, manifest, err = replacementEvidence(state.FinalCut, state.ManifestDigest, state.Manifest)
				if err != nil {
					return corecontract.PublicFacetReplacementStatus{}, err
				}
			} else {
				fenced, err := peers.Source.FenceSource(ctx, c.Input.SourceNextHop, c.Input.SourceKey, id, c.Input.Target)
				if err != nil {
					return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
				}
				if ctx.Err() != nil {
					return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
				}
				if fenced.Key != c.Input.SourceKey {
					return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrConflict
				}
				fence, manifest, err = replacementEvidence(fenced.FinalCut, fenced.ManifestDigest, fenced.Manifest)
				if err != nil {
					return corecontract.PublicFacetReplacementStatus{}, err
				}
			}
			if err := advance(func() error {
				_, _, err := core.owner.RecordFacetReplacementFenceLocked(caller.Subject, id, c.At, fence, manifest)
				return err
			}); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, err
			}
		case c.AcceptedAt == (corecontract.CoreReadCut{}):
			fence, manifest, err := core.replacementFence(caller.Subject, id)
			if err != nil {
				return corecontract.PublicFacetReplacementStatus{}, err
			}
			targetKey := values.FacetAcceptanceKey{Core: core.owner.AuthorityRef(), Facet: c.Input.Target, Element: element, Proposal: c.Proposal}
			state, err := peers.Target.CheckTargetImport(ctx, c.Input.TargetAddress, targetKey, id, []byte(manifest), c.Input.SourceAddress)
			if err != nil {
				return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
			}
			if ctx.Err() != nil {
				return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
			}
			if state.Digest != fence.ManifestDigest || state.FinalCut != fence.FinalCut {
				return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrConflict
			}
			if !state.Accepted {
				// The relay moves only the target's disclosed live key set,
				// entry by entry and payload-opaquely; staging progress stays
				// target-side and restarts cleanly from this same check.
				for _, live := range state.Live {
					entry, err := peers.Source.ReadSourceEntry(ctx, c.Input.SourceNextHop, c.Input.SourceKey, id, live.Type)
					if err != nil {
						return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
					}
					if entry.Type != live.Type || uint64(len(entry.Payload)) != live.Length {
						return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrConflict
					}
					if err := peers.Target.StageTargetEntry(ctx, c.Input.TargetAddress, targetKey, id, live.Type, entry.Payload); err != nil {
						return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
					}
					if ctx.Err() != nil {
						return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
					}
				}
				accepted, err := peers.Target.AcceptTarget(ctx, c.Input.TargetAddress, targetKey, id, c.Input.SourceAddress)
				if err != nil {
					return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
				}
				if accepted.Key != targetKey {
					return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrConflict
				}
			}
			if err := advance(func() error {
				_, err := core.owner.RecordFacetReplacementAcceptanceLocked(caller.Subject, id, c.At)
				return err
			}); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, err
			}
		case c.CommittedAt == (corecontract.CoreReadCut{}):
			// The one atomic relationship switch; no peer I/O guards it.
			if err := advance(func() error {
				_, err := core.owner.CommitFacetReplacementLocked(caller.Subject, id, c.At)
				return err
			}); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, err
			}
		case c.InstalledAt == (corecontract.CoreReadCut{}):
			targetKey := values.FacetAcceptanceKey{Core: core.owner.AuthorityRef(), Facet: c.Input.Target, Element: element, Proposal: c.Proposal}
			if err := peers.Target.InstallTarget(ctx, c.Input.TargetAddress, targetKey, id, c.Input.Source, c.Fence.FinalCut); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
			}
			if ctx.Err() != nil {
				return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
			}
			if err := advance(func() error {
				_, err := core.owner.RecordFacetReplacementInstallationLocked(caller.Subject, id, c.At)
				return err
			}); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, err
			}
		case c.SettledAt == (corecontract.CoreReadCut{}):
			if err := peers.Source.SettleSource(ctx, c.Input.SourceNextHop, c.Input.SourceKey, id, c.Input.Target); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrUnavailable
			}
			if ctx.Err() != nil {
				return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
			}
			if err := advance(func() error {
				_, err := core.owner.SettleFacetReplacementLocked(caller.Subject, id, c.At)
				return err
			}); err != nil {
				return corecontract.PublicFacetReplacementStatus{}, err
			}
		}
	}
	return corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
}

// ReadFacetReplacementRecord is the trusted local service port for the Facet
// side replacement read. The caller is independently established as the fixed
// Facet service by the composition, never by this value itself.
func (core *Operations) ReadFacetReplacementRecord(id corecontract.OperationID, element values.ElementID) (corecontract.FacetReplacementRecord, error) {
	if core == nil || !commitbinding.ValidOperationID(id) || !element.Valid() {
		return corecontract.FacetReplacementRecord{}, corecontract.ErrInvalid
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.FacetReplacementRecord{}, values.PublicUnavailable
	}
	c, err := core.owner.FacetReplacementControlLocked(id)
	if err != nil {
		return corecontract.FacetReplacementRecord{}, err
	}
	if c.Input.Element != element {
		return corecontract.FacetReplacementRecord{}, values.PublicNotFound
	}
	return corecontract.FacetReplacementRecord{
		Authority:      core.owner.AuthorityRef(),
		Element:        c.Input.Element,
		Operation:      c.Input.Operation,
		Source:         c.Input.Source,
		Target:         c.Input.Target,
		Proposal:       c.Proposal,
		FinalCut:       c.Fence.FinalCut,
		ManifestDigest: c.Fence.ManifestDigest,
		Permitted:      c.PermitAt != (corecontract.CoreReadCut{}),
		Fenced:         c.FencedAt != (corecontract.CoreReadCut{}),
		Accepted:       c.AcceptedAt != (corecontract.CoreReadCut{}),
		Committed:      c.CommittedAt != (corecontract.CoreReadCut{}),
		Settled:        c.SettledAt != (corecontract.CoreReadCut{}),
		Cancelled:      c.CancelledAt != (corecontract.CoreReadCut{}),
	}, nil
}

func replacementEvidence(finalCut uint64, digest [32]byte, manifest []byte) (transferfact.FacetReplacementFence, string, error) {
	if finalCut == 0 || digest == [32]byte{} || len(manifest) == 0 {
		return transferfact.FacetReplacementFence{}, "", corecontract.ErrConflict
	}
	return transferfact.FacetReplacementFence{FinalCut: finalCut, ManifestDigest: digest}, string(manifest), nil
}

func validReplacementCommand(c corecontract.PublicFacetReplacementCommand, owner values.FactAuthorityRef) bool {
	return c.Element.Valid() && c.Expected.Valid() && c.Expected.Owner == owner && c.Expected.Kind == "attachment-revision" &&
		c.Target.Valid() && c.TargetAddress.Valid() && c.SourceAddress.Valid()
}

// resolveReplacementLocked fixes the exact reservation input from the current
// relationship, or compares an existing reservation's original input on retry.
// No mutable fact other than the exact current relation is consulted.
func (core *Operations) resolveReplacementLocked(actor values.PrincipalRef, id corecontract.OperationID, command corecontract.PublicFacetReplacementCommand) (transferfact.FacetReplacementIntent, error) {
	if old, err := core.owner.FacetReplacementControlLocked(id); err == nil {
		if old.Input.Actor != actor || old.Input.Element != command.Element || old.Input.Target != command.Target ||
			old.Input.TargetAddress != command.TargetAddress || old.Input.SourceAddress != command.SourceAddress ||
			core.owner.SigningLocked().AttachmentRevision(old.Input.Expected) != command.Expected {
			return transferfact.FacetReplacementIntent{}, values.PublicConflict
		}
		return old.Input, nil
	} else if !errors.Is(err, corecontract.ErrNotFound) {
		return transferfact.FacetReplacementIntent{}, err
	}
	record, ok := core.owner.FactsLocked().Attachments.Record(command.Element)
	if !ok || record.Attachment.State != corecontract.FacetActive || record.Pending.Valid() ||
		record.Attachment.Key.Core != core.owner.AuthorityRef() {
		return transferfact.FacetReplacementIntent{}, values.PublicNotFound
	}
	if core.owner.SigningLocked().AttachmentRevision(record.Attachment.Revision) != command.Expected {
		return transferfact.FacetReplacementIntent{}, corecontract.ErrRevisionConflict
	}
	input := transferfact.FacetReplacementIntent{Operation: id, Actor: actor, Element: command.Element,
		Source: record.Attachment.Key.Facet, Target: command.Target,
		SourceKey: record.Attachment.Key, SourceNextHop: record.NextHop,
		SourceAddress: command.SourceAddress, TargetAddress: command.TargetAddress, Expected: record.Attachment.Revision}
	if !input.Valid() {
		return transferfact.FacetReplacementIntent{}, values.PublicInvalid
	}
	return input, nil
}

func (core *Operations) readReplacement(ctx context.Context, caller values.CallerContext, permission values.PermissionID, id corecontract.OperationID, element values.ElementID) (transferfact.FacetReplacementControl, corecontract.PublicFacetReplacementStatus, error) {
	if core == nil || !core.admission.Enabled() {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
	}
	if ctx == nil || !caller.Subject.Valid() || !commitbinding.ValidOperationID(id) || !element.Valid() {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, values.PublicInvalid
	}
	reqs := []values.Requirement{core.requirement(permission, element)}
	view, permit, err := core.admission.Authorize(ctx, caller, reqs)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if ctx.Err() != nil || core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, reqs, false, permit); err != nil {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, err
	}
	c, err := core.owner.FacetReplacementControlLocked(id)
	if err != nil {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, err
	}
	if c.Input.Actor != caller.Subject || c.Input.Element != element {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, values.PublicDenied
	}
	status, err := core.replacementStatusLocked(c)
	if err != nil {
		return transferfact.FacetReplacementControl{}, corecontract.PublicFacetReplacementStatus{}, err
	}
	return c, status, nil
}

func (core *Operations) replacementFinish(ctx context.Context, caller values.CallerContext, permission values.PermissionID, id corecontract.OperationID, element values.ElementID) (corecontract.PublicFacetReplacementStatus, error) {
	_, status, err := core.readReplacement(ctx, caller, permission, id, element)
	return status, err
}

// replacementFence returns the retained fence evidence and the exact journal
// manifest bytes; a fence-less record is a durable contradiction.
func (core *Operations) replacementFence(actor values.PrincipalRef, id corecontract.OperationID) (transferfact.FacetReplacementFence, string, error) {
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return transferfact.FacetReplacementFence{}, "", values.PublicUnavailable
	}
	return core.owner.FacetReplacementFenceLocked(actor, id)
}

func (core *Operations) replacementStatusLocked(c transferfact.FacetReplacementControl) (corecontract.PublicFacetReplacementStatus, error) {
	progress := corecontract.FacetReplacementReserved
	switch {
	case c.CancelledAt != (corecontract.CoreReadCut{}):
		progress = corecontract.FacetReplacementCancelled
	case c.SettledAt != (corecontract.CoreReadCut{}):
		progress = corecontract.FacetReplacementSettled
	case c.InstalledAt != (corecontract.CoreReadCut{}):
		progress = corecontract.FacetReplacementInstalled
	case c.CommittedAt != (corecontract.CoreReadCut{}):
		progress = corecontract.FacetReplacementCommitted
	case c.AcceptedAt != (corecontract.CoreReadCut{}):
		progress = corecontract.FacetReplacementAccepted
	case c.FencedAt != (corecontract.CoreReadCut{}):
		progress = corecontract.FacetReplacementFenced
	case c.PermitAt != (corecontract.CoreReadCut{}):
		progress = corecontract.FacetReplacementPermitted
	}
	revision := core.owner.SigningLocked().FacetReplacementRevision(c.At, string(c.Input.Operation), c.Input.Element)
	if !revision.Valid() {
		return corecontract.PublicFacetReplacementStatus{}, corecontract.ErrStore
	}
	return corecontract.PublicFacetReplacementStatus{
		Authority: core.owner.AuthorityRef(),
		Operation: c.Input.Operation,
		Element:   c.Input.Element,
		Source:    c.Input.Source,
		Target:    c.Input.Target,
		Progress:  progress,
		Refused:   c.Refusal != (transferfact.FacetReplacementRefusal{}),
		Revision:  revision,
	}, nil
}
