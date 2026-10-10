package connected

import (
	"context"
	"errors"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	coretoken "ucupa/ucupa-core/internal/token"
)

func (core *Operations) checkConnectedSource(ctx context.Context, context *connectedContext, index int, reply exchange.ReplyData, current bool, override ...exchange.Grant) error {
	plan := context.sources[index]
	grant := plan.grant
	if len(override) != 0 {
		grant = override[0]
	}
	_, err := core.callConnectedPeer(ctx, plan, exchange.RequestData{Action: exchange.Check,
		Key: context.key, Lease: reply.Lease, Contact: plan.contact, CheckCurrent: current, Grant: grant})
	return err
}

func (core *Operations) resumeConnected(ctx context.Context, callers corecontract.LocalCallers, cut corecontract.ConnectedCut, intent exchange.Intent) (*connectedOpening, error) {
	context, err := core.borrowConnectedContext(callers, cut)
	if err != nil {
		return nil, err
	}
	cleanup := &exchange.Cleanup{}
	opening := &connectedOpening{context: context, cleanup: cleanup, replies: make([]exchange.ReplyData, len(context.sources))}
	keep := false
	defer func() {
		if !keep {
			cleanup.Close()
			core.finishConnectedContext(context)
		}
	}()
	terminal, err := core.connectedTerminal(context, false)
	if err != nil {
		return nil, err
	}
	order := make([]int, 0, len(context.sources))
	for i, plan := range context.sources {
		if plan.authority == core.authority {
			order = append(order, i)
		}
	}
	for i, plan := range context.sources {
		if plan.authority != core.authority {
			order = append(order, i)
		}
	}
	for _, i := range order {
		plan := context.sources[i]
		if terminal && plan.authority == context.key.CarrierAuthority {
			continue
		}
		localIntent, verifyOnly := connectedResumeIntent(context, plan, intent)
		if intent.Kind == exchange.Query && plan.authority != core.authority {
			localIntent, verifyOnly = connectedResumeQueryDependency(context, intent, opening.replies[order[0]], localIntent, verifyOnly)
		}
		reply, err := core.callConnectedPeer(ctx, plan, exchange.RequestData{Action: exchange.Borrow,
			Grant: plan.grant, Key: context.key, Mode: context.mode, Cut: plan.cut, Run: plan.run,
			Snapshot: plan.snapshot, Cleanup: cleanup, Replan: true, Bounds: plan.grant, Intent: localIntent, VerifyOnly: verifyOnly})
		if err != nil {
			if !connectedNeedsCarrier(context, intent) && intent.Kind == exchange.Query && plan.authority == context.key.CarrierAuthority && connectedCanTerminate(err) {
				if _, err := core.connectedTerminal(context, true); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		opening.replies[i] = reply
	}
	keep = true
	return opening, nil
}

func connectedCanTerminate(err error) bool {
	return errors.Is(err, corecontract.ErrUnavailable) || errors.Is(err, corecontract.ErrDenied) || errors.Is(err, corecontract.ErrOwnerMismatch)
}

func (core *Operations) finalizeConnected(ctx context.Context, opening *connectedOpening, initial bool) (bool, error) {
	context := opening.context
	// CurrentVerified's last checks are B then A. Retained applies the same
	// disclosure order but checks pinned lifetime instead of ordinary cut drift.
	for i := len(context.sources) - 1; i >= 0; i-- {
		if !opening.replies[i].Facts.Present {
			continue
		}
		grant := context.sources[i].grant
		if len(opening.grants) > i && opening.grants[i].Scope.Valid() {
			grant = opening.grants[i]
		}
		err := core.checkConnectedSource(ctx, context, i, opening.replies[i], false, grant)
		if err == nil {
			continue
		}
		if !initial && opening.intent.Kind == exchange.Query && !connectedNeedsCarrier(context, opening.intent) && context.sources[i].authority == context.key.CarrierAuthority && connectedCanTerminate(err) {
			if _, err := core.connectedTerminal(context, true); err != nil {
				return false, err
			}
			continue
		}
		return false, err
	}
	return core.connectedTerminal(context, false)
}

func connectedWorldFromOpening(opening *connectedOpening, omitCarrier bool) (*connectedWorld, error) {
	parts := make([]exchange.Facts, 0, len(opening.replies))
	for _, reply := range opening.replies {
		if !reply.Facts.Present || (omitCarrier && reply.Authority == opening.context.key.CarrierAuthority) {
			continue
		}
		parts = append(parts, reply.Facts)
	}
	world, err := newConnectedWorld(parts)
	if err != nil {
		return nil, err
	}
	if opening.context.excluded {
		world.missing[opening.context.excludedAt] = true
	}
	if omitCarrier {
		world.missing[opening.context.key.MountAnchorUUID] = true
	}
	return world, nil
}

func connectedAllowed(context *connectedContext, world *connectedWorld, trace connectedTrace, scope values.ElementID, intent exchange.Intent) error {
	kind := intent.Kind
	switch context.intent.Kind {
	case exchange.Query:
		if !connectedDescendant(world.elements, scope, context.scope) {
			// Narrow owner replanning has already checked its original granted
			// closure. Explanatory records from another owner need not be sent
			// again merely to re-prove this fixed source's inclusion in the range.
			inside := false
			for _, source := range context.sources {
				if source.authority == world.sources[scope].Authority && source.grant.Kind == exchange.Query && !source.grant.ProofOnly &&
					connectedDescendant(world.elements, scope, source.grant.Scope) {
					inside = true
					break
				}
			}
			if !inside {
				return corecontract.ErrInvalid
			}
		}
	case exchange.List:
		if trace.absent && trace.parent != context.scope {
			return corecontract.ErrInvalid
		}
		if kind != exchange.Resolve && (kind != exchange.List || scope != context.scope) {
			return corecontract.ErrInvalid
		}
		if scope != context.scope {
			parent, ok := world.elements.ParentOf(scope)
			if !ok || parent != context.scope {
				return corecontract.ErrInvalid
			}
		}
	case exchange.Resolve:
		if kind != exchange.Resolve {
			if scope == context.scope && kind == exchange.List {
				return corecontract.ErrUnavailable
			}
			return corecontract.ErrInvalid
		}
		original, err := traceConnectedPath(world.elements, world.mount, world.transfers, context.intent.Path)
		if err != nil {
			return err
		}
		found := false
		for _, id := range original.ids {
			if id == scope {
				found = true
				break
			}
		}
		if !found || (trace.absent && (!original.absent || !queryrequest.EqualPath(context.intent.Path, intent.Path))) {
			return corecontract.ErrInvalid
		}
	}
	return nil
}

func (core *Operations) authorizeConnectedOperation(ctx context.Context, opening *connectedOpening, world *connectedWorld, intent exchange.Intent, trace connectedTrace, scope values.ElementID) error {
	opening.grants = make([]exchange.Grant, len(opening.context.sources))
	for i, source := range opening.context.sources {
		reply := opening.replies[i]
		if !reply.Facts.Present {
			continue
		}
		grant := connectedGrantForWorld(reply.Facts, world, intent, opening.context.key, trace, scope)
		if grant.ProofOnly && len(grant.Prefix) == 0 && len(grant.Adjacency) == 0 && source.authority == opening.context.key.CarrierAuthority {
			grant.VerifyOnly = true
		}
		opening.grants[i] = cloneConnectedGrant(grant)
		// A narrower operation has its own exact/list/query permission. The peer
		// checks it itself; the coordinator never treats a summary as full read.
		_, err := core.callConnectedPeer(ctx, source, exchange.RequestData{Action: exchange.Check,
			Key: opening.context.key, Lease: reply.Lease, Contact: source.contact, Grant: grant})
		if err != nil {
			return err
		}
	}
	return nil
}

func (core *Operations) retireConnected(context *connectedContext) {
	r := core.registry
	r.mu.Lock()
	context.retired = true
	r.mu.Unlock()
}

// ReleaseConnectedCut invalidates the local context before best-effort release
// of its bounded owner pins. Unreachable owners reclaim those pins by deadline.
func (core *Operations) ReleaseConnectedCut(ctx context.Context, callers corecontract.LocalCallers, cut corecontract.ConnectedCut) error {
	if core == nil || ctx == nil {
		return corecontract.ErrInvalid
	}
	err := core.checkConnectedBinding(callers, cut)
	if err != nil {
		if errors.Is(err, corecontract.ErrReadCutExpired) {
			return nil
		}
		return err
	}
	core.owner.RLock()
	r := core.registry
	core.owner.RUnlock()
	if r == nil {
		return nil
	}
	r.mu.Lock()
	entry := r.contexts[coretoken.ConnectedID(cut)]
	if entry == nil {
		r.mu.Unlock()
		return nil
	}
	if entry.cut != cut {
		r.mu.Unlock()
		return corecontract.ErrInvalid
	}
	entry.retired = true
	if entry.borrows == 0 {
		delete(r.contexts, coretoken.ConnectedID(cut))
		r.bytes -= entry.bytes
	}
	sources := append([]connectedSourcePlan(nil), entry.sources...)
	key := entry.key
	r.mu.Unlock()
	for _, source := range sources {
		if source.snapshot != nil {
			_, _ = core.callConnectedPeer(ctx, source, exchange.RequestData{Action: exchange.Release, Key: key, Snapshot: source.snapshot})
		}
	}
	return nil
}

func connectedRecordID(record corecontract.ElementRecord) values.ElementID {
	if record.Kind == corecontract.ElementAnchor && record.Anchor != nil {
		return record.Anchor.ID
	}
	if record.Kind == corecontract.ElementEntity && record.Entity != nil {
		return record.Entity.ID
	}
	return ""
}
