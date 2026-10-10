package connected

import (
	"slices"
	peercontract "ucupa/ucupa-core/contract/connected"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	queryrequest "ucupa/ucupa-core/internal/query/request"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type connectedWorld struct {
	elements    elementfact.View
	attachments facetfact.View
	mount       mountfact.View
	transfers   transferfact.Visibility
	sources     map[values.ElementID]contract.ConnectedSource
	missing     map[values.ElementID]bool
}

type connectedTrace struct {
	ids       []values.ElementID
	target    values.ElementID
	absent    bool
	absentID  values.ElementID
	parent    values.ElementID
	remote    bool
	remaining contract.Path
	key       contract.MountKey
	contact   mountfact.Contact
}

func canonicalConnectedIntent(intent exchange.Intent) (exchange.Intent, bool) {
	if intent.Kind == exchange.Query {
		q := intent.Query
		if !queryrequest.ValidPath(q.StartPath) || !queryrequest.ValidPath(q.BasisPath) ||
			q.StartPath.OriginAnchorID != q.BasisPath.OriginAnchorID {
			return exchange.Intent{}, false
		}
		local, ok := queryrequest.CanonicalRegionQuery(contract.RegionQuery{StartAnchorID: q.StartPath.OriginAnchorID,
			UpDepth: q.UpDepth, BasisAnchorID: q.BasisPath.OriginAnchorID, Sphere: q.Sphere, Selector: q.Selector})
		if !ok {
			return exchange.Intent{}, false
		}
		q.Sphere = local.Sphere
		intent.Query, intent.Path = queryrequest.CloneQuery(q), queryrequest.ClonePath(q.StartPath)
		return intent, true
	}
	if (intent.Kind != exchange.Resolve && intent.Kind != exchange.List) || !queryrequest.ValidPath(intent.Path) {
		return exchange.Intent{}, false
	}
	intent.Path = queryrequest.ClonePath(intent.Path)
	return intent, true
}

// traceConnectedPath validates every real edge. A retained mount boundary is
// not empty adjacency; it returns the exact next path segment and contact.
func traceConnectedPath(elements elementfact.View, mounts mountfact.View, transfers transferfact.Visibility, path contract.Path) (connectedTrace, error) {
	t := connectedTrace{target: path.OriginAnchorID, ids: []values.ElementID{path.OriginAnchorID}}
	if transfers.Inactive(t.target) {
		return t, contract.ErrUnavailable
	}
	if _, ok := elements.Anchor(t.target); !ok && !mountfact.IsExternalParent(mounts, t.target) {
		if elements.Kind(t.target) != 0 {
			return t, contract.ErrConflict
		}
		t.absent = true
		t.absentID = t.target
		return t, nil
	}
	for n := uint64(0); n < path.Up; n++ {
		if mountfact.IsExternalParent(mounts, t.target) {
			in := mounts.Incoming()
			t.remote, t.key, t.contact = true, in.Key, in.Contact
			t.remaining = contract.Path{OriginAnchorID: t.target, Up: path.Up - n, Down: append([]values.ElementID(nil), path.Down...)}
			return t, nil
		}
		a, ok := elements.Anchor(t.target)
		if !ok || a.Root() {
			return t, contract.ErrInvalid
		}
		t.target = a.Parent
		t.ids = append(t.ids, t.target)
		if n > uint64(elements.Counts().Anchors) {
			return t, contract.ErrStore
		}
	}
	for n, id := range path.Down {
		if record, ok := mounts.Outgoing(t.target); ok && record.State != contract.MountNone {
			t.remote, t.key, t.contact = true, record.Key, record.Contact
			t.remaining = contract.Path{OriginAnchorID: t.target, Down: append([]values.ElementID(nil), path.Down[n:]...)}
			return t, nil
		}
		if _, ok := elements.Anchor(t.target); !ok && !mountfact.IsExternalParent(mounts, t.target) {
			return t, contract.ErrConflict
		}
		parent := t.target
		var actual values.ElementID
		if a, ok := elements.Anchor(id); ok {
			actual = a.Parent
		} else if e, ok := elements.Entity(id); ok {
			actual = e.Parent
		}
		if actual != parent {
			for fence := range transfers.Fences() {
				if fence.Boundary == id && fence.Origin.Parent == parent {
					return t, contract.ErrUnavailable
				}
			}
			t.absent, t.parent, t.absentID = true, parent, id
			return t, nil
		}
		t.target = id
		t.ids = append(t.ids, id)
		if transfers.Inactive(id) {
			return t, contract.ErrUnavailable
		}
	}
	return t, nil
}

func connectedDescendant(elements elementfact.View, id, scope values.ElementID) bool {
	for n := 0; n <= elements.Counts().Anchors+1; n++ {
		if id == scope {
			return true
		}
		parent, ok := elements.ParentOf(id)
		if !ok || parent == "" {
			return false
		}
		id = parent
	}
	return false
}

func connectedScope(elements elementfact.View, mounts mountfact.View, start values.ElementID, depth int64) (values.ElementID, uint64, error) {
	current := start
	for n := uint64(0); n <= uint64(elements.Counts().Anchors)+1; n++ {
		if mountfact.IsExternalParent(mounts, current) {
			return current, n, contract.ErrMountContextRequired
		}
		a, ok := elements.Anchor(current)
		if !ok {
			if elements.Kind(current) != 0 {
				return "", 0, contract.ErrConflict
			}
			return "", 0, contract.ErrNotFound
		}
		if a.Root() || (depth >= 0 && n == uint64(depth)) {
			return current, n, nil
		}
		current = a.Parent
	}
	return "", 0, contract.ErrStore
}

func connectedShortestPath(elements elementfact.View, origin, target values.ElementID) (contract.Path, error) {
	ancestors := make(map[values.ElementID]uint64)
	id := origin
	for n := uint64(0); n <= uint64(elements.Counts().Anchors)+1; n++ {
		ancestors[id] = n
		parent, ok := elements.ParentOf(id)
		if !ok || parent == "" {
			break
		}
		id = parent
	}
	down := make([]values.ElementID, 0)
	id = target
	for n := 0; n <= elements.Counts().Anchors+1; n++ {
		if up, ok := ancestors[id]; ok {
			slices.Reverse(down)
			return contract.Path{OriginAnchorID: origin, Up: up, Down: down}, nil
		}
		down = append(down, id)
		parent, ok := elements.ParentOf(id)
		if !ok || parent == "" {
			break
		}
		id = parent
	}
	return contract.Path{}, contract.ErrUnavailable
}

func newConnectedWorld(parts []exchange.Facts) (*connectedWorld, error) {
	builder := elementfact.NewProjection()
	attachmentBuilder := facetfact.NewProjectionBuilder()
	inactive := make(map[values.ElementID]values.ElementID)
	var fences []transferfact.SourceFootprint
	w := &connectedWorld{mount: mountfact.New().View,
		sources: make(map[values.ElementID]contract.ConnectedSource), missing: make(map[values.ElementID]bool)}
	for _, part := range parts {
		if !part.Present {
			continue
		}
		source := contract.ConnectedSource{Authority: part.Authority, Cut: part.Cut}
		for id, boundary := range part.Transfers.InactiveIDs() {
			if old, exists := inactive[id]; exists && old != boundary {
				return nil, peercontract.ErrIdentityConflict
			}
			inactive[id] = boundary
		}
		for fence := range part.Transfers.Fences() {
			fences = append(fences, fence)
		}
		for id, a := range part.Elements.Anchors() {
			if _, duplicate := w.sources[id]; duplicate {
				return nil, peercontract.ErrIdentityConflict
			}
			builder.AddAnchor(id, a)
			w.sources[id] = source
			if record, ok := part.Attachments.Record(id); ok {
				attachmentBuilder.Add(id, record)
			}
		}
		for id, e := range part.Elements.Entities() {
			if _, duplicate := w.sources[id]; duplicate {
				return nil, peercontract.ErrIdentityConflict
			}
			builder.AddEntity(id, e)
			w.sources[id] = source
			if record, ok := part.Attachments.Record(id); ok {
				attachmentBuilder.Add(id, record)
			}
		}
	}
	w.elements = builder.Build().View
	w.attachments = attachmentBuilder.Build().View
	// The merged spatial world has no outgoing relations: they have already
	// been matched at each source. Do not retain unrelated parent-hold evidence.
	w.transfers = transferfact.NewVisibility(inactive, fences, nil, nil, nil)
	return w, nil
}
