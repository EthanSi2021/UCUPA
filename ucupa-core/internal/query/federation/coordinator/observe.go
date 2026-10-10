package coordinator

import (
	"context"
	"encoding/base64"
	"strconv"

	"ucupa/shared/values"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

func (core *Operations) Observe(ctx context.Context, caller values.CallerContext, operation string, request map[string]any) (map[string]any, error) {
	kind, field := "observation-preparation", "preparation"
	if operation == "poll" || operation == "release-observe" && request["cursor"] != nil {
		kind, field = "observation-cursor", "cursor"
	}
	core.owner.Lock()
	var prep accessregistry.PreparationUse
	var cursor accessregistry.CursorUse
	var err error
	if kind == "observation-cursor" {
		cursor, err = accessresolve.CursorLocked(core.owner, core.registry, caller, request[field])
		if err == nil {
			var ok bool
			prep, ok = core.registry.PreparationLocked(cursor.Preparation)
			if !ok {
				err = values.PublicGap
			}
		}
	} else {
		prep, err = accessresolve.PreparationLocked(core.owner, core.registry, caller, request[field])
	}
	var receipt accessregistry.ReadUse
	var read *federationcollect.Read
	if err == nil && prep.HasReceipt {
		var ok bool
		receipt, ok = core.registry.ReadLocked(prep.Receipt)
		if ok {
			read = receipt.Composite
		}
	}
	core.owner.Unlock()
	if read == nil {
		return core.observations.Run(ctx, caller, operation, request)
	}
	allowed := []string{"op", field}
	if operation == "poll" {
		allowed = append(allowed, "limit")
	}
	if _, err := accessvalue.Object(request, allowed...); err != nil {
		return nil, err
	}
	if operation == "release-observe" {
		core.owner.Lock()
		core.registry.RemoveLocked(prep.Ref)
		core.owner.Unlock()
		return map[string]any{"released": true}, nil
	}
	limit, err := accessvalue.Limit(request["limit"], 64)
	if err != nil {
		return nil, err
	}
	position := uint64(0)
	if kind == "observation-cursor" {
		position = cursor.Position
	}
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(receipt.Lifetime, cancel)
	defer func() { stop(); cancel() }()
	page, lease, err := read.Observe(callCtx, operation, position, limit)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	groups := make([]any, 0, len(page.Groups))
	for _, group := range page.Groups {
		categories := make([]any, len(group.Categories))
		for i, category := range group.Categories {
			categories[i] = category
		}
		entry := map[string]any{
			"changeId": accessvalue.TokenJSON(group.ID),
			"owner":    string(group.ID.Owner),
			"category": categories,
			"basis":    accessvalue.TokenJSON(group.Basis),
		}
		if group.Terminal != nil {
			// Terminal successor evidence passes through the composite page with
			// the same public encoding as a local observation group.
			entry["terminal"] = map[string]any{
				"operationId": base64.RawURLEncoding.EncodeToString([]byte(group.Terminal.Operation)),
				"source":      string(group.Terminal.Source),
				"target":      string(group.Terminal.Target),
				"finalCut":    strconv.FormatUint(group.Terminal.FinalCut, 10),
				"settled":     group.Terminal.Settled,
				"nextHop":     string(group.Terminal.NextHop),
			}
		}
		groups = append(groups, entry)
	}
	core.owner.Lock()
	defer core.owner.Unlock()
	currentPrep, prepPresent := core.registry.PreparationLocked(prep.Ref)
	currentReceipt, receiptPresent := core.registry.ReadLocked(receipt.Ref)
	cursorPresent := true
	if kind == "observation-cursor" {
		_, cursorPresent = core.registry.CursorLocked(cursor.Ref)
	}
	if callCtx.Err() != nil || core.owner.ClosedLocked() || !prepPresent || !receiptPresent || currentReceipt.Composite != read || !cursorPresent {
		return nil, values.PublicGap
	}
	core.registry.ExpireLocked()
	currentPrep, prepPresent = core.registry.PreparationLocked(currentPrep.Ref)
	if !prepPresent {
		return nil, values.PublicGap
	}
	next, err := core.registry.CompositePositionLocked(currentPrep.Ref, operation, page.Position, page.Base)
	if err != nil {
		return nil, err
	}
	if operation == "begin-observe" {
		return map[string]any{"cursor": accessvalue.TokenJSON(next)}, nil
	}
	return map[string]any{"groups": groups, "nextCursor": accessvalue.TokenJSON(next)}, nil
}
