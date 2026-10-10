package mount

import (
	"context"
	"errors"
	"sort"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/token"
)

// One pass has at most Prepare, Decide, Finish, Settle. Unavailability never
// chooses Abort, releases a gate, or starts a second forward traversal.
func (core *Operations) driveMount(ctx context.Context, id corecontract.OperationID) (corecontract.MountControlResult, error) {
	return core.driveMountGuarded(ctx, id, nil, nil)
}

func (core *Operations) driveMountGuarded(ctx context.Context, id corecontract.OperationID, invocation corecontract.MountTransport, guard *controlAuthorization) (corecontract.MountControlResult, error) {
	operation, transport, address, err := core.mountWork(id)
	if err != nil {
		return corecontract.MountControlResult{}, err
	}
	if invocation != nil {
		transport = invocation
	}
	// A network recovery pass may finish a durable decision, but cannot turn
	// persisted subject names into a fresh authenticated pending write. Only
	// ApplyMountControl supplies the original caller/view for such a decision.
	_, network := transport.(corecontract.MountControlTransport)
	recoveryWithoutView := network && guard == nil
	if operation.Settled {
		return operation.Result(id), nil
	}
	if operation.Decision == corecontract.MountPending {
		authorized := false
		approval := corecontract.RealmAttachment{}
		if guard == nil && !recoveryWithoutView {
			// A proven denial keeps the recovery decision; an unreachable Realm
			// blocks instead of being recorded as a denial.
			if granted, err := core.authorizeMount(operation.Binding.Actor, operation.Binding.Command); err != nil {
				if !errors.Is(err, corecontract.ErrDenied) {
					return blockedMount(operation, id, err), nil
				}
			} else {
				authorized = true
				approval = granted
			}
		}
		if guard != nil || recoveryWithoutView {
			var check error = values.PublicUnavailable
			if guard != nil {
				check = guard.validate(core.admission)
			}
			if check != nil && !errors.Is(check, values.PublicDenied) && !errors.Is(check, values.PublicInvalidContext) {
				return blockedMount(operation, id, corecontract.ErrUnavailable), nil
			}
			authorized = check == nil
		}
		var prepared corecontract.MountPrepared
		if authorized {
			if ctx.Err() != nil || transport == nil {
				return blockedMount(operation, id, corecontract.ErrUnavailable), nil
			}
			prepared, err = transport.Prepare(ctx, address, operation.Request.Key.CarrierAuthority, operation.Request)
			if err != nil {
				return blockedMount(operation, id, err), nil
			}
			if prepared.Request != operation.Request || !commitpkg.ValidMountPrepared(prepared) {
				return blockedMount(operation, id, corecontract.ErrOwnerMismatch), nil
			}
			// Realm is called outside the Core lock and after the peer roundtrip.
			if guard == nil && !recoveryWithoutView {
				if granted, err := core.authorizeMount(operation.Binding.Actor, operation.Binding.Command); err != nil {
					if !errors.Is(err, corecontract.ErrDenied) {
						return blockedMount(operation, id, err), nil
					}
					authorized = false
				} else {
					approval = granted
				}
			}
		}
		if !authorized {
			prepared = corecontract.MountPrepared{}
		}
		core.owner.Lock()
		if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
			core.owner.Unlock()
			return corecontract.MountControlResult{}, corecontract.ErrStore
		}
		if guard == nil && !recoveryWithoutView && authorized && !core.settled(approval) {
			core.owner.Unlock()
			return blockedMount(operation, id, corecontract.ErrUnavailable), nil
		}
		current, _ := core.owner.MountOperationLocked(id)
		if current.Decision == corecontract.MountPending {
			if guard != nil || recoveryWithoutView {
				var check error = values.PublicUnavailable
				if guard != nil {
					check = guard.validate(core.admission)
				}
				if check != nil && !errors.Is(check, values.PublicDenied) && !errors.Is(check, values.PublicInvalidContext) {
					core.owner.Unlock()
					return blockedMount(operation, id, corecontract.ErrUnavailable), nil
				}
				authorized = check == nil
				if !authorized {
					prepared = corecontract.MountPrepared{}
				}
			}
			err = core.executeMountEventLocked(commitpkg.MountEvent{Kind: commitpkg.MountEventDecide, Operation: id, Prepared: prepared, Authorized: authorized})
		}
		core.owner.Unlock()
		if err != nil {
			return corecontract.MountControlResult{}, err
		}
	}
	operation, transport, address, err = core.mountWork(id)
	if err != nil {
		return corecontract.MountControlResult{}, err
	}
	if invocation != nil {
		transport = invocation
	}
	if operation.Settled {
		return operation.Result(id), nil
	}
	if ctx.Err() != nil || transport == nil {
		return blockedMount(operation, id, corecontract.ErrUnavailable), nil
	}
	closed, err := transport.Finish(ctx, address, operation.Request.Key.CarrierAuthority, corecontract.MountFinishRequest{Evidence: operation.Evidence()})
	if err != nil {
		return blockedMount(operation, id, err), nil
	}
	if closed.Evidence != operation.Evidence() || !token.Valid(closed.Cut) {
		return blockedMount(operation, id, corecontract.ErrOwnerMismatch), nil
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return corecontract.MountControlResult{}, corecontract.ErrStore
	}
	if current, _ := core.owner.MountOperationLocked(id); !current.Settled {
		if err := core.executeMountEventLocked(commitpkg.MountEvent{Kind: commitpkg.MountEventSettle, Operation: id, Closed: closed}); err != nil {
			return corecontract.MountControlResult{}, err
		}
	}
	settled, _ := core.owner.MountOperationLocked(id)
	return settled.Result(id), nil
}

