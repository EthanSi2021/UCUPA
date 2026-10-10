package coordinator

import (
	"context"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	"ucupa/ucupa-core/internal/geometry"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

// federatedBasisChainLocked walks the exact basis chain. With captured nil it
// only parses entries; with a captured walk and its permits, each entry must
// still be the same Ref, view and requirement set, and each non-terminal view
// is guarded only by the permit its outside-lock recheck minted.
func (core *Operations) federatedBasisChainLocked(ctx context.Context, caller values.CallerContext, raw any, handle accessregistry.SpaceUse, captured []accessregistry.ReadUse, permits []accesspkg.Permit) ([]accessregistry.ReadUse, error) {
	verify := len(captured) > 0
	if verify && len(permits) != len(captured)-1 {
		return nil, values.PublicInvalidContext
	}
	chain := make([]accessregistry.ReadUse, 0)
	for i := 0; ; i++ {
		entry, err := accessresolve.ReadLocked(core.owner, core.registry, caller, raw)
		if err != nil {
			return nil, err
		}
		if entry.Parent != handle.Ref || len(chain) >= geometry.MaxExactProjectionEdges {
			return nil, values.PublicInvalidContext
		}
		if verify && (i >= len(captured) || entry.Ref != captured[i].Ref || entry.View != captured[i].View || !accesspkg.SameRequirements(entry.Requirements, captured[i].Requirements)) {
			return nil, values.PublicInvalidContext
		}
		chain = append(chain, entry)
		if entry.Composite != nil {
			if verify && i != len(captured)-1 {
				return nil, values.PublicInvalidContext
			}
			return chain, nil
		}
		if !entry.HasBasis || entry.Exact == nil {
			return nil, values.PublicInvalidContext
		}
		if verify {
			if err := core.admission.Validate(ctx, caller, entry.View, entry.Requirements, false, permits[i]); err != nil {
				return nil, err
			}
		}
		parent, present := core.registry.ReadLocked(entry.Basis)
		if !present {
			return nil, values.PublicInvalidContext
		}
		raw = accessvalue.TokenJSON(parent.Token)
	}
}

func (core *Operations) readAt(ctx context.Context, caller values.CallerContext, request map[string]any) (map[string]any, error) {
	core.owner.Lock()
	handle, err := accessresolve.SpaceLocked(core.owner, core.registry, caller, request["handle"])
	var chain []accessregistry.ReadUse
	if err == nil {
		chain, err = core.federatedBasisChainLocked(ctx, caller, request["basis"], handle, nil, nil)
	}
	var read *federationcollect.Read
	if err == nil {
		read = chain[len(chain)-1].Composite
	}
	core.owner.Unlock()
	if err != nil {
		return nil, err
	}
	requirements := accesspkg.ClosureRequirements(core.owner.AuthorityRef(), handle.Boundary, corecontract.PermissionReadElement)
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
	// Every retained basis view is rechecked online outside the fact lock; the
	// verify walks below consume only the permits that recheck minted.
	permits := make([]accesspkg.Permit, 0, len(chain)-1)
	for i := 0; i < len(chain)-1; i++ {
		entry, err := core.admission.Recheck(ctx, caller, chain[i].View, chain[i].Requirements, false)
		if err != nil {
			return nil, err
		}
		permits = append(permits, entry)
	}
	core.owner.Lock()
	_, err = core.federatedBasisChainLocked(ctx, caller, request["basis"], handle, chain, permits)
	var receipt accessregistry.ReadUse
	if err == nil {
		receipt, err = core.registry.IssueDerivedReadLocked(caller, handle.Boundary, handle.Ref, chain[0].Ref, view, requirements)
		if err == nil {
			retainedView = core.registry.OwnAuthorizationReleaseLocked(receipt.Ref, core.admission.PermitRelease(permit))
		}
	}
	core.owner.Unlock()
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			core.owner.Lock()
			core.registry.RemoveLocked(receipt.Ref)
			core.owner.Unlock()
		}
	}()
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(receipt.Lifetime, cancel)
	defer func() { stop(); cancel() }()
	address, err := decodeFederatedAddress(request["address"], handle.Boundary)
	if err != nil {
		return nil, err
	}
	prior := make([]*federationcollect.ExactBasis, 0, len(chain)-1)
	for _, entry := range chain {
		if entry.Exact != nil {
			prior = append(prior, entry.Exact)
		}
	}
	result, basis, lease, err := read.ReadAt(callCtx, address, prior)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	encoded := map[string]any{
		"state":       "PRESENT",
		"record":      federatedRecordJSON(result.Record),
		"attachment":  federatedAttachmentJSON(*result.Record.Attachment),
		"closure":     values.EncodePublicBytes(result.Closure[:]),
		"sourceGraph": federatedSourceGraphJSON(result.SourceGraph),
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if current, present := core.registry.ReadLocked(receipt.Ref); callCtx.Err() != nil || !present || current.Ref != receipt.Ref {
		return nil, values.PublicInvalidContext
	}
	if _, err := core.federatedBasisChainLocked(ctx, caller, request["basis"], handle, chain, permits); err != nil {
		return nil, err
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return nil, err
	}
	if !core.registry.InstallExactLocked(receipt.Ref, basis) {
		return nil, values.PublicInvalidContext
	}
	encoded["receipt"] = accessvalue.TokenJSON(receipt.Token)
	encoded["readMode"] = "CURRENT_VERIFIED"
	if result.ReadMode == corecontract.QueryRetained {
		encoded["readMode"] = "RETAINED"
	}
	keep = true
	return encoded, nil
}
