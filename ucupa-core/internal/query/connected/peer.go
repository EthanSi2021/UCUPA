package connected

import (
	"context"
	"errors"
	"math"
	"reflect"
	peercontract "ucupa/ucupa-core/contract/connected"
	querypkg "ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

type localConnectedPeer struct{ core *Operations }

func (core *Operations) ConnectedPeer() peercontract.PeerPort { return localConnectedPeer{core: core} }

func (peer localConnectedPeer) AuthorityRef() values.FactAuthorityRef {
	return peer.core.AuthorityRef()
}

func (core *Operations) ConfigureConnectedTransport(transport peercontract.Transport) error {
	if core == nil {
		return contract.ErrInvalid
	}
	if transport != nil {
		value := reflect.ValueOf(transport)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			if value.IsNil() {
				return contract.ErrInvalid
			}
		}
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return contract.ErrStore
	}
	core.connectedTransport = transport
	return nil
}

func (peer localConnectedPeer) ReadConnected(ctx context.Context, source values.FactAuthorityRef, request peercontract.Request) (peercontract.Reply, error) {
	data := exchange.ReadRequest(request)
	if peer.core == nil || !source.Valid() || !data.Key.Valid() ||
		!((source == data.Key.ParentAuthority && peer.core.authority == data.Key.CarrierAuthority) ||
			(source == data.Key.CarrierAuthority && peer.core.authority == data.Key.ParentAuthority)) {
		return peercontract.Reply{}, contract.ErrInvalid
	}
	reply, err := peer.core.readConnectedPeer(ctx, source, data)
	if err != nil {
		return peercontract.Reply{}, err
	}
	return exchange.NewReply(reply), nil
}

func (core *Operations) callConnectedPeer(ctx context.Context, plan connectedSourcePlan, request exchange.RequestData) (exchange.ReplyData, error) {
	if ctx == nil {
		return exchange.ReplyData{}, contract.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return exchange.ReplyData{}, err
	}
	request.Actor = plan.actor
	if plan.authority == core.authority {
		return core.readConnectedPeer(ctx, core.authority, request)
	}
	core.owner.RLock()
	transport := core.connectedTransport
	core.owner.RUnlock()
	if transport == nil || !plan.hop.Valid() {
		return exchange.ReplyData{}, contract.ErrUnavailable
	}
	result, err := transport.ReadConnected(ctx, plan.hop, plan.authority, exchange.NewRequest(request))
	if err != nil {
		if errors.Is(err, contract.ErrOwnerMismatch) || errors.Is(err, contract.ErrStore) {
			return exchange.ReplyData{}, contract.ErrUnavailable
		}
		return exchange.ReplyData{}, err
	}
	reply := exchange.ReadReply(result)
	if reply.Authority != plan.authority {
		return exchange.ReplyData{}, contract.ErrUnavailable
	}
	if err := validateConnectedPeerReply(plan, request, reply); err != nil {
		return exchange.ReplyData{}, err
	}
	return reply, nil
}

func validateConnectedPeerReply(plan connectedSourcePlan, request exchange.RequestData, reply exchange.ReplyData) error {
	if request.Action != exchange.Probe && request.Action != exchange.Capture && request.Action != exchange.Borrow {
		return nil
	}
	facts := reply.Facts
	if !facts.Present || facts.Authority != plan.authority || !token.Valid(facts.Cut) ||
		token.FromParts(facts.Scope, facts.CutSerial) != facts.Cut || facts.Run == ([32]byte{}) || reply.Lease.Run != facts.Run || reply.Lease.ID == 0 ||
		(reply.Mode != contract.QueryCurrentVerified && reply.Mode != contract.QueryRetained) {
		return contract.ErrUnavailable
	}
	if request.Run != ([32]byte{}) && request.Run != facts.Run {
		return contract.ErrReadCutExpired
	}
	if request.Action != exchange.Probe && request.Cut != facts.Cut {
		return contract.ErrReadCutExpired
	}
	if !request.Replan && request.Grant.Scope.Valid() && !sameConnectedGrant(request.Grant, reply.Grant) {
		return contract.ErrUnavailable
	}
	if reply.Grant.Mount != request.Key || connectedContact(facts.Authority, facts.Mount, request.Key) != reply.Contact {
		return contract.ErrUnavailable
	}
	retained := request.Action != exchange.Probe && request.Mode == contract.QueryRetained
	if retained {
		if reply.Snapshot == nil || !token.ValidCoreReadSnapshot(*(reply.Snapshot)) || token.SnapshotOwner(*(reply.Snapshot)) != plan.authority ||
			token.SnapshotActor(*(reply.Snapshot)) != plan.actor || token.SnapshotRun(*(reply.Snapshot)) != facts.Run || token.SnapshotCut(*(reply.Snapshot)) != facts.Cut ||
			(request.Action != exchange.Borrow && token.SnapshotScope(*(reply.Snapshot)) != reply.Grant.Scope) ||
			(request.Action == exchange.Borrow && (request.Snapshot == nil || *request.Snapshot != *reply.Snapshot)) {
			return contract.ErrUnavailable
		}
	} else if reply.Snapshot != nil {
		return contract.ErrUnavailable
	}
	return nil
}