func (core *Operations) mountWork(id corecontract.OperationID) (commitpkg.MountOperation, corecontract.MountTransport, values.NextHop, error) {
	core.owner.RLock()
	defer core.owner.RUnlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return commitpkg.MountOperation{}, nil, "", corecontract.ErrStore
	}
	operation, exists := core.owner.MountOperationLocked(id)
	if !exists {
		return commitpkg.MountOperation{}, nil, "", corecontract.ErrNotFound
	}
	contact := core.owner.FactsLocked().Mount.Record(operation.Binding.Command.MountAnchorUUID).Contact
	return operation, core.mountTransport, contact.NextHop, nil
}

func blockedMount(operation commitpkg.MountOperation, id corecontract.OperationID, err error) corecontract.MountControlResult {
	result := operation.Result(id)
	result.BlockedBy = corecontract.MountBlockedUnavailable
	if errors.Is(err, corecontract.ErrOwnerMismatch) {
		result.BlockedBy = corecontract.MountBlockedOwnerMismatch
	}
	if errors.Is(err, corecontract.ErrStore) {
		result.BlockedBy = corecontract.MountBlockedPeerStore
	}
	return result
}

func (core *Operations) RecoverMounts(ctx context.Context) (corecontract.MountRecoveryReport, error) {
	if core == nil || ctx == nil {
		return corecontract.MountRecoveryReport{}, corecontract.ErrInvalid
	}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		return corecontract.MountRecoveryReport{}, corecontract.ErrStore
	}
	operations, attempts := core.owner.PendingMountsLocked()
	core.owner.RUnlock()
	sort.Slice(operations, func(i, j int) bool { return operations[i] < operations[j] })
	sort.Slice(attempts, func(i, j int) bool {
		if attempts[i].ParentAuthority != attempts[j].ParentAuthority {
			return attempts[i].ParentAuthority < attempts[j].ParentAuthority
		}
		return attempts[i].OperationID < attempts[j].OperationID
	})
	report := corecontract.MountRecoveryReport{}
	for _, id := range operations {
		result, err := core.driveMount(ctx, id)
		report.Items = append(report.Items, corecontract.MountRecoveryItem{Attempt: corecontract.MountAttemptKey{ParentAuthority: core.owner.AuthorityRef(), OperationID: id}, Result: result, Err: err})
	}
	for _, key := range attempts {
		report.Items = append(report.Items, core.recoverPeerAttempt(ctx, key))
	}
	return report, nil
}

func (core *Operations) recoverPeerAttempt(ctx context.Context, key corecontract.MountAttemptKey) corecontract.MountRecoveryItem {
	item := corecontract.MountRecoveryItem{Attempt: key}
	core.owner.RLock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		core.owner.RUnlock()
		item.Err = corecontract.ErrStore
		return item
	}
	attempt, _ := core.owner.PeerAttemptLocked(key)
	incoming, transport := core.owner.FactsLocked().Mount.Incoming(), core.mountTransport
	core.owner.RUnlock()
	if attempt.IsClosed {
		return item
	}
	if transport == nil || ctx.Err() != nil || incoming.Key != attempt.Request.Key {
		item.Err = corecontract.ErrUnavailable
		return item
	}
	evidence, err := transport.ReadDecision(ctx, incoming.Contact.NextHop, key.ParentAuthority, key)
	if err != nil {
		item.Err = err
		return item
	}
	if evidence.Request != attempt.Request {
		item.Err = corecontract.ErrOwnerMismatch
		return item
	}
	if evidence.Decision == corecontract.MountPending {
		item.Result.Decision = corecontract.MountPending
		return item
	}
	_, item.Err = core.MountPeer().Finish(ctx, key.ParentAuthority, corecontract.MountFinishRequest{Evidence: evidence})
	return item
}
