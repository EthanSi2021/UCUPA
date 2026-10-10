package connected

import (
	"context"
	"errors"
	"slices"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	queryrequest "ucupa/ucupa-core/internal/query/request"
)

type connectedOpening struct {
	context *connectedContext
	replies []exchange.ReplyData
	cleanup *exchange.Cleanup
	grants  []exchange.Grant
	intent  exchange.Intent
}

func (core *Operations) inspectConnectedLocal(actor values.PrincipalRef, intent exchange.Intent) (exchange.Facts, error) {
	if !actor.Valid() {
		return exchange.Facts{}, corecontract.ErrDenied
	}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		grant := exchange.Grant{Kind: intent.Kind, Scope: intent.Path.OriginAnchorID}
		if _, err := core.authorizeConnected(actor, grant); err != nil {
			return exchange.Facts{}, err
		}
		return exchange.Facts{}, corecontract.ErrStore
	}
	candidate, valid := core.owner.CaptureReadLocked()
	cut := core.owner.CurrentCutLocked()
	core.owner.RUnlock()
	if !valid {
		if _, err := core.authorizeConnected(actor, exchange.Grant{Kind: intent.Kind, Scope: intent.Path.OriginAnchorID}); err != nil {
			return exchange.Facts{}, err
		}
		return exchange.Facts{}, corecontract.ErrStore
	}
	projection := candidate.Projection
	return exchange.Facts{Present: true, Authority: projection.Authority, Scope: projection.Scope, CutSerial: projection.CutSerial,
		Elements: projection.Elements, Attachments: projection.Attachments, Mount: projection.Mount, Transfers: projection.Transfers,
		Cut: cut, Run: core.queries.Run()}, nil
}

func connectedIntentAt(intent exchange.Intent, path corecontract.Path, depth int64) exchange.Intent {
	intent.Path = queryrequest.ClonePath(path)
	if intent.Kind == exchange.Query {
		intent.Query.StartPath = queryrequest.ClonePath(path)
		intent.Query.BasisPath = corecontract.Path{OriginAnchorID: path.OriginAnchorID}
		intent.Query.UpDepth = depth
	}
	return intent
}

// remoteConnectedPlan derives the sole permitted hop from an observed mount.
// It never scans endpoints or asks an authority Ref to locate its address.
func remoteConnectedPlan(local exchange.Facts, intent exchange.Intent) (connectedSourcePlan, corecontract.MountKey, bool, bool, error) {
	state := local
	trace, err := traceConnectedPath(state.Elements, state.Mount, state.Transfers, intent.Path)
	if err != nil || trace.absent {
		return connectedSourcePlan{}, corecontract.MountKey{}, false, false, err
	}
	depth := int64(0)
	if intent.Kind == exchange.Query {
		depth = intent.Query.UpDepth
	}
	if trace.remote {
		ref := trace.key.CarrierAuthority
		if state.Authority == ref {
			ref = trace.key.ParentAuthority
		}
		return connectedSourcePlan{authority: ref, hop: trace.contact.NextHop,
			intent: connectedIntentAt(intent, trace.remaining, depth)}, trace.key, true, false, nil
	}
	if in := state.Mount.Incoming(); in.Key.Valid() {
		remote := connectedSourcePlan{authority: in.Key.ParentAuthority, hop: in.Contact.NextHop,
			intent: connectedIntentAt(intent, corecontract.Path{OriginAnchorID: in.Key.MountAnchorUUID}, 0)}
		proofOnly := true
		if intent.Kind == exchange.Query {
			_, n, scopeErr := connectedScope(state.Elements, state.Mount, trace.target, depth)
			if errors.Is(scopeErr, corecontract.ErrMountContextRequired) {
				remaining := depth
				if remaining >= 0 {
					remaining -= int64(n)
				}
				remote.intent = connectedIntentAt(intent, corecontract.Path{OriginAnchorID: in.Key.MountAnchorUUID}, remaining)
				proofOnly = false
			}
		} else if trace.target == in.Key.MountAnchorUUID {
			proofOnly = false
		}
		if proofOnly && slices.Contains(trace.ids, in.Key.MountAnchorUUID) {
			remote.intent = exchange.Intent{Kind: exchange.Resolve, Path: corecontract.Path{OriginAnchorID: in.Key.MountAnchorUUID}}
			proofOnly = false
		}
		return remote, in.Key, true, proofOnly, nil
	}
	if intent.Kind == exchange.Resolve {
		return connectedSourcePlan{}, corecontract.MountKey{}, false, false, nil
	}
	scope := trace.target
	if intent.Kind == exchange.Query {
		scope, _, err = connectedScope(state.Elements, state.Mount, trace.target, depth)
		if err != nil {
			return connectedSourcePlan{}, corecontract.MountKey{}, false, false, err
		}
	}
	for id, mount := range state.Mount.Outgoings() {
		if mount.State == corecontract.MountNone || state.Transfers.Inactive(id) {
			continue
		}
		if (intent.Kind == exchange.List && id == scope) ||
			(intent.Kind == exchange.Query && connectedDescendant(state.Elements, id, scope)) {
			necessary := intent.Kind == exchange.List
			if intent.Kind == exchange.Query {
				basis, _ := traceConnectedPath(state.Elements, state.Mount, state.Transfers, intent.Query.BasisPath)
				necessary = basis.remote && basis.key == mount.Key
			}
			return connectedSourcePlan{authority: mount.Key.CarrierAuthority, hop: mount.Contact.NextHop,
				intent: connectedIntentAt(intent, corecontract.Path{OriginAnchorID: id}, 0)}, mount.Key, necessary, false, nil
		}
	}
	return connectedSourcePlan{}, corecontract.MountKey{}, false, false, nil
}

