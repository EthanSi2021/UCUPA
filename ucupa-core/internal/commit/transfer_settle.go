package commit

import (
	"math"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// SettleTransferSourceLocked completes an already decided local-slice transfer.
// The operation layer must obtain the installation from the exact authenticated
// target. A caller-supplied result, timeout or bare successor is not evidence.
// This private transaction performs no network I/O and creates no new authority.
func (o *Owner) SettleTransferSourceLocked(completion transferfact.CoreCompletion) (transferfact.SourceSettlement, error) {
	if !completion.Valid() || completion.Installed.Carrier != (transferfact.CarrierInstallation{}) {
		return transferfact.SourceSettlement{}, contract.ErrInvalid
	}
	installed := completion.Installed.Source
	i := installed.Decision.Acceptance.Fence.Input
	if !i.Valid() {
		return transferfact.SourceSettlement{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return transferfact.SourceSettlement{}, err
	}
	if o.store == nil {
		return transferfact.SourceSettlement{}, values.PublicUnavailable
	}
	r, ok := o.state.transfers.records[i.Operation]
	if !ok {
		return transferfact.SourceSettlement{}, contract.ErrNotFound
	}
	if r.Progress == transferfact.IntentSettled {
		e := o.state.transfers.events[r.Revision-1]
		if e.Installation != installed || e.Completion != completion {
			return transferfact.SourceSettlement{}, contract.ErrOperationConflict
		}
		return transferSettlement(e, r.Revision), nil
	}
	next := o.state.clone()
	e := transferencoding.TransferEvent{Kind: transferencoding.TransferSourceSettled, At: next.currentCut(), Installation: installed, Completion: completion}
	if err := next.applyTransferEvent(e); err != nil {
		return transferfact.SourceSettlement{}, err
	}
	if err := o.publishStateLocked(next); err != nil {
		return transferfact.SourceSettlement{}, err
	}
	return transferSettlement(e, o.state.transfers.records[i.Operation].Revision), nil
}

func transferSettlement(e transferencoding.TransferEvent, revision uint64) transferfact.SourceSettlement {
	return transferfact.SourceSettlement{Installation: e.Installation,
		At: token.FromParts(token.Scope(e.At), token.Serial(e.At)+1), Revision: revision}
}

func (s *coreState) applyTransferSettlement(e transferencoding.TransferEvent) error {
	if !e.Completion.Valid() || e.Completion.Installed != transferencoding.SettlementParent(e) {
		return contract.ErrInvalid
	}
	installed := e.Installation
	d, a := installed.Decision, installed.Decision.Acceptance
	i := a.Fence.Input
	if e.Input != (transferfact.Intent{}) || e.Operation != "" || e.Actor != (values.PrincipalRef{}) || e.Expected != 0 || e.Manifest != "" ||
		!i.Valid() || i.Source != s.authority || !token.Valid(installed.At) || token.Scope(installed.At) != token.Scope(a.At) ||
		token.Serial(installed.At) <= token.Serial(a.At) || installed.Revision <= a.Revision {
		return contract.ErrInvalid
	}
	r, ok := s.transfers.records[i.Operation]
	if !ok || r.Progress != transferfact.IntentCommitted {
		return contract.ErrConflict
	}
	decision, err := s.sourceDecision(r)
	if err != nil {
		return err
	}
	if decision != d {
		return contract.ErrOperationConflict
	}
	if s.cutSerial == math.MaxUint64 {
		return values.PublicResourceExhausted
	}
	// The first profile has no external adjacency to repair. Re-prove every
	// member's exact departure from the original fence before publishing the
	// completed source coverage. Later carrier/root profiles add their own edges.
	f, raw, err := s.sourceFence(r)
	if err != nil {
		return err
	}
	slice, err := transferencoding.DecodeTransferManifest(f, raw)
	if err != nil {
		return err
	}
	if slice.HasMounts() && e.Completion.At == installed.At {
		return contract.ErrMountCoordinationRequired
	}
	for _, member := range slice.Members() {
		departure, ok := s.elements.Departure(member.ID())
		if !ok || departure.Controller != i.Key().Controller || departure.Operation != i.Operation || s.elements.Kind(member.ID()) != 0 {
			return contract.ErrStore
		}
		want := member.Entity.Revision
		if member.Kind == contract.ElementAnchor {
			want = member.Anchor.Revision
		}
		if departure.Revision != want || departure.Root != (member.Kind == contract.ElementAnchor && member.Anchor.Root()) {
			return contract.ErrStore
		}
	}
	if i.Profile == transferfact.RootSuccessor {
		if root, ok := s.elements.RetiredRoot(); !ok || root != i.Boundary || r.Origin != (contract.SpatialTuple{}) {
			return contract.ErrStore
		}
	} else if _, ok := s.elements.Anchor(r.Origin.Parent); !ok && r.Origin.Parent != slice.ExternalParent().ID {
		return contract.ErrStore
	}
	r.Progress, r.Revision = transferfact.IntentSettled, uint64(len(s.transfers.events))+1
	s.transfers.records[i.Operation] = r
	// Coverage is query-visible state. Publish a fresh cut, rather than silently
	// clearing a cached gap at the same cut or issuing a fictitious Element write.
	s.cutSerial++
	s.commits = append(s.commits, coreCommit{kind: transferCommitted, controlIndex: r.Revision})
	return validateCurrentSnapshot(s)
}
