package connected

import (
	"context"
	"errors"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	coretoken "ucupa/ucupa-core/internal/token"
)

func (core *Operations) beginConnected(ctx context.Context, callers corecontract.LocalCallers, intent exchange.Intent, at *corecontract.ConnectedCut) (*connectedOpening, bool, error) {
	if core == nil || ctx == nil {
		return nil, false, corecontract.ErrInvalid
	}
	if _, valid := connectedCallerBinding(callers); !valid {
		return nil, false, corecontract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if !callers[core.authority].Valid() {
		return nil, false, corecontract.ErrDenied
	}
	if at == nil {
		opening, err := core.openConnected(ctx, callers, intent)
		if opening != nil {
			opening.intent = intent
		}
		return opening, true, err
	}
	opening, err := core.resumeConnected(ctx, callers, *at, intent)
	if opening != nil {
		opening.intent = intent
	}
	return opening, false, err
}

func (core *Operations) completeConnected(opening *connectedOpening, initial, success bool) {
	if initial {
		if success {
			opening.cleanup.PreservePins()
		} else {
			core.retireConnected(opening.context)
		}
	}
	opening.cleanup.Close()
	core.finishConnectedContext(opening.context)
}

func (core *Operations) prepareConnectedOperation(ctx context.Context, opening *connectedOpening, intent exchange.Intent, initial bool) (*connectedWorld, connectedTrace, values.ElementID, uint64, corecontract.Path, error) {
	terminal, err := core.connectedTerminal(opening.context, false)
	if err != nil {
		return nil, connectedTrace{}, "", 0, corecontract.Path{}, err
	}
	world, err := connectedWorldFromOpening(opening, terminal)
	if err != nil {
		return nil, connectedTrace{}, "", 0, corecontract.Path{}, err
	}
	trace, scope, depth, scopePath, decision := connectedWorldScope(world, intent)
	if err := core.authorizeConnectedOperation(ctx, opening, world, intent, trace, scope); err != nil {
		return nil, connectedTrace{}, "", 0, corecontract.Path{}, err
	}
	if decision != nil {
		return nil, trace, "", 0, corecontract.Path{}, decision
	}
	if initial {
		if scope != opening.context.scope || depth != opening.context.depth ||
			(!trace.absent && !queryrequest.EqualPath(scopePath, opening.context.scopePath)) {
			return nil, trace, "", 0, corecontract.Path{}, corecontract.ErrReadCutExpired
		}
	} else {
		if intent.Path.OriginAnchorID != opening.context.intent.Path.OriginAnchorID {
			return nil, trace, "", 0, corecontract.Path{}, corecontract.ErrInvalid
		}
		if err := connectedAllowed(opening.context, world, trace, scope, intent); err != nil {
			return nil, trace, "", 0, corecontract.Path{}, err
		}
	}
	if trace.absent && world.missing[trace.parent] {
		// A missing source cannot prove a missing child. No new source is
		// contacted merely because an at-read needs an excluded adjacency.
		return nil, trace, "", 0, corecontract.Path{}, corecontract.ErrUnavailable
	}
	return world, trace, scope, depth, scopePath, nil
}

// ResolvePath follows a proven path at the original entry. Exact read grants
// remain independent of the query that originally exposed a match summary.
func (core *Operations) ResolvePath(ctx context.Context, callers corecontract.LocalCallers, path corecontract.Path, at *corecontract.ConnectedCut) (corecontract.PathRead, error) {
	intent, valid := canonicalConnectedIntent(exchange.Intent{Kind: exchange.Resolve, Path: path})
	if !valid {
		return corecontract.PathRead{}, corecontract.ErrInvalid
	}
	opening, initial, err := core.beginConnected(ctx, callers, intent, at)
	if err != nil {
		return corecontract.PathRead{}, err
	}
	success := false
	defer func() { core.completeConnected(opening, initial, success) }()
	world, trace, _, _, normalized, decision := core.prepareConnectedOperation(ctx, opening, intent, initial)
	var result corecontract.PathRead
	if decision == nil {
		result = corecontract.PathRead{Cut: opening.context.cut, ReadMode: opening.context.mode}
		if trace.absent {
			result.State = corecontract.ExistenceNotFound
			prefixTarget := trace.parent
			if prefixTarget == "" {
				result.VerifiedPrefix = corecontract.Path{OriginAnchorID: intent.Path.OriginAnchorID}
				result.AbsenceSource = corecontract.ConnectedSource{Authority: core.authority, Cut: opening.context.sources[0].cut}
			} else {
				result.VerifiedPrefix, decision = connectedShortestPath(world.elements, intent.Path.OriginAnchorID, prefixTarget)
				result.AbsenceSource = world.sources[prefixTarget]
				if prefixTarget == opening.context.key.MountAnchorUUID {
					for _, source := range opening.context.sources {
						if source.authority == opening.context.key.CarrierAuthority {
							result.AbsenceSource = corecontract.ConnectedSource{Authority: source.authority, Cut: source.cut}
						}
					}
				}
			}
		} else {
			record, ok := connectedWorldRecord(world, trace.target)
			if !ok {
				decision = corecontract.ErrUnavailable
			} else {
				result.State, result.Record, result.Source, result.Path = corecontract.ExistencePresent, &record.Record, record.Source, normalized
				attachment, ok := world.attachments.Record(trace.target)
				if !ok || !attachment.Attachment.Revision.Valid() {
					decision = corecontract.ErrUnavailable
				} else {
					result.Attachment = attachment.Attachment
				}
			}
		}
	}
	terminal, err := core.finalizeConnected(ctx, opening, initial)
	if err != nil {
		return corecontract.PathRead{}, err
	}
	if terminal && (result.Source.Authority == opening.context.key.CarrierAuthority || result.AbsenceSource.Authority == opening.context.key.CarrierAuthority) {
		return corecontract.PathRead{}, corecontract.ErrUnavailable
	}
	if decision != nil {
		return corecontract.PathRead{}, decision
	}
	success = true
	return result, nil
}

func connectedWorldRecord(world *connectedWorld, id values.ElementID) (corecontract.ConnectedRecord, bool) {
	source, ok := world.sources[id]
	if !ok {
		return corecontract.ConnectedRecord{}, false
	}
	if anchor, exists := world.elements.Anchor(id); exists {
		return corecontract.ConnectedRecord{Record: anchor.Record(), Source: source}, true
	}
	if entity, exists := world.elements.Entity(id); exists {
		return corecontract.ConnectedRecord{Record: entity.Record(), Source: source}, true
	}
	return corecontract.ConnectedRecord{}, false
}

func (core *Operations) ListDirectPath(ctx context.Context, callers corecontract.LocalCallers, path corecontract.Path, at *corecontract.ConnectedCut, limit uint16, continuation *corecontract.ConnectedDirectContinuation) (corecontract.ConnectedDirectPage, error) {
	intent, valid := canonicalConnectedIntent(exchange.Intent{Kind: exchange.List, Path: path})
	if !valid || limit == 0 || limit > 256 || (continuation != nil && (at != nil || !coretoken.ValidConnectedCut(queryrequest.ConnectedDirectCut(*(continuation))) || !queryrequest.ConnectedDirectLast(*(continuation)).Valid())) {
		return corecontract.ConnectedDirectPage{}, corecontract.ErrInvalid
	}
	if continuation != nil {
		cut := queryrequest.ConnectedDirectCut(*continuation)
		at = &cut
	}
	opening, initial, err := core.beginConnected(ctx, callers, intent, at)
	if err != nil {
		return corecontract.ConnectedDirectPage{}, err
	}
	success := false
	defer func() { core.completeConnected(opening, initial, success) }()
	world, trace, _, _, normalized, decision := core.prepareConnectedOperation(ctx, opening, intent, initial)
	var page corecontract.ConnectedDirectPage
	if decision == nil {
		if _, ok := world.elements.Anchor(trace.target); !ok {
			decision = corecontract.ErrConflict
		} else if continuation != nil && !queryrequest.EqualPath(queryrequest.ConnectedDirectPath(*(continuation)), normalized) {
			decision = corecontract.ErrInvalid
		} else {
			var after values.ElementID
			if continuation != nil {
				after = queryrequest.ConnectedDirectLast(*(continuation))
			}
			page = connectedDirectPage(world, opening.context, trace.target, normalized, limit, after)
		}
	}
	terminal, err := core.finalizeConnected(ctx, opening, initial)
	if err != nil {
		return corecontract.ConnectedDirectPage{}, err
	}
	if decision != nil {
		return corecontract.ConnectedDirectPage{}, decision
	}
	if terminal {
		kept := page.Records[:0]
		for _, record := range page.Records {
			if record.Source.Authority != opening.context.key.CarrierAuthority {
				kept = append(kept, record)
			}
		}
		page.Records, page.Continuation = kept, nil
		page.Status, page.Gap = corecontract.QueryPartial, page.Gap|corecontract.QueryUnresolvedCoverage
	}
	success = true
	return page, nil
}

func connectedDirectPage(world *connectedWorld, context *connectedContext, parent values.ElementID, path corecontract.Path, limit uint16, after values.ElementID) corecontract.ConnectedDirectPage {
	ids := make([]values.ElementID, 0)
	for id, anchor := range world.elements.Anchors() {
		if anchor.Parent == parent && !world.transfers.Inactive(id) {
			ids = append(ids, id)
		}
	}
	for id, entity := range world.elements.Entities() {
		if entity.Parent == parent && !world.transfers.Inactive(id) {
			ids = append(ids, id)
		}
	}
	elementfact.SortIDs(ids)
	page := corecontract.ConnectedDirectPage{Records: make([]corecontract.ConnectedRecord, 0), Cut: context.cut, ReadMode: context.mode, Status: corecontract.QueryComplete}
	for _, id := range ids {
		if after != "" && id <= after {
			continue
		}
		if len(page.Records) == int(limit) {
			page.Status = corecontract.QueryPartial
			page.Continuation = queryrequest.ConnectedDirectContinuationFromParts(context.cut, queryrequest.ClonePath(path), connectedRecordID(page.Records[len(page.Records)-1].Record))
			break
		}
		record, _ := connectedWorldRecord(world, id)
		page.Records = append(page.Records, record)
	}
	if context.excluded {
		page.Gap, page.Status = corecontract.QueryUnresolvedCoverage, corecontract.QueryPartial
	}
	for fence := range world.transfers.Fences() {
		if fence.Origin.Parent == parent {
			page.Gap, page.Status = corecontract.QueryUnresolvedCoverage, corecontract.QueryPartial
		}
	}
	return page
}

func connectedRequiredError(err error) error {
	if errors.Is(err, corecontract.ErrMountContextRequired) || errors.Is(err, corecontract.ErrOwnerMismatch) {
		return corecontract.ErrUnavailable
	}
	return err
}