func connectedWorldScope(world *connectedWorld, intent exchange.Intent) (connectedTrace, values.ElementID, uint64, corecontract.Path, error) {
	trace, err := traceConnectedPath(world.elements, world.mount, world.transfers, intent.Path)
	if err != nil {
		return trace, "", 0, corecontract.Path{}, err
	}
	if trace.absent {
		if intent.Kind == exchange.Resolve {
			return trace, trace.target, 0, corecontract.Path{}, nil
		}
		return trace, "", 0, corecontract.Path{}, corecontract.ErrNotFound
	}
	scope, depth := trace.target, uint64(0)
	if intent.Kind == exchange.Query {
		scope, depth, err = connectedScope(world.elements, world.mount, trace.target, intent.Query.UpDepth)
		if err != nil {
			return trace, "", 0, corecontract.Path{}, err
		}
	}
	path, err := connectedShortestPath(world.elements, intent.Path.OriginAnchorID, scope)
	return trace, scope, depth, path, err
}

func connectedGrantForWorld(local exchange.Facts, world *connectedWorld, intent exchange.Intent, key corecontract.MountKey, trace connectedTrace, scope values.ElementID) exchange.Grant {
	grant := exchange.Grant{Kind: intent.Kind, Scope: scope, Mount: key}
	owner := local.Authority
	source, owned := world.sources[scope]
	owned = owned && source.Authority == owner
	if intent.Kind == exchange.Query {
		if !owned {
			if owner == key.CarrierAuthority && connectedDescendant(world.elements, key.MountAnchorUUID, scope) {
				grant.Scope = key.MountAnchorUUID
			} else {
				grant.ProofOnly = true
				grant.Scope = key.MountAnchorUUID
			}
		}
	} else if !owned {
		if intent.Kind == exchange.List && owner == key.CarrierAuthority && scope == key.MountAnchorUUID {
			grant.Scope = key.MountAnchorUUID
		} else {
			grant.ProofOnly = true
			grant.Scope = key.MountAnchorUUID
		}
	}
	for _, id := range trace.ids {
		if local.Elements.Kind(id) == 0 {
			continue
		}
		if intent.Kind == exchange.Query && connectedDescendant(world.elements, id, scope) {
			continue
		}
		if intent.Kind == exchange.List && id == trace.target {
			continue
		}
		if !slices.Contains(grant.Prefix, id) {
			grant.Prefix = append(grant.Prefix, id)
		}
	}
	if trace.absent && intent.Kind == exchange.Resolve {
		parentSource := world.sources[trace.parent]
		if trace.parent == key.MountAnchorUUID {
			parentSource.Authority = key.CarrierAuthority
		}
		if parentSource.Authority == owner || trace.parent == "" {
			grant.ProofOnly = false
			grant.Scope = intent.Path.OriginAnchorID
			if len(intent.Path.Down) > len(trace.ids)-1 {
				grant.Scope = intent.Path.Down[len(trace.ids)-1]
			}
			if trace.parent == key.MountAnchorUUID && owner == key.CarrierAuthority {
				grant.Adjacency = append(grant.Adjacency, trace.parent)
			}
		}
	}
	if !grant.Scope.Valid() {
		grant.Scope = intent.Path.OriginAnchorID
	}
	if grant.ProofOnly && owner == key.CarrierAuthority && intent.Kind == exchange.Resolve &&
		intent.Path.OriginAnchorID == key.MountAnchorUUID && intent.Path.Up == 0 && len(intent.Path.Down) == 0 {
		grant.EntryProof = true
	}
	return grant
}

