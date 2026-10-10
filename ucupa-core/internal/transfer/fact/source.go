// Package fact owns deterministic Core transfer preflight rules, not transfer
// authority. Its observations are neither durable fences nor acceptance proofs.
package fact

import (
	"slices"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

// Member uses the existing value records. Only the record matching Kind is
// populated; Facet is the source relationship, never the Facet's payload.
type Member struct {
	Kind   contract.ElementKind
	Anchor elementfact.Anchor
	Entity elementfact.Entity
	Facet  facetfact.Record
	Mount  mountfact.Record // exact source-owned outgoing boundary; never its carrier's content
}

func (m Member) ID() values.ElementID {
	if m.Kind == contract.ElementAnchor {
		return m.Anchor.ID
	}
	return m.Entity.ID
}

// Slice retains only selected value copies, not a borrowed owner state. The
// source cut is an observation, not a final cut or remote installation token.
type Slice struct {
	source      values.FactAuthorityRef
	cut         contract.CoreReadCut
	boundary    values.ElementID
	members     []Member
	ancestors   []elementfact.Anchor               // immediate parent first, root last
	lineage     map[values.ElementID][]LineageStep // narrowed per-member custody, not another live state
	external    elementfact.ExternalParent         // upstream boundary is never a local member
	upstream    ParentHold                         // slice source dependency; not a carrier replacement
	edgeTarget  values.FactAuthorityRef
	closedEdges []EdgeClosure // immutable carrier holds, not active target relations
}

func (s Slice) Source() values.FactAuthorityRef            { return s.source }
func (s Slice) Cut() contract.CoreReadCut                  { return s.cut }
func (s Slice) Boundary() values.ElementID                 { return s.boundary }
func (s Slice) Members() []Member                          { return slices.Clone(s.members) }
func (s Slice) Ancestors() []elementfact.Anchor            { return slices.Clone(s.ancestors) }
func (s Slice) ExternalParent() elementfact.ExternalParent { return s.external }
func (s Slice) SourceParent() ParentHold                   { return s.upstream }

func (s Slice) Profile() Profile {
	if s.external.Present && s.boundary == s.external.ID {
		return CarrierReplacement
	}
	for _, member := range s.members {
		if member.ID() == s.boundary {
			if member.Kind == contract.ElementAnchor && member.Anchor.Root() {
				return RootSuccessor
			}
			return SliceTransfer
		}
	}
	return 0
}

// Select reads a validated, single-cut source under its owner's lock. maxRecords
// bounds members plus ancestor dependencies; no truncated slice is returned.
// Root selection includes all owner-local records and has no ancestors. The
// caller must bind its explicit profile before a durable intent or fence.
// Carrier and external-boundary continuations still require matching evidence.
func Select(owner values.FactAuthorityRef, cut contract.CoreReadCut, elements elementfact.View,
	facets facetfact.View, mounts mountfact.View, boundary values.ElementID, maxRecords int) (Slice, error) {
	if !owner.Valid() || cut == (contract.CoreReadCut{}) || !boundary.Valid() || maxRecords <= 0 {
		return Slice{}, contract.ErrInvalid
	}
	in := mounts.Incoming()
	var external elementfact.ExternalParent
	if in.Key.Valid() {
		if in.Key.CarrierAuthority != owner || in.Gate != mountfact.GateOpen || in.Detached || in.Preparing != (contract.MountAttemptKey{}) ||
			!mountfact.ValidMountAdmissionKey(in.Admission.Key) || in.Admission.Key.MountKey != in.Key {
			return Slice{}, contract.ErrMountCoordinationRequired
		}
		external = elementfact.ExternalParent{ID: in.Key.MountAnchorUUID, Range: in.Admission.RangeValue, Present: true, Admitted: true}
	}
	return selectWithin(owner, cut, elements, facets, mounts, boundary, maxRecords, external)
}

// SelectHeld reuses the same finite closure rule for a source manifest. The
// authenticated parent hold supplies the external parent; its Element is never
// inserted into the source tree. The live owner checks its actual incoming
// relationship separately before publishing a fence.
func SelectHeld(h ParentHold, cut contract.CoreReadCut, elements elementfact.View, facets facetfact.View,
	mounts mountfact.View, maxRecords int) (Slice, error) {
	if maxRecords <= 0 || !h.Valid() || h.Input.Role != SourceParent || !token.Valid(cut) || token.Scope(cut) != token.Scope(h.Input.Initial.Admission) ||
		token.Serial(cut) < token.Serial(h.Input.Initial.Admission) {
		return Slice{}, contract.ErrMountCoordinationRequired
	}
	i := h.Input.Initial.Input
	external := elementfact.ExternalParent{ID: h.Input.Mount, Range: h.Tuple.Range, Present: true, Admitted: true}
	s, err := selectWithin(i.Source, cut, elements, facets, mounts, i.Boundary, maxRecords, external)
	if err == nil {
		s.upstream = h
	}
	return s, err
}

func selectWithin(owner values.FactAuthorityRef, cut contract.CoreReadCut, elements elementfact.View,
	facets facetfact.View, mounts mountfact.View, boundary values.ElementID, maxRecords int, external elementfact.ExternalParent) (Slice, error) {
	kind := elements.Kind(boundary)
	if kind == 0 {
		return Slice{}, contract.ErrNotFound
	}
	ids, err := selectMembers(elements, boundary, kind, maxRecords)
	if err != nil {
		return Slice{}, err
	}
	s := Slice{source: owner, cut: cut, boundary: boundary, external: external}
	if err := s.selectRecords(ids, elements, facets, mounts); err != nil {
		return Slice{}, err
	}
	parent, _ := elements.ParentOf(boundary)
	seen := make(map[values.ElementID]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for parent != "" && (!external.Present || parent != external.ID) {
		if seen[parent] {
			return Slice{}, contract.ErrStore
		}
		seen[parent] = true
		ancestor, ok := elements.Anchor(parent)
		if !ok {
			return Slice{}, contract.ErrStore
		}
		if err := checkLocalRelation(mounts, parent); err != nil {
			return Slice{}, err
		}
		if len(s.members)+len(s.ancestors) >= maxRecords {
			return Slice{}, values.PublicResourceExhausted
		}
		s.ancestors = append(s.ancestors, ancestor)
		parent = ancestor.Parent
	}
	if external.Present && parent != external.ID {
		return Slice{}, contract.ErrConflict
	}
	return s, nil
}

func (s *Slice) selectRecords(ids []values.ElementID, elements elementfact.View, facets facetfact.View, mounts mountfact.View) error {
	for _, id := range ids {
		edge, err := s.selectMount(elements, mounts, id)
		if err != nil {
			return err
		}
		facet, ok := facets.Record(id)
		if !ok {
			return contract.ErrStore
		}
		if facet.Pending != (values.FacetAcceptanceKey{}) {
			return contract.ErrAttachmentStateConflict
		}
		member := Member{Kind: elements.Kind(id), Facet: facet, Mount: edge}
		member.Anchor, _ = elements.Anchor(id)
		member.Entity, _ = elements.Entity(id)
		s.members = append(s.members, member)
	}
	return nil
}

func selectMembers(elements elementfact.View, boundary values.ElementID, kind contract.ElementKind, limit int) ([]values.ElementID, error) {
	if kind == contract.ElementEntity {
		return []values.ElementID{boundary}, nil
	}
	// Build adjacency once. Rewalking every parent chain would make a deep
	// owner-local tree quadratic. This temporary index is not another fact owner.
	children := make(map[values.ElementID][]values.ElementID)
	for id, anchor := range elements.Anchors() {
		children[anchor.Parent] = append(children[anchor.Parent], id)
	}
	for id, entity := range elements.Entities() {
		children[entity.Parent] = append(children[entity.Parent], id)
	}
	ids := []values.ElementID{boundary}
	seen := map[values.ElementID]bool{boundary: true}
	for cursor := 0; cursor < len(ids); cursor++ {
		for _, child := range children[ids[cursor]] {
			if seen[child] {
				return nil, contract.ErrStore
			}
			if len(ids) >= limit {
				return nil, values.PublicResourceExhausted
			}
			seen[child] = true
			ids = append(ids, child)
		}
	}
	elementfact.SortIDs(ids)
	return ids, nil
}

func checkLocalRelation(mounts mountfact.View, id values.ElementID) error {
	r, exists := mounts.Outgoing(id)
	if !exists {
		return nil
	}
	if r.Slot != "" {
		return contract.ErrMountBusy
	}
	if r.State != contract.MountNone {
		return contract.ErrMountCoordinationRequired
	}
	return nil
}
