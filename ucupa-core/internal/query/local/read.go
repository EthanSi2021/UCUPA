package local

import (
	"context"
	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	elementread "ucupa/ucupa-core/internal/element/read"
	querypkg "ucupa/ucupa-core/internal/query"
)

func (core *Operations) Read(ctx context.Context, caller values.CallerContext, operation string, request map[string]any) (map[string]any, error) {
	allowed := []string{"op", "handle", "prepareObserve"}
	switch operation {
	case "read":
		allowed = append(allowed, "address", "basis")
	case "list":
		allowed = append(allowed, "address", "limit", "continuation")
	case "query":
		allowed = append(allowed, "start", "basis", "upDepth", "sphere", "selector", "limit", "continuation")
	}
	if _, err := accessvalue.Object(request, allowed...); err != nil {
		return nil, err
	}
	prepare := false
	if raw, exists := request["prepareObserve"]; exists {
		var ok bool
		prepare, ok = raw.(bool)
		if !ok {
			return nil, values.PublicInvalid
		}
	}
	if operation == "read" && request["basis"] != nil && prepare {
		return nil, values.PublicInvalid
	}
	limit, err := accessvalue.Limit(request["limit"], 64)
	if err != nil {
		return nil, err
	}
	core.owner.Lock()
	handle, err := accessresolve.SpaceLocked(core.owner, core.access, caller, request["handle"])
	core.owner.Unlock()
	if err != nil {
		return nil, err
	}
	permissions := accessvalue.Intent(operation)
	if prepare {
		permissions = append(permissions, corecontract.PermissionObserveElement)
	}
	requirements := accesspkg.ClosureRequirements(core.owner.AuthorityRef(), handle.Boundary, permissions...)
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	retainedView := false
	defer func() {
		if !retainedView {
			core.admission.ReleasePermit(ctx, permit)
		}
	}()
	if err != nil {
		return nil, err
	}
	initial := request["continuation"] == nil
	conditions := accessvalue.ReadConditions(request)
	hasBasis := operation == "read" && request["basis"] != nil
	// Retained authorization bases (a continuation receipt, a basis chain) are
	// captured under the lock without validation, rechecked online outside it,
	// and only then re-verified and guarded under the final lock.
	var receipt accessregistry.ReadUse
	var preparation accessregistry.PreparationUse
	var after values.ElementID
	var basisChain []accessregistry.ReadUse
	if !initial || hasBasis {
		core.owner.Lock()
		if _, err := accessresolve.SpaceLocked(core.owner, core.access, caller, request["handle"]); err != nil {
			core.owner.Unlock()
			return nil, err
		}
		if _, exists := core.owner.FactsLocked().Elements.Anchor(handle.Boundary); !exists {
			core.owner.Unlock()
			return nil, values.PublicInvalidContext
		}
		if !initial {
			continuation, e := accessresolve.ContinuationLocked(core.owner, core.access, caller, request["continuation"])
			if e != nil {
				core.owner.Unlock()
				return nil, e
			}
			var present bool
			receipt, present = core.access.ReadLocked(continuation.Receipt)
			if !present || receipt.Conditions != conditions || receipt.Parent != handle.Ref {
				core.owner.Unlock()
				return nil, values.PublicInvalid
			}
			after = continuation.Last
			if receipt.HasPreparation {
				preparation, present = core.access.PreparationLocked(receipt.Preparation)
				if !present || preparation.Gap {
					core.owner.Unlock()
					return nil, values.PublicGap
				}
			}
		}
		if hasBasis {
			basisChain, err = core.resolveBasisChainLocked(caller, request["basis"], handle.Ref)
			if err != nil {
				core.owner.Unlock()
				return nil, err
			}
		}
		core.owner.Unlock()
	}
	var contPermit accesspkg.Permit
	var basisPermits []accesspkg.Permit
	if !initial {
		contPermit, err = core.admission.Recheck(ctx, caller, receipt.View, receipt.Requirements, false)
		if err != nil {
			return nil, err
		}
	}
	if hasBasis {
		basisPermits = make([]accesspkg.Permit, len(basisChain))
		for i, entry := range basisChain {
			basisPermits[i], err = core.admission.Recheck(ctx, caller, entry.View, entry.Requirements, false)
			if err != nil {
				return nil, err
			}
		}
	}
	keep := false
	defer func() {
		if initial && !keep {
			core.owner.Lock()
			if receipt.Ref != (accessregistry.Ref{}) {
				core.access.RemoveLocked(receipt.Ref)
			}
			if preparation.Ref != (accessregistry.Ref{}) {
				core.access.RemoveLocked(preparation.Ref)
			}
			core.owner.Unlock()
		}
	}()
	// Re-verify the captured state under the lock, guard with the permits
	// minted above, and only then issue, bind or prepare.
	core.owner.Lock()
	if _, err := accessresolve.SpaceLocked(core.owner, core.access, caller, request["handle"]); err != nil {
		core.owner.Unlock()
		return nil, err
	}
	if _, exists := core.owner.FactsLocked().Elements.Anchor(handle.Boundary); !exists {
		core.owner.Unlock()
		return nil, values.PublicInvalidContext
	}
	if !initial {
		continuation, e := accessresolve.ContinuationLocked(core.owner, core.access, caller, request["continuation"])
		if e != nil {
			core.owner.Unlock()
			return nil, e
		}
		current, present := core.access.ReadLocked(continuation.Receipt)
		if !present || current.Ref != receipt.Ref || current.View != receipt.View || !accesspkg.SameRequirements(current.Requirements, receipt.Requirements) ||
			current.Conditions != conditions || current.Parent != handle.Ref {
			core.owner.Unlock()
			return nil, values.PublicInvalid
		}
		if err := core.admission.Validate(ctx, caller, current.View, current.Requirements, false, contPermit); err != nil {
			core.owner.Unlock()
			return nil, err
		}
		receipt, after = current, continuation.Last
		if receipt.HasPreparation {
			preparation, present = core.access.PreparationLocked(receipt.Preparation)
			if !present || preparation.Gap {
				core.owner.Unlock()
				return nil, values.PublicGap
			}
		}
	} else {
		receipt, err = core.access.IssueLocalReadLocked(caller, handle.Boundary, handle.Ref, core.owner.CurrentCutLocked(), view, requirements, operation, conditions)
		if err != nil {
			core.owner.Unlock()
			return nil, err
		}
		retainedView = core.access.OwnAuthorizationReleaseLocked(receipt.Ref, core.admission.PermitRelease(permit))
	}
	var basisReceipt accessregistry.ReadUse
	if hasBasis {
		chain, e := core.resolveBasisChainLocked(caller, request["basis"], handle.Ref)
		if e != nil {
			core.owner.Unlock()
			return nil, e
		}
		if len(chain) != len(basisChain) {
			core.owner.Unlock()
			return nil, values.PublicInvalid
		}
		for i := range chain {
			if chain[i].Ref != basisChain[i].Ref || chain[i].View != basisChain[i].View || !accesspkg.SameRequirements(chain[i].Requirements, basisChain[i].Requirements) {
				core.owner.Unlock()
				return nil, values.PublicInvalid
			}
			if e := core.admission.Validate(ctx, caller, chain[i].View, chain[i].Requirements, false, basisPermits[i]); e != nil {
				core.owner.Unlock()
				return nil, e
			}
		}
		var ok bool
		receipt, ok = core.access.BindBasisLocked(receipt.Ref, chain[0].Ref)
		if !ok {
			core.owner.Unlock()
			return nil, values.PublicInvalidContext
		}
		// A derived exact read borrows the originating root basis's retained
		// snapshot; it never owns or silently releases another receipt's one.
		basisReceipt = chain[len(chain)-1]
	}
	if initial && prepare {
		// Resolve exact observation against the very cut being registered.
		var exact values.ElementID
		if operation == "read" {
			exact, err = publicAddressForViews(core.owner.FactsLocked().Elements, core.owner.FactsLocked().Mount, core.owner.SourceVisibilityLocked(), handle.Boundary, request["address"])
			if err != nil {
				core.owner.Unlock()
				return nil, err
			}
		}
		preparation, err = core.access.PrepareLocalLocked(handle.Ref, requirements, view, core.owner.CurrentCutLocked(), exact, operation == "read")
		if err != nil {
			core.owner.Unlock()
			return nil, err
		}
		if !core.access.LinkPreparationLocked(receipt.Ref, preparation.Ref) {
			core.owner.Unlock()
			return nil, values.PublicInvalidContext
		}
	}
	cut := receipt.Cut
	signer := core.owner.SigningLocked()
	core.owner.Unlock()
	var borrow *querypkg.Lease
	if hasBasis && basisReceipt.Snapshot != nil {
		borrow, err = core.queries.Borrow(caller.Subject, *basisReceipt.Snapshot)
	} else if receipt.Snapshot != nil {
		borrow, err = core.queries.Borrow(caller.Subject, *receipt.Snapshot)
	} else {
		mode := corecontract.QueryCurrentVerified
		if initial && operation != "read" {
			mode = core.queries.Mode()
		}
		borrow, err = core.queries.Capture(caller.Subject, handle.Boundary, &cut, mode)
	}
	if err != nil {
		return nil, err
	}
	defer borrow.Finish()
	if initial && operation != "read" && borrow.Snapshot() != nil {
		core.owner.Lock()
		if current, present := core.access.ReadLocked(receipt.Ref); !present || current.Ref != receipt.Ref {
			core.owner.Unlock()
			_ = core.queries.ReleaseReadSnapshot(caller.Subject, *borrow.Snapshot())
			return nil, values.PublicInvalidContext
		}
		if !core.access.InstallSnapshotLocked(receipt.Ref, borrow.Snapshot()) {
			core.owner.Unlock()
			_ = core.queries.ReleaseReadSnapshot(caller.Subject, *borrow.Snapshot())
			return nil, values.PublicInvalidContext
		}
		receipt, _ = core.access.ReadLocked(receipt.Ref)
		core.owner.Unlock()
	}
	projection := borrow.Projection()
	var result map[string]any
	var last values.ElementID
	var exact values.ElementID
	switch operation {
	case "read":
		var id values.ElementID
		id, err = publicAddressForViews(projection.Elements, projection.Mount, projection.Transfers, handle.Boundary, request["address"])
		exact = id
		if err == nil {
			var read corecontract.ElementRead
			read, err = elementread.Element(projection.Authority, projection.Elements, projection.Attachments, projection.Transfers, id, cut)
			if err == nil {
				result = accessvalue.ElementReadJSON(signer, read)
			}
		}
	case "list":
		result, last, err = publicListJSON(projection, signer, handle.Boundary, request["address"], after, limit)
	case "query":
		result, last, err = publicQueryJSON(projection, signer, handle.Boundary, request, after, limit)
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if err := core.queries.CheckLocked(borrow, exact); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	core.access.ExpireLocked()
	currentReceipt, present := core.access.ReadLocked(receipt.Ref)
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() || !present || currentReceipt.Ref != receipt.Ref {
		return nil, values.PublicInvalidContext
	}
	if borrow.Mode() == corecontract.QueryCurrentVerified && core.owner.CurrentCutLocked() != borrow.Cut() {
		return nil, values.PublicInvalidContext
	}
	if borrow.Mode() == corecontract.QueryRetained {
		if !core.queries.SnapshotPresent(borrow.Snapshot()) {
			return nil, values.PublicInvalidContext
		}
	}
	receiptPermit := permit
	if !initial {
		receiptPermit = contPermit
	}
	if err := core.admission.Validate(ctx, caller, currentReceipt.View, currentReceipt.Requirements, false, receiptPermit); err != nil {
		return nil, err
	}
	if preparation.Ref != (accessregistry.Ref{}) {
		currentPreparation, ok := core.access.PreparationLocked(preparation.Ref)
		if !ok || currentPreparation.Gap {
			return nil, values.PublicGap
		}
	}
	if ctx.Err() != nil {
		return nil, values.PublicUnavailable
	}
	result["receipt"] = accessvalue.TokenJSON(receipt.Token)
	result["readMode"] = "CURRENT_VERIFIED"
	if borrow.Mode() == corecontract.QueryRetained {
		result["readMode"] = "RETAINED"
	}
	if preparation.Ref != (accessregistry.Ref{}) {
		result["preparation"] = accessvalue.TokenJSON(preparation.Token)
	}
	if last != "" {
		continuation, err := core.access.IssueContinuationLocked(receipt.Ref, last, "")
		if err != nil {
			return nil, err
		}
		result["continuation"] = accessvalue.TokenJSON(continuation.Token)
	}
	keep = true
	return result, nil
}

// resolveBasisChainLocked resolves the caller's basis receipt and its retained
// basis ancestors without validating any retained view. The caller's online
// recheck runs outside the lock; the final locked pass re-resolves, compares
// and guards this captured chain.
func (core *Operations) resolveBasisChainLocked(caller values.CallerContext, raw any, parent accessregistry.Ref) ([]accessregistry.ReadUse, error) {
	first, err := accessresolve.ReadLocked(core.owner, core.access, caller, raw)
	if err != nil {
		return nil, err
	}
	if first.Parent != parent {
		return nil, values.PublicInvalid
	}
	chain := []accessregistry.ReadUse{first}
	current := first
	for current.HasBasis {
		next, present := core.access.ReadLocked(current.Basis)
		if !present {
			return nil, values.PublicInvalidContext
		}
		chain = append(chain, next)
		current = next
	}
	return chain, nil
}
