package namespace

import (
	"context"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
)

func (f *Owner) publicObserve(ctx context.Context, caller values.CallerContext, operation string, request map[string]any) (map[string]any, error) {
	kind, field := "observation-preparation", "preparation"
	allowed := []string{"op", "preparation"}
	if operation == "poll" {
		kind, field = "observation-cursor", "cursor"
		allowed = []string{"op", "cursor", "limit"}
	}
	if operation == "release-observe" {
		allowed = []string{"op", "preparation", "cursor"}
		if _, ok := request["cursor"]; ok {
			kind, field = "observation-cursor", "cursor"
		}
		if len(request) != 2 {
			return nil, values.PublicInvalid
		}
	}
	if _, err := requestvalue.Object(request, allowed...); err != nil {
		return nil, err
	}
	limit := 64
	if raw, exists := request["limit"]; exists {
		n, ok := raw.(float64)
		if !ok || n < 1 || n > 256 || float64(int(n)) != n {
			return nil, values.PublicInvalid
		}
		limit = int(n)
	}
	f.mu.Lock()
	entry, err := f.publicContextLocked(caller, request[field], kind)
	if err != nil {
		f.mu.Unlock()
		if operation == "release-observe" && err == values.PublicGap {
			return map[string]any{"released": true}, nil
		}
		return nil, err
	}
	prep := entry
	if kind == "observation-cursor" {
		prep = f.public.LookupLocked(entry.Parent)
	}
	if prep == nil || prep.Stream == nil {
		f.mu.Unlock()
		return nil, values.PublicGap
	}
	s := prep.Stream
	address, key, controlPrep := s.Address, prep.Key, s.CorePreparation
	if operation == "release-observe" {
		f.public.RemoveLocked(prep.Token.Value)
		f.mu.Unlock()
		f.releaseControl(ctx, address, key, controlPrep)
		return map[string]any{"released": true}, nil
	}
	if s.Gap {
		f.mu.Unlock()
		return nil, values.PublicGap
	}
	view, requirements := prep.View, append([]values.Requirement(nil), prep.Requirements...)
	f.mu.Unlock()
	permit, checkErr := f.recheckPublic(ctx, caller, view, requirements, true)
	f.mu.Lock()
	f.public.ExpireLocked()
	if f.public.LookupLocked(prep.Token.Value) != prep || s.Gap || f.public.LookupLocked(entry.Token.Value) != entry {
		f.mu.Unlock()
		return nil, values.PublicGap
	}
	if checkErr == nil {
		checkErr = permit.guardLocked(ctx, true)
	}
	if err := checkErr; err != nil {
		if err != values.PublicUnavailable {
			f.public.GapLocked(s)
		}
		f.mu.Unlock()
		if err != values.PublicUnavailable {
			f.releaseControl(ctx, address, key, controlPrep)
		}
		return nil, err
	}
	if current, ok := f.state.Contacts[packKey(key).id()]; !ok || (current.Address != s.Address || current.Serial != s.ContactSerial) {
		f.public.GapLocked(s)
		f.mu.Unlock()
		f.releaseControl(ctx, address, key, controlPrep)
		return nil, values.PublicUnavailable
	}
	coreCursor := s.CoreCursor
	f.mu.Unlock()
	source, ok := f.cores.(coreControlSource)
	if !ok {
		return nil, values.PublicUnavailable
	}
	control, err := source.ReadControl(ctx, address, key, false)
	if err != nil {
		err = facetControlError(err)
		if err == values.PublicGap {
			f.mu.Lock()
			f.public.GapLocked(s)
			f.mu.Unlock()
		}
		return nil, err
	}
	if !packProof(control.Evidence).valid(packKey(key), false) || coreRejection(packProof(control.Evidence), packKey(key)) != "" || control.Proposal != s.Proposal {
		f.mu.Lock()
		f.public.GapLocked(s)
		f.mu.Unlock()
		f.releaseControl(ctx, address, key, controlPrep)
		return nil, values.PublicUnavailable
	}
	if !coreCursor.Valid() {
		begun, err := f.controlCall(ctx, address, key, map[string]any{"op": "begin-observe", "preparation": requestvalue.TokenJSON(controlPrep)})
		if err != nil {
			return nil, err
		}
		coreCursor, err = requestvalue.Token(begun["cursor"], "observation-cursor")
		if err != nil || coreCursor.Owner != key.Core {
			return nil, values.PublicUnavailable
		}
	}
	polled, err := f.controlCall(ctx, address, key, map[string]any{"op": "poll", "cursor": requestvalue.TokenJSON(coreCursor)})
	if err != nil {
		return nil, err
	}
	groups, ok := polled["groups"].([]any)
	if !ok {
		return nil, values.PublicUnavailable
	}
	if len(groups) > 0 {
		f.mu.Lock()
		f.public.GapLocked(s)
		f.mu.Unlock()
		f.releaseControl(ctx, address, key, controlPrep)
		return nil, values.PublicGap
	}
	nextControl, err := requestvalue.Token(polled["nextCursor"], "observation-cursor")
	if err != nil || nextControl != coreCursor {
		return nil, values.PublicUnavailable
	}
	if err := permit.recheck(ctx, true); err != nil {
		if err != values.PublicUnavailable {
			f.mu.Lock()
			f.public.GapLocked(s)
			f.mu.Unlock()
		}
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.public.ExpireLocked()
	if f.public.LookupLocked(prep.Token.Value) != prep || s.Gap || f.public.LookupLocked(entry.Token.Value) != entry {
		return nil, values.PublicGap
	}
	if err := f.publicAvailableLocked(); err != nil {
		return nil, err
	}
	if err := permit.guardLocked(ctx, true); err != nil {
		if err != values.PublicUnavailable {
			f.public.GapLocked(s)
		}
		return nil, err
	}
	if current := f.state.Contacts[packKey(key).id()]; current.Address != s.Address || current.Serial != s.ContactSerial {
		f.public.GapLocked(s)
		return nil, values.PublicGap
	}
	s.CoreCursor = coreCursor
	if operation == "begin-observe" {
		if !s.Active {
			cursor, err := f.issuePublicLocked(caller, "observation-cursor", prep)
			if err != nil {
				return nil, err
			}
			cursor.Stream = s
			s.Initial = cursor.Token
			s.Active = true
		}
		return map[string]any{"cursor": requestvalue.TokenJSON(s.Initial)}, nil
	}
	page, err := f.public.PageLocked(entry, prep, limit)
	if err != nil {
		return nil, err
	}
	next := page.Next
	if page.NeedsCursor {
		cursor, err := f.issuePublicLocked(caller, "observation-cursor", prep)
		if err != nil {
			return nil, err
		}
		cursor.Stream, cursor.Position = s, page.Position
		next = cursor.Token
	}
	return map[string]any{"groups": page.Groups, "nextCursor": requestvalue.TokenJSON(next)}, nil
}

func (f *Owner) projectPublicChangesLocked(c command, out outcome) {
	if f.public == nil || out.Status != facetcontract.Committed {
		return
	}
	category := "contact"
	if c.Kind == "put" || c.Kind == "delete" {
		category = "content"
	} else if c.Kind == "accept" || c.Kind == "retire" {
		category = "attachment"
	}
	f.public.PublishLocked(c.Key.Element, c.Type, category)
}
