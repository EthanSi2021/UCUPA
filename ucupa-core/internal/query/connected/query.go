package connected

import (
	"context"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	queryscan "ucupa/ucupa-core/internal/query/scan"
	coretoken "ucupa/ucupa-core/internal/token"
)

// QueryRegionPath resolves one fixed Scope, discovers its complete descendant
// closure, and returns source-qualified summaries in canonical UUID order.
func (core *Operations) QueryRegionPath(ctx context.Context, callers contract.LocalCallers, query contract.PathRegionQuery, at *contract.ConnectedCut, limit uint16, continuation *contract.ConnectedQueryContinuation) (contract.ConnectedQueryPage, error) {
	intent, valid := canonicalConnectedIntent(exchange.Intent{Kind: exchange.Query, Query: query})
	if !valid || limit == 0 || limit > 256 || (continuation != nil && (at != nil || !coretoken.ValidConnectedCut(queryrequest.ConnectedQueryCut(*(continuation))) || !queryrequest.ConnectedQueryLast(*(continuation)).Valid())) {
		return contract.ConnectedQueryPage{}, contract.ErrInvalid
	}
	if continuation != nil {
		cut := queryrequest.ConnectedQueryCut(*continuation)
		at = &cut
	}
	opening, initial, err := core.beginConnected(ctx, callers, intent, at)
	if err != nil {
		return contract.ConnectedQueryPage{}, connectedRequiredError(err)
	}
	success := false
	defer func() { core.completeConnected(opening, initial, success) }()
	world, trace, scope, depth, scopePath, decision := core.prepareConnectedOperation(ctx, opening, intent, initial)
	var page contract.ConnectedQueryPage
	if decision == nil {
		basis, basisErr := traceConnectedPath(world.elements, world.mount, world.transfers, intent.Query.BasisPath)
		if basisErr != nil || basis.absent || basis.remote || !connectedDescendant(world.elements, basis.target, scope) {
			decision = contract.ErrInvalid
			if basis.remote || world.missing[basis.parent] {
				decision = contract.ErrUnavailable
			}
		} else if _, anchor := world.elements.Anchor(basis.target); !anchor {
			decision = contract.ErrInvalid
		} else {
			canonical := intent.Query
			canonical.StartPath, decision = connectedShortestPath(world.elements, intent.Path.OriginAnchorID, trace.target)
			if decision == nil {
				canonical.BasisPath, decision = connectedShortestPath(world.elements, intent.Path.OriginAnchorID, basis.target)
			}
			if decision == nil && continuation != nil && !queryrequest.EqualQuery(queryrequest.ConnectedQuery(*(continuation)), canonical) {
				decision = contract.ErrInvalid
			}
			if decision == nil {
				page, decision = scanConnectedQuery(world, opening.context, canonical, scope, basis.target, depth, scopePath, limit, continuation)
			}
		}
	}
	terminal, err := core.finalizeConnected(ctx, opening, initial)
	if err != nil {
		return contract.ConnectedQueryPage{}, connectedRequiredError(err)
	}
	if decision != nil {
		return contract.ConnectedQueryPage{}, connectedRequiredError(decision)
	}
	if terminal {
		kept := page.Matches[:0]
		for _, match := range page.Matches {
			if match.Source.Authority != opening.context.key.CarrierAuthority {
				kept = append(kept, match)
			}
		}
		page.Matches, page.Continuation = kept, nil
		page.Status, page.Gap = contract.QueryPartial, page.Gap|contract.QueryUnresolvedCoverage
	}
	success = true
	return page, nil
}

func scanConnectedQuery(world *connectedWorld, context *connectedContext, query contract.PathRegionQuery, scope, basis values.ElementID, depth uint64, scopePath contract.Path, limit uint16, continuation *contract.ConnectedQueryContinuation) (contract.ConnectedQueryPage, error) {
	var cursor *contract.QueryContinuation
	if continuation != nil {
		cursor = queryrequest.QueryContinuationFromParts("", [32]byte{}, [32]byte{}, contract.RegionQuery{}, values.PrincipalRef{}, contract.CoreReadCut{}, queryrequest.ConnectedQueryLast(*(continuation)), 0, "", 0, nil)
	}
	localQuery := contract.RegionQuery{StartAnchorID: scope, BasisAnchorID: basis, Sphere: query.Sphere, Selector: query.Selector}
	local, err := queryscan.Coverage(world.elements, world.mount, world.transfers, [32]byte{}, localQuery, contract.CoreReadCut{}, limit, cursor, false)
	if err != nil {
		return contract.ConnectedQueryPage{}, err
	}
	gap, err := queryscan.TransferGap(world.elements, world.transfers, localQuery)
	if err != nil {
		return contract.ConnectedQueryPage{}, err
	}
	if gap {
		local.Status, local.Gap = contract.QueryPartial, local.Gap|contract.QueryUnresolvedCoverage
	}
	page := contract.ConnectedQueryPage{Matches: make([]contract.ConnectedMatch, 0, len(local.Matches)), Cut: context.cut,
		ReadMode: context.mode, ResolvedScopePath: queryrequest.ClonePath(scopePath), ActualUpDepth: depth,
		Status: local.Status, Gap: local.Gap}
	for _, match := range local.Matches {
		source, exists := world.sources[match.ElementID]
		if !exists {
			return contract.ConnectedQueryPage{}, contract.ErrStore
		}
		page.Matches = append(page.Matches, contract.ConnectedMatch{ElementID: match.ElementID, Kind: match.Kind,
			Revision: match.Revision, Path: append([]values.ElementID(nil), match.Path...), Source: source})
	}
	if local.Continuation != nil {
		page.Continuation = queryrequest.ConnectedQueryContinuationFromParts(context.cut, queryrequest.CloneQuery(query), queryrequest.QueryLast(*(local.Continuation)))
	}
	for id := range world.missing {
		if connectedDescendant(world.elements, id, scope) {
			page.Gap |= contract.QueryUnresolvedCoverage
			page.Status = contract.QueryPartial
		}
	}
	return page, nil
}
