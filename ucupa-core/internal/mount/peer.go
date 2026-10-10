package mount

import (
	"context"
	"errors"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
)

func (peer localMountPeer) Prepare(ctx context.Context, source values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	core := peer.core
	if core == nil || ctx == nil || !commitpkg.ValidMountRequest(request) || source != request.Key.ParentAuthority || core.owner.AuthorityRef() != request.Key.CarrierAuthority || request.Command.Kind == corecontract.MountRevoke {
		return corecontract.MountPrepared{}, corecontract.ErrInvalid
	}
	if ctx.Err() != nil {
		return corecontract.MountPrepared{}, corecontract.ErrUnavailable
	}
	key := corecontract.MountAttemptKey{ParentAuthority: source, OperationID: request.OperationID}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return corecontract.MountPrepared{}, corecontract.ErrStore
	}
	prepared, replayErr, exists := core.owner.LookupMountPrepareLocked(key, request)
	core.owner.RUnlock()
	if exists {
		return prepared, replayErr
	}
	// A proven denial stays a journaled, replayable prepare decision; only an
	// unreachable or unverifiable Realm never reaches the journal.
	authorized := true
	approval, err := core.authorize(request.CarrierActor, core.requirement(corecontract.PermissionMountAccept, request.Key.MountAnchorUUID))
	if err != nil {
		if !errors.Is(err, corecontract.ErrDenied) {
			return corecontract.MountPrepared{}, err
		}
		authorized = false
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.MountPrepared{}, corecontract.ErrStore
	}
	if authorized && !core.settled(approval) {
		return corecontract.MountPrepared{}, corecontract.ErrUnavailable
	}
	return core.prepareMountPeerLocked(source, request, authorized)
}

func (core *Operations) prepareMountPeerLocked(source values.FactAuthorityRef, request corecontract.MountPrepareRequest, authorized bool) (corecontract.MountPrepared, error) {
	return core.owner.ApplyMountPrepareLocked(source, request, authorized)
}

func (peer localMountPeer) Finish(ctx context.Context, source values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	core := peer.core
	if core == nil || ctx == nil || !commitpkg.ValidMountDecision(request.Evidence) || source != request.Evidence.Request.Key.ParentAuthority || core.owner.AuthorityRef() != request.Evidence.Request.Key.CarrierAuthority {
		return corecontract.MountClosed{}, corecontract.ErrInvalid
	}
	if ctx.Err() != nil {
		return corecontract.MountClosed{}, corecontract.ErrUnavailable
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.MountClosed{}, corecontract.ErrStore
	}
	return core.owner.ApplyMountFinishLocked(source, request)
}

func (peer localMountPeer) ReadDecision(ctx context.Context, source values.FactAuthorityRef, key corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	core := peer.core
	if core == nil || ctx == nil || !source.Valid() || !commitpkg.ValidMountAttemptKey(key) || key.ParentAuthority != core.owner.AuthorityRef() {
		return corecontract.MountDecisionEvidence{}, corecontract.ErrInvalid
	}
	if ctx.Err() != nil {
		return corecontract.MountDecisionEvidence{}, corecontract.ErrUnavailable
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.MountDecisionEvidence{}, corecontract.ErrStore
	}
	operation, exists := core.owner.MountOperationLocked(key.OperationID)
	if !exists || !operation.Request.Key.Valid() || operation.Request.Key.CarrierAuthority != source {
		return corecontract.MountDecisionEvidence{}, corecontract.ErrNotFound
	}
	return operation.Evidence(), nil
}
