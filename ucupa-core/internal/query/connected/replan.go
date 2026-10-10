package connected

import (
	"slices"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func connectedNeedsCarrier(context *connectedContext, intent exchange.Intent) bool {
	if context.requiredB || intent.Kind != exchange.Query {
		return true
	}
	for _, path := range []corecontract.Path{intent.Path, intent.Query.BasisPath} {
		if path.OriginAnchorID == context.key.MountAnchorUUID && len(path.Down) != 0 {
			return true
		}
		for index, id := range path.Down {
			if id == context.key.MountAnchorUUID && index+1 < len(path.Down) {
				return true
			}
		}
	}
	return false
}

func connectedResumeQueryDependency(context *connectedContext, intent exchange.Intent, local exchange.ReplyData, remoteIntent exchange.Intent, verifyOnly bool) (exchange.Intent, bool) {
	if !local.Facts.Present || local.Grant.Kind != exchange.Query || local.Grant.ProofOnly {
		return remoteIntent, verifyOnly
	}
	mount := context.key.MountAnchorUUID
	if local.Authority == context.key.ParentAuthority {
		if !connectedDescendant(local.Facts.Elements, mount, local.Grant.Scope) {
			return exchange.Intent{Kind: exchange.Query, Path: corecontract.Path{OriginAnchorID: mount}}, true
		}
		return remoteIntent, verifyOnly
	}
	if local.Grant.Scope != mount {
		return exchange.Intent{Kind: exchange.Query, Path: corecontract.Path{OriginAnchorID: mount}}, true
	}
	trace, err := traceConnectedPath(local.Facts.Elements, local.Facts.Mount, local.Facts.Transfers, intent.Path)
	if err != nil || trace.absent || trace.remote {
		return remoteIntent, verifyOnly
	}
	_, depth, err := connectedScope(local.Facts.Elements, local.Facts.Mount, trace.target, intent.Query.UpDepth)
	if err != corecontract.ErrMountContextRequired {
		return remoteIntent, verifyOnly
	}
	remaining := intent.Query.UpDepth
	if remaining >= 0 {
		remaining -= int64(depth)
	}
	return connectedIntentAt(intent, corecontract.Path{OriginAnchorID: mount}, remaining), false
}

// Every resumed operation is planned privately by each owner against its
// original cut. No old broad query grant is a prerequisite for a narrow read,
// and no broad records cross the peer boundary before the new grant succeeds.
func connectedResumeIntent(context *connectedContext, source connectedSourcePlan, intent exchange.Intent) (exchange.Intent, bool) {
	if source.authority == token.ConnectedOwner(context.cut) || !context.key.Valid() {
		return intent, false
	}
	mount := context.key.MountAnchorUUID
	path := intent.Path
	if source.authority == context.key.CarrierAuthority {
		if path.OriginAnchorID == mount && path.Up == 0 {
			return connectedIntentAt(intent, path, intent.Query.UpDepth), intent.Kind == exchange.Resolve && len(path.Down) == 0
		}
		for index, id := range path.Down {
			if id == mount {
				// M itself belongs to A. A read ending here needs only the
				// already registered B source's zero-content validity proof.
				return connectedIntentAt(intent, corecontract.Path{OriginAnchorID: mount, Down: append([]values.ElementID(nil), path.Down[index+1:]...)}, intent.Query.UpDepth), intent.Kind == exchange.Resolve && index+1 == len(path.Down)
			}
		}
		if intent.Kind == exchange.Query {
			return connectedIntentAt(intent, corecontract.Path{OriginAnchorID: mount}, 0), false
		}
		return exchange.Intent{Kind: intent.Kind, Path: corecontract.Path{OriginAnchorID: mount}}, true
	}
	// The receiving Core is B. Its Origin-to-M path consists of real upward
	// edges only; switching services consumes none of the remaining edge count.
	distance := context.originToMount.Up
	if path.Up >= distance {
		return connectedIntentAt(intent, corecontract.Path{OriginAnchorID: mount, Up: path.Up - distance,
			Down: append([]values.ElementID(nil), path.Down...)}, intent.Query.UpDepth), false
	}
	if intent.Kind == exchange.Query {
		// Ordinary continuation retains its original locally resolved segment.
		// The full world resolves and checks actual Scope again before disclosure.
		return source.intent, source.grant.ProofOnly
	}
	return exchange.Intent{Kind: intent.Kind, Path: corecontract.Path{OriginAnchorID: mount}}, true
}

func connectedReplanFallback(request exchange.RequestData) exchange.Grant {
	if request.VerifyOnly {
		return exchange.Grant{Scope: request.Key.MountAnchorUUID, Mount: request.Key, ProofOnly: true, VerifyOnly: true}
	}
	grant := exchange.Grant{Kind: request.Intent.Kind, Scope: request.Intent.Path.OriginAnchorID,
		Mount: request.Key, Prefix: append([]values.ElementID{request.Intent.Path.OriginAnchorID}, request.Intent.Path.Down...)}
	if len(request.Intent.Path.Down) != 0 {
		grant.Scope = request.Intent.Path.Down[len(request.Intent.Path.Down)-1]
	}
	if grant.Kind == exchange.Query {
		grant.Scope = request.Bounds.Scope
		grant.Prefix = append([]values.ElementID(nil), request.Bounds.Prefix...)
	}
	return grant
}

func (core *Operations) replanConnectedGrant(request exchange.RequestData) (exchange.Grant, error) {
	fallback := connectedReplanFallback(request)
	if !request.Bounds.Scope.Valid() || !token.Valid(request.Cut) || request.Run == ([32]byte{}) {
		return fallback, corecontract.ErrInvalid
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || request.Run != core.queries.Run() {
		return fallback, corecontract.ErrReadCutExpired
	}
	if !core.owner.AvailableLocked() {
		return fallback, corecontract.ErrStore
	}
	elements := core.owner.FactsLocked().Elements
	mounts := core.owner.FactsLocked().Mount
	transfers := core.owner.SourceVisibilityLocked()
	if request.Mode == corecontract.QueryRetained {
		plan, err := core.queries.PlanRetained(request.Actor, request.Snapshot)
		if err != nil {
			return fallback, err
		}
		defer plan.Finish()
		projection := plan.Projection()
		elements, mounts = projection.Elements, projection.Mount
		transfers = projection.Transfers
	} else if core.owner.CurrentCutLocked() != request.Cut {
		return fallback, corecontract.ErrReadCutExpired
	}
	if request.VerifyOnly {
		return fallback, nil
	}
	grant, err := connectedProbeGrant(elements, mounts, transfers, request.Intent, request.Key, false)
	if err != nil {
		return grant, err
	}
	if err := connectedGrantWithin(elements, mounts, transfers, request.Bounds, grant, request.Intent); err != nil {
		return grant, err
	}
	return grant, nil
}

func connectedGrantWithin(elements elementfact.View, mounts mountfact.View, transfers transferfact.Visibility, bounds, grant exchange.Grant, intent exchange.Intent) error {
	allowed := func(id values.ElementID) bool {
		if slices.Contains(bounds.Prefix, id) || id == bounds.Scope {
			return true
		}
		switch bounds.Kind {
		case exchange.Query:
			return !bounds.ProofOnly && connectedDescendant(elements, id, bounds.Scope)
		case exchange.List:
			parent, exists := elements.ParentOf(id)
			return !bounds.ProofOnly && exists && parent == bounds.Scope
		}
		return false
	}
	for _, id := range grant.Prefix {
		if elements.Kind(id) != 0 && !allowed(id) {
			return corecontract.ErrInvalid
		}
	}
	if grant.ProofOnly {
		return nil
	}
	if grant.Kind == exchange.Query && (bounds.Kind != exchange.Query || bounds.ProofOnly || !connectedDescendant(elements, grant.Scope, bounds.Scope)) {
		return corecontract.ErrInvalid
	}
	if grant.Kind == exchange.List && bounds.Kind == exchange.Resolve {
		if grant.Scope == bounds.Scope {
			return corecontract.ErrUnavailable
		}
		return corecontract.ErrInvalid
	}
	if grant.Kind == exchange.List && bounds.Kind == exchange.List && grant.Scope != bounds.Scope {
		return corecontract.ErrInvalid
	}
	if allowed(grant.Scope) {
		return nil
	}
	trace, err := traceConnectedPath(elements, mounts, transfers, intent.Path)
	if err != nil || !trace.absent {
		return corecontract.ErrInvalid
	}
	if (bounds.Kind == exchange.Query && connectedDescendant(elements, trace.parent, bounds.Scope)) ||
		(bounds.Kind == exchange.List && trace.parent == bounds.Scope) ||
		(bounds.Kind == exchange.Resolve && trace.absentID == bounds.Scope) {
		return nil
	}
	return corecontract.ErrInvalid
}

// A state-derived failure has the same last-cut rule as successful disclosure.
// Authorization callbacks may have committed facts after the private diagnosis.
func (core *Operations) checkConnectedFailureCut(request exchange.RequestData, approval corecontract.RealmAttachment, authorized bool) error {
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() {
		if request.Action == exchange.Borrow {
			return corecontract.ErrReadCutExpired
		}
		return corecontract.ErrStore
	}
	if !core.owner.AvailableLocked() {
		return corecontract.ErrStore
	}
	if authorized && !core.settled(approval) {
		return corecontract.ErrUnavailable
	}
	if request.Action == exchange.Borrow && request.Mode == corecontract.QueryRetained {
		return core.queries.CheckRetainedFailure(request.Snapshot)
	}
	if core.owner.CurrentCutLocked() != request.Cut {
		return corecontract.ErrReadCutExpired
	}
	return nil
}
