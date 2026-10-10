package entry

import (
	"context"

	"ucupa/shared/values"
	accesspkg "ucupa/ucupa-core/internal/access"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

func (core *Operations) publicEnter(ctx context.Context, caller values.CallerContext, request map[string]any, federated bool) (map[string]any, error) {
	if _, err := accessvalue.Object(request, "op", "reference", "intent"); err != nil {
		return nil, err
	}
	boundary, err := accessvalue.Reference(core.owner.AuthorityRef(), request["reference"])
	if err != nil {
		return nil, err
	}
	intent, ok := request["intent"].(string)
	permissions := accessvalue.Intent(intent)
	if !ok || len(permissions) == 0 {
		return nil, values.PublicInvalid
	}
	requirements := accesspkg.ClosureRequirements(core.owner.AuthorityRef(), boundary, permissions...)
	view, permit, err := core.admission.Authorize(ctx, caller, requirements)
	defer core.admission.ReleasePermit(ctx, permit)
	if err != nil {
		return nil, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return nil, values.PublicUnavailable
	}
	if err := core.admission.Validate(ctx, caller, view, requirements, false, permit); err != nil {
		return nil, err
	}
	if _, exists := core.owner.FactsLocked().Elements.Anchor(boundary); !exists {
		return nil, values.PublicNotFound
	}
	if err := mountfact.GuardLocalList(core.owner.FactsLocked().Mount, boundary); err != nil && !federated {
		return nil, values.PublicUnavailable
	}
	entry, err := core.access.IssueSpaceLocked(caller, boundary)
	if err != nil {
		return nil, err
	}
	return map[string]any{"handle": accessvalue.TokenJSON(entry.Token), "boundary": map[string]any{"core": string(core.owner.AuthorityRef()), "anchor": string(boundary)}}, nil
}

func (core *Operations) publicClose(caller values.CallerContext, request map[string]any) (map[string]any, error) {
	if _, err := accessvalue.Object(request, "op", "handle"); err != nil {
		return nil, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	entry, err := accessresolve.SpaceLocked(core.owner, core.access, caller, request["handle"])
	if err == values.PublicInvalidContext {
		return map[string]any{"closed": true}, nil
	}
	if err != nil {
		return nil, err
	}
	core.access.RemoveLocked(entry.Ref)
	return map[string]any{"closed": true}, nil
}
