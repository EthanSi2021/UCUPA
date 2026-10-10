package local

import (
	"ucupa/shared/values"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// A structural address resolves only along verified real edges inside the
// declared boundary, never by an unknown Element UUID alone.

func publicAddressForViews(elements elementfact.View, mounts mountfact.View, transfers transferfact.Visibility, boundary values.ElementID, raw any) (values.ElementID, error) {
	address, err := accessvalue.Object(raw, "origin", "up", "down")
	if err != nil {
		return "", err
	}
	origin := boundary
	if raw, ok := address["origin"]; ok {
		text, ok := raw.(string)
		if !ok {
			return "", values.PublicInvalid
		}
		origin = values.ElementID(text)
	}
	if transfers.Inactive(boundary) {
		return "", values.PublicUnavailable
	}
	if departed := transfers.Boundary(origin); departed != "" {
		for fence := range transfers.Fences() {
			if fence.Boundary == departed && elements.DescendsFrom(fence.Origin.Parent, boundary) {
				return "", values.PublicUnavailable
			}
		}
	}
	if !origin.Valid() || !elements.DescendsFrom(origin, boundary) {
		return "", values.PublicInvalid
	}
	if _, exists := elements.Anchor(origin); !exists {
		return "", values.PublicInvalid
	}
	up := uint64(0)
	if raw, exists := address["up"]; exists {
		text, ok := raw.(string)
		if !ok {
			return "", values.PublicInvalid
		}
		up, err = values.ParsePublicParentCount(text)
		if err != nil {
			return "", err
		}
	}
	for i := uint64(0); i < up; i++ {
		if origin == boundary {
			return "", values.PublicInvalid
		}
		anchor, exists := elements.Anchor(origin)
		if !exists || !anchor.Parent.Valid() {
			return "", values.PublicInvalid
		}
		origin = anchor.Parent
	}
	if raw, exists := address["down"]; exists {
		chain, ok := raw.([]any)
		if !ok {
			return "", values.PublicInvalid
		}
		for _, rawID := range chain {
			text, ok := rawID.(string)
			if !ok || !values.ElementID(text).Valid() {
				return "", values.PublicInvalid
			}
			id := values.ElementID(text)
			if mountfact.HasContent(mounts, origin) {
				return "", values.PublicUnavailable
			}
			if anchor, exists := elements.Anchor(id); exists && anchor.Parent == origin {
				if transfers.Inactive(id) {
					return "", values.PublicUnavailable
				}
				origin = id
				continue
			}
			if entity, exists := elements.Entity(id); exists && entity.Parent == origin {
				if transfers.Inactive(id) {
					return "", values.PublicUnavailable
				}
				origin = id
				continue
			}
			for fence := range transfers.Fences() {
				if fence.Boundary == id && fence.Origin.Parent == origin {
					return "", values.PublicUnavailable
				}
			}
			return "", values.PublicNotFound
		}
	}
	return origin, nil
}