func (core *Operations) readConnectedPeer(ctx context.Context, source values.FactAuthorityRef, request exchange.RequestData) (result exchange.ReplyData, resultErr error) {
	if ctx == nil {
		return exchange.ReplyData{}, contract.ErrInvalid
	}
	if !request.Actor.Valid() {
		return exchange.ReplyData{}, contract.ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return exchange.ReplyData{}, err
	}
	if request.Action == exchange.End || request.Action == exchange.Check {
		return core.connectedLeaseAction(source, request)
	}
	if request.Action == exchange.Release {
		if request.Snapshot == nil {
			return exchange.ReplyData{Authority: core.authority}, nil
		}
		err := core.queries.ReleaseReadSnapshot(request.Actor, *request.Snapshot)
		return exchange.ReplyData{Authority: core.authority}, err
	}
	if request.Action != exchange.Probe && request.Action != exchange.Capture && request.Action != exchange.Borrow {
		return exchange.ReplyData{}, contract.ErrInvalid
	}
	if request.Cleanup == nil {
		return exchange.ReplyData{}, contract.ErrInvalid
	}
	grant := request.Grant
	defer func() {
		approval := contract.RealmAttachment{}
		authorized := false
		if resultErr != nil && grant.Scope.Valid() {
			granted, err := core.authorizeConnected(request.Actor, grant)
			if err != nil {
				result, resultErr = exchange.ReplyData{}, err
			} else {
				approval, authorized = granted, true
			}
		}
		if resultErr != nil && !errors.Is(resultErr, contract.ErrDenied) && token.Valid(request.Cut) {
			if err := core.checkConnectedFailureCut(request, approval, authorized); err != nil {
				result, resultErr = exchange.ReplyData{}, err
			}
		}
	}()
	var decision error
	if request.Action == exchange.Borrow && request.Replan {
		grant, decision = core.replanConnectedGrant(request)
	}
	if grant.VerifyOnly && (request.Action != exchange.Borrow || !request.Replan || !request.VerifyOnly) {
		return exchange.ReplyData{}, contract.ErrInvalid
	}
	if grant.EntryProof && (source != core.authority || request.Intent.Kind != exchange.Resolve ||
		request.Intent.Path.OriginAnchorID != request.Key.MountAnchorUUID) {
		return exchange.ReplyData{}, contract.ErrInvalid
	}
	if request.Action == exchange.Probe {
		core.owner.RLock()
		if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
			core.owner.RUnlock()
			fallback := exchange.Grant{Kind: request.Intent.Kind, Scope: request.Intent.Path.OriginAnchorID,
				Prefix: []values.ElementID{request.Intent.Path.OriginAnchorID}, Mount: request.Key}
			if request.Intent.Kind == exchange.Query {
				fallback.Prefix = nil
			}
			if request.ProofOnly {
				fallback = exchange.Grant{Scope: request.Key.MountAnchorUUID, Mount: request.Key, ProofOnly: true}
			}
			grant = fallback
			if _, err := core.authorizeConnected(request.Actor, fallback); err != nil {
				return exchange.ReplyData{}, err
			}
			return exchange.ReplyData{}, contract.ErrStore
		}
		if !grant.Scope.Valid() {
			grant, decision = connectedProbeGrant(core.owner.FactsLocked().Elements, core.owner.FactsLocked().Mount, core.owner.SourceVisibilityLocked(), request.Intent, request.Key, request.ProofOnly)
		}
		request.Cut = core.owner.CurrentCutLocked()
		core.owner.RUnlock()
	}
	approval, err := core.authorizeConnected(request.Actor, grant)
	if err != nil {
		return exchange.ReplyData{}, err
	}
	if decision != nil {
		return exchange.ReplyData{}, decision
	}
	if request.Run != ([32]byte{}) && request.Run != core.queries.Run() {
		return exchange.ReplyData{}, contract.ErrReadCutExpired
	}
	var borrow *querypkg.Lease
	if request.Action == exchange.Borrow && request.Mode == contract.QueryRetained {
		if request.Snapshot == nil {
			return exchange.ReplyData{}, contract.ErrReadCutExpired
		}
		borrow, err = core.queries.Borrow(request.Actor, *request.Snapshot)
	} else {
		mode := request.Mode
		if request.Action == exchange.Probe {
			mode = contract.QueryCurrentVerified
		}
		borrow, err = core.queries.Capture(request.Actor, grant.Scope, &request.Cut, mode)
	}
	if err != nil {
		return exchange.ReplyData{}, err
	}
	keep := false
	defer func() {
		if !keep {
			borrow.Finish()
			if request.Action == exchange.Capture && borrow.Snapshot() != nil {
				_ = core.queries.ReleaseReadSnapshot(request.Actor, *borrow.Snapshot())
			}
		}
	}()
	core.owner.RLock()
	if !core.settled(approval) {
		core.owner.RUnlock()
		return exchange.ReplyData{}, contract.ErrUnavailable
	}
	core.owner.RUnlock()
	if err := borrow.Validate(); err != nil {
		return exchange.ReplyData{}, err
	}
	if !connectedPairSupported(borrow.Projection().Mount) {
		return exchange.ReplyData{}, contract.ErrUnavailable
	}
	if request.Action != exchange.Borrow {
		if err := connectedInitialRelation(borrow.Projection().Authority, borrow.Projection().Mount, request.Key); err != nil {
			return exchange.ReplyData{}, err
		}
	}
	facts := filterConnectedFacts(borrow, grant)
	approval, err = core.authorizeConnected(request.Actor, grant)
	if err != nil {
		return exchange.ReplyData{}, err
	}
	r, err := core.connectedRegistry()
	if err != nil {
		return exchange.ReplyData{}, err
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if !core.settled(approval) {
		return exchange.ReplyData{}, contract.ErrUnavailable
	}
	if err := core.queries.CheckLocked(borrow, ""); err != nil {
		return exchange.ReplyData{}, err
	}
	r.mu.Lock()
	if r.closed || r.serial == math.MaxUint64 {
		r.mu.Unlock()
		return exchange.ReplyData{}, contract.ErrSnapshotCapacity
	}
	r.serial++
	lease := exchange.LeaseID{Run: core.queries.Run(), ID: r.serial}
	r.leases[lease.ID] = connectedLeaseEntry{borrow: borrow, actor: request.Actor, source: source, grant: cloneConnectedGrant(grant)}
	r.mu.Unlock()
	item := exchange.CleanupItem{Finish: func() {
		r.mu.Lock()
		delete(r.leases, lease.ID)
		r.mu.Unlock()
		borrow.Finish()
	}}
	if request.Action == exchange.Capture && borrow.Snapshot() != nil {
		token := *borrow.Snapshot()
		item.Discard = func() { _ = core.queries.ReleaseReadSnapshot(request.Actor, token) }
	}
	if !request.Cleanup.Add(item) {
		return exchange.ReplyData{}, contract.ErrReadCutExpired
	}
	keep = true
	return exchange.ReplyData{Authority: core.authority, Facts: facts, Grant: grant, Mode: core.queries.Mode(),
		Lease: lease, Snapshot: borrow.Snapshot(), Contact: connectedContact(borrow.Projection().Authority, borrow.Projection().Mount, request.Key)}, nil
}

