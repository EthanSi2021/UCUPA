package access

import (
	"ucupa/shared/values"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
)

// Page is the local part of one public observation poll. When NeedsCursor is
// true, Position is the position for the namespace owner to attach to a new
// observation-cursor context after its normal issue/availability checks.
type Page struct {
	Groups      []any
	Next        values.PublicToken
	Position    uint64
	NeedsCursor bool
}

// PublishLocked appends one already-adjudicated committed change to matching
// preparation streams.  Namespace/commit decides whether the change is
// committed and supplies its category; this method never calls back into the
// owner for facts.
func (m *Manager) PublishLocked(element values.ElementID, typ []byte, category string) {
	if m == nil || m.closed {
		return
	}
	m.ExpireLocked()
	for _, prep := range m.contexts {
		if prep.Token.Kind != "observation-preparation" || prep.Stream == nil || prep.Stream.Gap || prep.Key.Element != element {
			continue
		}
		if category == "content" && prep.Typ.Valid() && string(prep.Typ.Bytes()) != string(typ) {
			continue
		}
		stream := prep.Stream
		if uint64(len(stream.Groups)) >= m.limits.Groups || m.limits.Bytes < groupBytes || m.bytes > m.limits.Bytes-groupBytes {
			m.GapLocked(stream)
			continue
		}
		id, idErr := values.NewPublicToken(m.owner, "change-id")
		basis, basisErr := values.NewPublicToken(m.owner, "change-basis")
		if idErr != nil || basisErr != nil {
			m.GapLocked(stream)
			continue
		}
		stream.Groups = append(stream.Groups, Group{ID: id, Basis: basis, Category: category})
		m.bytes += groupBytes
	}
}

// PageLocked performs the local observation page operation after the caller
// has completed any required namespace/Core rechecks. It consumes old groups,
// evicts stale cursors, reuses a same-position cursor when possible, and
// returns the existing public group representation. It never issues a new
// cursor: the namespace owner must perform that step with its normal guards.
func (m *Manager) PageLocked(entry, prep *Context, limit int) (Page, error) {
	if m == nil {
		return Page{}, values.PublicUnavailable
	}
	if limit < 1 || limit > 256 {
		return Page{}, values.PublicInvalid
	}
	if entry == nil || prep == nil || prep.Stream == nil || prep.Stream.Gap || entry.Parent != prep.Token.Value || m.contexts[prep.Token.Value] != prep || m.contexts[entry.Token.Value] != entry {
		return Page{}, values.PublicGap
	}
	stream := prep.Stream
	if !stream.Active || entry.Position < stream.Base || entry.Position > stream.Base+uint64(len(stream.Groups)) {
		return Page{}, values.PublicGap
	}
	consumed := entry.Position - stream.Base
	if consumed > 0 {
		stream.Groups = append([]Group(nil), stream.Groups[consumed:]...)
		stream.Base = entry.Position
		charge := consumed * groupBytes
		m.bytes -= charge
		for id, c := range m.contexts {
			if c.Parent == prep.Token.Value && c.Position < stream.Base {
				m.RemoveLocked(id)
			}
		}
	}

	count := limit
	if len(stream.Groups) < count {
		count = len(stream.Groups)
	}
	result := make([]any, 0, count)
	for _, group := range stream.Groups[:count] {
		result = append(result, map[string]any{
			"changeId": requestvalue.TokenJSON(group.ID),
			"owner":    string(m.owner),
			"category": []any{group.Category},
			"basis":    requestvalue.TokenJSON(group.Basis),
		})
	}

	page := Page{Groups: result, Next: entry.Token}
	if len(result) > 0 {
		position := entry.Position + uint64(len(result))
		for _, c := range m.contexts {
			if c.Parent == prep.Token.Value && c.Position == position {
				page.Next = c.Token
				return page, nil
			}
		}
		page.Next = values.PublicToken{}
		page.Position = position
		page.NeedsCursor = true
	}
	return page, nil
}