func (core *Operations) openConnected(ctx context.Context, callers corecontract.LocalCallers, intent exchange.Intent) (opened *connectedOpening, resultErr error) {
	local, err := core.inspectConnectedLocal(callers[core.authority], intent)
	if err != nil {
		return nil, err
	}
	diagnostic, _ := connectedProbeGrant(local.Elements, local.Mount, local.Transfers, intent, corecontract.MountKey{}, false)
	defer func() {
		if resultErr == nil {
			return
		}
		approval, err := core.authorizeConnected(callers[core.authority], diagnostic)
		if err != nil {
			opened, resultErr = nil, err
			return
		}
		if errors.Is(resultErr, corecontract.ErrDenied) {
			return
		}
		core.owner.RLock()
		defer core.owner.RUnlock()
		if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
			opened, resultErr = nil, corecontract.ErrStore
		} else if !core.settled(approval) {
			opened, resultErr = nil, corecontract.ErrUnavailable
		} else if core.owner.CurrentCutLocked() != local.Cut {
			opened, resultErr = nil, corecontract.ErrReadCutExpired
		}
	}()
	probeCleanup := &exchange.Cleanup{}
	defer probeCleanup.Close()
	remote, key, required, proofOnly, err := remoteConnectedPlan(local, intent)
	if err != nil {
		grant, _ := connectedProbeGrant(local.Elements, local.Mount, local.Transfers, intent, corecontract.MountKey{}, false)
		if _, err := core.authorizeConnected(callers[core.authority], grant); err != nil {
			return nil, err
		}
		return nil, err
	}
	preliminary, _ := connectedProbeGrant(local.Elements, local.Mount, local.Transfers, intent, corecontract.MountKey{}, false)
	if _, err := core.authorizeConnected(callers[core.authority], preliminary); err != nil {
		return nil, err
	}
	if !connectedPairSupported(local.Mount) {
		return nil, corecontract.ErrUnavailable
	}
	parts := []exchange.Facts{local}
	var remoteReply exchange.ReplyData
	excluded := false
	excludedAt := key.MountAnchorUUID
	mountApproval := corecontract.RealmAttachment{}
	mountAuthorized := false
	if remote.authority.Valid() {
		if core.authority == key.ParentAuthority {
			granted, err := core.authorize(callers[core.authority], core.requirement(corecontract.PermissionMountRead, key.MountAnchorUUID))
			if err != nil {
				if required || !errors.Is(err, corecontract.ErrDenied) {
					return nil, err
				}
				excluded = true
				remote.authority = ""
				key = corecontract.MountKey{}
			} else {
				mountApproval, mountAuthorized = granted, true
			}
		}
	}
	if remote.authority.Valid() {
		remote.actor = callers[remote.authority]
		if !remote.actor.Valid() {
			if required {
				return nil, corecontract.ErrDenied
			}
			excluded = true
		}
		remoteReply, err = core.callConnectedPeer(ctx, remote, exchange.RequestData{Action: exchange.Probe,
			Intent: remote.intent, Key: key, ProofOnly: proofOnly, Cleanup: probeCleanup})
		if err != nil {
			if required || errors.Is(err, corecontract.ErrSnapshotCapacity) || ctx.Err() != nil {
				return nil, err
			}
			excluded = true
			key = corecontract.MountKey{}
		} else {
			parts = append(parts, remoteReply.Facts)
		}
	}
	if mountAuthorized {
		core.owner.RLock()
		if !core.settled(mountApproval) {
			core.owner.RUnlock()
			return nil, corecontract.ErrUnavailable
		}
		core.owner.RUnlock()
	}
	world, err := newConnectedWorld(parts)
	if err != nil {
		return nil, err
	}
	trace, scope, depth, scopePath, decision := connectedWorldScope(world, intent)
	grant := connectedGrantForWorld(local, world, intent, key, trace, scope)
	if decision != nil {
		grant, _ = connectedProbeGrant(local.Elements, local.Mount, local.Transfers, intent, key, false)
	}
	diagnostic = grant
	approval, err := core.authorizeConnected(callers[core.authority], grant)
	if err != nil {
		return nil, err
	}
	if decision != nil {
		return nil, decision
	}
	localPlan := connectedSourcePlan{authority: core.authority, actor: callers[core.authority],
		grant: grant, intent: intent, cut: local.Cut, run: local.Run}
	localReply, err := core.callConnectedPeer(ctx, localPlan, exchange.RequestData{Action: exchange.Probe,
		Intent: intent, Grant: grant, Key: key, Cleanup: probeCleanup})
	if err != nil {
		return nil, err
	}
	if localReply.Facts.Cut != local.Cut {
		return nil, corecontract.ErrReadCutExpired
	}
	localPlan.cut, localPlan.run, localPlan.contact = localReply.Facts.Cut, localReply.Facts.Run, localReply.Contact
	context := &connectedContext{mode: localReply.Mode, intent: intent, scope: scope, depth: depth,
		scopePath: scopePath, key: key, excluded: excluded, excludedAt: excludedAt, requiredB: required, sources: []connectedSourcePlan{localPlan}}
	if key.Valid() {
		context.originToMount, err = connectedShortestPath(world.elements, intent.Path.OriginAnchorID, key.MountAnchorUUID)
		if err != nil {
			return nil, err
		}
	}
	if remote.authority.Valid() && !excluded {
		remote.grant, remote.cut, remote.run, remote.contact = remoteReply.Grant, remoteReply.Facts.Cut, remoteReply.Facts.Run, remoteReply.Contact
		context.sources = append(context.sources, remote)
		if remoteReply.Mode == corecontract.QueryCurrentVerified {
			context.mode = corecontract.QueryCurrentVerified
		}
	}
	core.owner.RLock()
	if !core.settled(approval) {
		core.owner.RUnlock()
		return nil, corecontract.ErrUnavailable
	}
	core.owner.RUnlock()
	// Drop every probe borrow before reserving the formal views. A Core with a
	// single concurrent-read slot can therefore serve a single connected call.
	probeCleanup.Close()
	return core.captureConnected(ctx, callers, context)
}