func connectedInitialRelation(authority values.FactAuthorityRef, mounts mountfact.View, key contract.MountKey) error {
	if !key.Valid() {
		return nil
	}
	if authority == key.ParentAuthority {
		m, exists := mounts.Outgoing(key.MountAnchorUUID)
		if !exists || m.State != contract.MountActive || m.Key != key || !mountfact.ValidMountAdmissionKey(m.Admission) {
			return contract.ErrUnavailable
		}
	} else {
		m := mounts.Incoming()
		if m.Key != key || m.Detached || m.Gate != mountfact.GateOpen || !mountfact.ValidMountAdmissionKey(m.Admission.Key) {
			return contract.ErrUnavailable
		}
	}
	return nil
}

func connectedContact(authority values.FactAuthorityRef, mounts mountfact.View, key contract.MountKey) mountfact.Contact {
	if authority == key.ParentAuthority {
		return mounts.Record(key.MountAnchorUUID).Contact
	}
	if authority == key.CarrierAuthority {
		return mounts.Incoming().Contact
	}
	return mountfact.Contact{}
}

func (core *Operations) connectedLeaseAction(source values.FactAuthorityRef, request exchange.RequestData) (exchange.ReplyData, error) {
	if request.Action == exchange.Check && request.Grant.Scope.Valid() {
		approval, err := core.authorizeConnected(request.Actor, request.Grant)
		if err != nil {
			return exchange.ReplyData{}, err
		}
		core.owner.RLock()
		closed := core.owner.ClosedLocked()
		unsettled := !core.settled(approval)
		core.owner.RUnlock()
		if closed {
			return exchange.ReplyData{}, contract.ErrReadCutExpired
		}
		if unsettled {
			return exchange.ReplyData{}, contract.ErrUnavailable
		}
	}
	r, err := core.connectedRegistry()
	if err != nil {
		return exchange.ReplyData{}, err
	}
	r.mu.Lock()
	lease, exists := r.leases[request.Lease.ID]
	if request.Lease.Run != core.queries.Run() || !exists {
		r.mu.Unlock()
		if request.Action == exchange.End {
			return exchange.ReplyData{Authority: core.authority}, nil
		}
		return exchange.ReplyData{}, contract.ErrReadCutExpired
	}
	if lease.actor != request.Actor || lease.source != source {
		r.mu.Unlock()
		return exchange.ReplyData{}, contract.ErrInvalid
	}
	if request.Action == exchange.End {
		delete(r.leases, request.Lease.ID)
		r.mu.Unlock()
		lease.borrow.Finish()
		return exchange.ReplyData{Authority: core.authority}, nil
	}
	r.mu.Unlock()
	approval, err := core.authorizeConnected(request.Actor, lease.grant)
	if err != nil {
		return exchange.ReplyData{}, err
	}
	if request.Grant.Scope.Valid() {
		approval, err = core.authorizeConnected(request.Actor, request.Grant)
		if err != nil {
			return exchange.ReplyData{}, err
		}
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if err := core.queries.CheckLocked(lease.borrow, ""); err != nil {
		return exchange.ReplyData{}, err
	}
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return exchange.ReplyData{}, contract.ErrStore
	}
	if !core.settled(approval) {
		return exchange.ReplyData{}, contract.ErrUnavailable
	}
	if request.CheckCurrent && core.owner.CurrentCutLocked() != lease.borrow.Cut() {
		return exchange.ReplyData{}, contract.ErrReadCutExpired
	}
	if request.Key.Valid() {
		if connectedContact(core.owner.FactsLocked().Authority, core.owner.FactsLocked().Mount, request.Key) != request.Contact {
			return exchange.ReplyData{}, contract.ErrReadCutExpired
		}
		if core.authority == request.Key.ParentAuthority {
			m := core.owner.FactsLocked().Mount.Record(request.Key.MountAnchorUUID)
			if m.State != contract.MountActive || m.Key != request.Key {
				return exchange.ReplyData{}, contract.ErrUnavailable
			}
		} else if core.owner.FactsLocked().Mount.Incoming().Key != request.Key || core.owner.FactsLocked().Mount.Incoming().Detached {
			return exchange.ReplyData{}, contract.ErrUnavailable
		}
	}
	return exchange.ReplyData{Authority: core.authority}, nil
}
