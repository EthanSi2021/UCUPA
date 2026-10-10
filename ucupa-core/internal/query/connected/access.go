package connected

import (
	"slices"
	querypkg "ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Planning is private to the owner. A probe releases only the spatial closure
// covered by the exact grant plus separately authorized explanatory prefixes.
func connectedProbeGrant(elements elementfact.View, mounts mountfact.View, transfers transferfact.Visibility, intent exchange.Intent, key contract.MountKey, proofOnly bool) (exchange.Grant, error) {
	grant := exchange.Grant{Kind: intent.Kind, Mount: key, ProofOnly: proofOnly}
	if in := mounts.Incoming(); intent.Kind == exchange.Resolve && in.Key.Valid() &&
		intent.Path.OriginAnchorID == in.Key.MountAnchorUUID && intent.Path.Up == 0 && len(intent.Path.Down) == 0 {
		return exchange.Grant{Kind: exchange.Resolve, Scope: in.Key.MountAnchorUUID, Mount: in.Key, ProofOnly: true, EntryProof: true}, nil
	}
	if proofOnly {
		grant.Scope = key.MountAnchorUUID
		return grant, nil
	}
	trace, err := traceConnectedPath(elements, mounts, transfers, intent.Path)
	if err == nil && trace.remote && trace.remaining.Up != 0 && mounts.Incoming().Key.Valid() {
		mount := mounts.Incoming().Key.MountAnchorUUID
		for index, id := range intent.Path.Down {
			if id != mount {
				continue
			}
			tail := connectedIntentAt(intent, contract.Path{OriginAnchorID: mount, Down: append([]values.ElementID(nil), intent.Path.Down[index+1:]...)}, intent.Query.UpDepth)
			joined, tailErr := connectedProbeGrant(elements, mounts, transfers, tail, key, false)
			for _, prefix := range trace.ids {
				if mountfact.IsExternalParent(mounts, prefix) {
					continue
				}
				if intent.Kind == exchange.Query && !joined.ProofOnly && connectedDescendant(elements, prefix, joined.Scope) {
					continue
				}
				if !slices.Contains(joined.Prefix, prefix) {
					joined.Prefix = append(joined.Prefix, prefix)
				}
			}
			if len(joined.Prefix) != 0 {
				joined.EntryProof = false
			}
			return joined, tailErr
		}
	}
	grant.Prefix = append([]values.ElementID(nil), trace.ids...)
	grant.Scope = trace.target
	if err != nil || trace.absent {
		if len(intent.Path.Down) != 0 {
			grant.Scope = intent.Path.Down[len(intent.Path.Down)-1]
			grant.Prefix = append(grant.Prefix, grant.Scope)
		}
		if intent.Kind == exchange.Query {
			grant.Prefix = slices.DeleteFunc(grant.Prefix, func(id values.ElementID) bool { return id == grant.Scope })
		}
		return grant, err
	}
	if intent.Kind != exchange.Query {
		if trace.remote {
			grant.ProofOnly = true
		}
		if intent.Kind == exchange.Resolve && mountfact.IsExternalParent(mounts, trace.target) {
			grant.ProofOnly = true
		}
		if intent.Kind == exchange.List && !trace.remote && len(grant.Prefix) != 0 {
			grant.Prefix = grant.Prefix[:len(grant.Prefix)-1]
		}
		return grant, nil
	}
	depth := intent.Query.UpDepth
	if trace.remote {
		if trace.remaining.Up != 0 || (depth >= 0 && uint64(depth) < uint64(len(trace.remaining.Down))) {
			grant.ProofOnly = true
			return grant, nil
		}
		if depth >= 0 {
			depth -= int64(len(trace.remaining.Down))
		}
	}
	scope, _, decision := connectedScope(elements, mounts, trace.target, depth)
	if decision != nil && decision != contract.ErrMountContextRequired {
		grant.Prefix = slices.DeleteFunc(grant.Prefix, func(id values.ElementID) bool { return id == grant.Scope })
		return grant, decision
	}
	grant.Scope = scope
	grant.Prefix = slices.DeleteFunc(grant.Prefix, func(id values.ElementID) bool {
		return connectedDescendant(elements, id, scope)
	})
	return grant, nil
}

func (core *Operations) connectedRequirements(grant exchange.Grant) []values.Requirement {
	result := make([]values.Requirement, 0, len(grant.Prefix)+len(grant.Adjacency)+2)
	add := func(permission values.PermissionID, id values.ElementID) {
		if !id.Valid() {
			return
		}
		r := core.requirement(permission, id)
		if !slices.Contains(result, r) {
			result = append(result, r)
		}
	}
	if !grant.ProofOnly {
		switch grant.Kind {
		case exchange.Query:
			add(contract.PermissionQueryRegion, grant.Scope)
		case exchange.List:
			add(contract.PermissionListDirect, grant.Scope)
		case exchange.Resolve:
			add(contract.PermissionReadElement, grant.Scope)
		}
	}
	for _, id := range grant.Prefix {
		if id != grant.Mount.MountAnchorUUID || core.authority != grant.Mount.CarrierAuthority {
			add(contract.PermissionReadElement, id)
		}
	}
	for _, id := range grant.Adjacency {
		add(contract.PermissionListDirect, id)
	}
	if grant.Mount.Valid() && core.authority == grant.Mount.ParentAuthority {
		add(contract.PermissionMountRead, grant.Mount.MountAnchorUUID)
	}
	return result
}

func (core *Operations) authorizeConnected(actor values.PrincipalRef, grant exchange.Grant) (contract.RealmAttachment, error) {
	requirements := core.connectedRequirements(grant)
	if grant.EntryProof && grant.Kind == exchange.Resolve && grant.ProofOnly && grant.Scope == grant.Mount.MountAnchorUUID &&
		len(grant.Prefix) == 0 && len(grant.Adjacency) == 0 && grant.Mount.Valid() && core.authority == grant.Mount.CarrierAuthority {
		// The sole external Origin names A's already accepted M, not B content.
		// A independently grants exact-read and mount-read before any record is
		// disclosed. B contributes only this established entry's contact proof.
		if !actor.Valid() {
			return contract.RealmAttachment{}, contract.ErrDenied
		}
		approval, ok := core.owner.CurrentContact()
		if !ok {
			return contract.RealmAttachment{}, contract.ErrUnavailable
		}
		return approval, nil
	}
	if grant.VerifyOnly && grant.ProofOnly && len(grant.Prefix) == 0 && len(grant.Adjacency) == 0 &&
		grant.Mount.Valid() && core.authority == grant.Mount.CarrierAuthority {
		if !actor.Valid() {
			return contract.RealmAttachment{}, contract.ErrDenied
		}
		approval, ok := core.owner.CurrentContact()
		if !ok {
			return contract.RealmAttachment{}, contract.ErrUnavailable
		}
		return approval, nil
	}
	if len(requirements) == 0 {
		return contract.RealmAttachment{}, contract.ErrDenied
	}
	return core.authorize(actor, requirements...)
}

func filterConnectedFacts(view *querypkg.Lease, grant exchange.Grant) exchange.Facts {
	source := view.Projection()
	builder := elementfact.NewProjection()
	attachmentBuilder := facetfact.NewProjectionBuilder()
	mount := mountfact.New()
	inactive := make(map[values.ElementID]values.ElementID)
	var fences []transferfact.SourceFootprint
	include := func(id values.ElementID) {
		if source.Transfers.Inactive(id) {
			inactive[id] = source.Transfers.Boundary(id)
		}
		if a, ok := source.Elements.Anchor(id); ok {
			builder.AddAnchor(id, a)
			if grant.Kind == exchange.Resolve {
				record, _ := source.Attachments.Record(id)
				attachmentBuilder.Add(id, facetfact.Record{Attachment: record.Attachment})
			}
		} else if e, ok := source.Elements.Entity(id); ok {
			builder.AddEntity(id, e)
			if grant.Kind == exchange.Resolve {
				record, _ := source.Attachments.Record(id)
				attachmentBuilder.Add(id, facetfact.Record{Attachment: record.Attachment})
			}
		}
	}
	for _, id := range grant.Prefix {
		include(id)
	}
	if !grant.ProofOnly {
		include(grant.Scope)
		for id, a := range source.Elements.Anchors() {
			if (grant.Kind == exchange.Query && connectedDescendant(source.Elements, id, grant.Scope)) ||
				(grant.Kind == exchange.List && a.Parent == grant.Scope) {
				include(id)
			}
		}
		for id, e := range source.Elements.Entities() {
			if (grant.Kind == exchange.Query && connectedDescendant(source.Elements, id, grant.Scope)) ||
				(grant.Kind == exchange.List && e.Parent == grant.Scope) {
				include(id)
			}
		}
	}
	if grant.Mount.Valid() {
		if source.Authority == grant.Mount.ParentAuthority {
			mount.ReplaceOutgoing(grant.Mount.MountAnchorUUID, source.Mount.Record(grant.Mount.MountAnchorUUID))
		} else if source.Authority == grant.Mount.CarrierAuthority {
			mount.ReplaceIncoming(source.Mount.Incoming())
		}
	}
	// A peer receives only the unresolved envelopes covered by its original
	// Query/List grant, never the source's whole inactive-identity collection.
	for fence := range source.Transfers.Fences() {
		if !grant.ProofOnly && ((grant.Kind == exchange.Query && connectedDescendant(source.Elements, fence.Origin.Parent, grant.Scope)) ||
			(grant.Kind == exchange.List && fence.Origin.Parent == grant.Scope)) {
			fences = append(fences, fence)
			inactive[fence.Boundary] = fence.Boundary
		}
	}
	facts := exchange.Facts{Present: true, Authority: source.Authority, Scope: source.Scope, CutSerial: source.CutSerial,
		Elements: builder.Build().View, Attachments: attachmentBuilder.Build().View, Mount: mount.View,
		Transfers: transferfact.NewVisibility(inactive, fences, nil, nil, nil),
		Cut:       view.Cut(), Run: view.Run()}
	if grant.Mount.Valid() && source.Authority == grant.Mount.ParentAuthority {
		if anchor, ok := source.Elements.Anchor(grant.Mount.MountAnchorUUID); ok {
			facts.AdmissionRange, _ = anchor.Range.BoundedSphere()
		}
	}
	return facts
}

func sameConnectedGrant(a, b exchange.Grant) bool {
	return a.Kind == b.Kind && a.Scope == b.Scope && a.ProofOnly == b.ProofOnly && a.VerifyOnly == b.VerifyOnly && a.EntryProof == b.EntryProof && a.Mount == b.Mount &&
		slices.Equal(a.Prefix, b.Prefix) && slices.Equal(a.Adjacency, b.Adjacency)
}

func cloneConnectedGrant(grant exchange.Grant) exchange.Grant {
	grant.Prefix = append([]values.ElementID(nil), grant.Prefix...)
	grant.Adjacency = append([]values.ElementID(nil), grant.Adjacency...)
	return grant
}