func (core *Operations) captureConnected(ctx context.Context, callers corecontract.LocalCallers, context *connectedContext) (*connectedOpening, error) {
	cleanup := &exchange.Cleanup{}
	keep := false
	defer func() {
		if !keep {
			cleanup.Close()
		}
	}()
	// A first, then B, independent of the API entry Core. There are at most two
	// sources in this profile; the list remains fixed after mode selection.
	slices.SortFunc(context.sources, func(a, b connectedSourcePlan) int {
		if a.authority == context.key.ParentAuthority {
			return -1
		}
		if b.authority == context.key.ParentAuthority {
			return 1
		}
		return 0
	})
	opening := &connectedOpening{context: context, cleanup: cleanup}
	for i, plan := range context.sources {
		reply, err := core.callConnectedPeer(ctx, plan, exchange.RequestData{Action: exchange.Capture,
			Grant: plan.grant, Intent: plan.intent, Key: context.key, Mode: context.mode, Cut: plan.cut, Run: plan.run, Cleanup: cleanup})
		if err != nil {
			return nil, err
		}
		context.sources[i].snapshot = reply.Snapshot
		opening.replies = append(opening.replies, reply)
	}
	if err := connectedMatchAdmission(context, opening.replies); err != nil {
		return nil, err
	}
	if context.mode == corecontract.QueryRetained {
		if err := core.checkConnectedSource(ctx, context, 0, opening.replies[0], true); err != nil {
			return nil, err
		}
	}
	if err := core.publishConnectedContext(callers, context); err != nil {
		return nil, err
	}
	keep = true
	return opening, nil
}

func connectedMatchAdmission(context *connectedContext, replies []exchange.ReplyData) error {
	if !context.key.Valid() || len(replies) != 2 {
		return nil
	}
	a, b := replies[0].Facts, replies[1].Facts
	mount, exists := a.Mount.Outgoing(context.key.MountAnchorUUID)
	incoming := b.Mount.Incoming()
	if !exists || mount.Key != context.key || mount.Admission != incoming.Admission.Key || incoming.Key != context.key {
		return corecontract.ErrUnavailable
	}
	// MountRead authorizes this Range evidence without exposing M's full
	// Element record when A contributes only the current relationship proof.
	if !geometry.ValidSphere(replies[0].Facts.AdmissionRange) || replies[0].Facts.AdmissionRange != incoming.Admission.RangeValue {
		return corecontract.ErrUnavailable
	}
	return nil
}
