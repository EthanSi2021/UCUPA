package observation

import (
	"context"
	"encoding/base64"
	"strconv"

	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/access"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	accessvalue "ucupa/ucupa-core/internal/access/value"
)

// Run captures the retained observation authorization outside the lock,
// rechecks it online, and runs the operation under the lock guarded only by
// the permit that recheck minted. Release is idempotent, proves no authority
// and never waits online.
func (core *Operations) Run(ctx context.Context, caller values.CallerContext, operation string, request map[string]any) (map[string]any, error) {
	if operation == "release-observe" {
		core.owner.Lock()
		defer core.owner.Unlock()
		return core.RunLocked(ctx, caller, operation, request, nil)
	}
	core.owner.Lock()
	prep, _, err := core.observationUse(caller, operation, request)
	core.owner.Unlock()
	if err != nil {
		return nil, err
	}
	permit, err := core.admission.Recheck(ctx, caller, prep.View, prep.Requirements, true)
	if err != nil {
		// A denial or changed basis permanently gaps the same preparation,
		// exactly as the original in-lock failure did; a temporarily
		// unavailable Realm keeps it retryable.
		if err != values.PublicUnavailable {
			core.owner.Lock()
			if current, _, e := core.observationUse(caller, operation, request); e == nil && current.Ref == prep.Ref {
				core.registry.GapLocked(prep.Ref)
			}
			core.owner.Unlock()
		}
		return nil, err
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	return core.RunLocked(ctx, caller, operation, request, permit)
}

// Shared by public callers and authenticated federation source access. The
// source binding and observation operation are checked under one owner lock.
// permit carries the online recheck evidence minted outside the lock; the
// in-lock steps consume only that permit.
func (core *Operations) RunLocked(ctx context.Context, caller values.CallerContext, operation string, request map[string]any, permit access.Permit) (map[string]any, error) {
	prep, cursor, err := core.observationUse(caller, operation, request)
	if err != nil {
		if operation == "release-observe" && err == values.PublicGap {
			return map[string]any{"released": true}, nil
		}
		return nil, err
	}
	if operation == "release-observe" {
		core.registry.RemoveLocked(prep.Ref)
		return map[string]any{"released": true}, nil
	}
	if !prep.HasQueue || prep.Gap {
		return nil, values.PublicGap
	}
	if err := core.admission.Validate(ctx, caller, prep.View, prep.Requirements, true, permit); err != nil {
		if err != values.PublicUnavailable {
			core.registry.GapLocked(prep.Ref)
		}
		return nil, err
	}
	if source, ok := core.registry.ReadLocked(prep.Source); ok && source.Source.Present {
		if source.Source.Request.Intent == "read" && prep.Selection.Exact == "" && len(prep.Selection.Path) == 0 {
			return nil, values.PublicInvalidContext
		}
		if err := core.registry.CheckSourceRelationsLocked(source.Ref, core.owner.FactsLocked().Mount, core.owner.ParentReadVisibilityLocked()); err != nil {
			core.registry.GapLocked(prep.Ref)
			return nil, values.PublicGap
		}
	}
	if operation == "begin-observe" {
		token, err := core.registry.BeginLocalLocked(prep.Ref)
		if err != nil {
			return nil, err
		}
		return map[string]any{"cursor": accessvalue.TokenJSON(token)}, nil
	}
	if !prep.Active || cursor.Position < prep.Base {
		return nil, values.PublicGap
	}
	limit, err := accessvalue.Limit(request["limit"], 64)
	if err != nil {
		return nil, err
	}
	groups, next, err := core.registry.PollLocalLocked(cursor.Ref, limit)
	if err != nil {
		return nil, err
	}
	encoded := make([]any, 0, len(groups))
	for _, group := range groups {
		categories := make([]any, len(group.Categories))
		for i, category := range group.Categories {
			categories[i] = category
		}
		entry := map[string]any{
			"changeId": accessvalue.TokenJSON(group.ID), "owner": string(core.owner.AuthorityRef()),
			"category": categories, "basis": accessvalue.TokenJSON(group.Basis),
		}
		if group.Terminal != nil {
			// Terminal successor evidence is disclosed once with the group; the
			// serial is a decimal string and the operation id a bounded byte
			// string, matching the public encoding bridges.
			entry["terminal"] = map[string]any{
				"operationId": base64.RawURLEncoding.EncodeToString([]byte(group.Terminal.Operation)),
				"source":      string(group.Terminal.Source),
				"target":      string(group.Terminal.Target),
				"finalCut":    strconv.FormatUint(group.Terminal.FinalCut, 10),
				"settled":     group.Terminal.Settled,
				"nextHop":     string(group.Terminal.NextHop),
			}
		}
		encoded = append(encoded, entry)
	}
	return map[string]any{"groups": encoded, "nextCursor": accessvalue.TokenJSON(next)}, nil
}

// observationUse validates the request shape and resolves the observation
// entry one operation will consume. It is shared by the pre-lock capture and
// the in-lock run so both see exactly the same request and token; it performs
// no authorization.
func (core *Operations) observationUse(caller values.CallerContext, operation string, request map[string]any) (accessregistry.PreparationUse, accessregistry.CursorUse, error) {
	allowed := []string{"op", "preparation"}
	kind, field := "observation-preparation", "preparation"
	if operation == "poll" {
		allowed = []string{"op", "cursor", "limit"}
		kind, field = "observation-cursor", "cursor"
	}
	if operation == "release-observe" {
		allowed = []string{"op", "preparation", "cursor"}
		if _, ok := request["cursor"]; ok {
			kind, field = "observation-cursor", "cursor"
		}
		if len(request) != 2 {
			return accessregistry.PreparationUse{}, accessregistry.CursorUse{}, values.PublicInvalid
		}
	}
	if _, err := accessvalue.Object(request, allowed...); err != nil {
		return accessregistry.PreparationUse{}, accessregistry.CursorUse{}, err
	}
	if kind == "observation-cursor" {
		cursor, err := accessresolve.CursorLocked(core.owner, core.registry, caller, request[field])
		if err != nil {
			return accessregistry.PreparationUse{}, accessregistry.CursorUse{}, err
		}
		prep, ok := core.registry.PreparationLocked(cursor.Preparation)
		if !ok {
			return accessregistry.PreparationUse{}, accessregistry.CursorUse{}, values.PublicGap
		}
		return prep, cursor, nil
	}
	prep, err := accessresolve.PreparationLocked(core.owner, core.registry, caller, request[field])
	return prep, accessregistry.CursorUse{}, err
}
