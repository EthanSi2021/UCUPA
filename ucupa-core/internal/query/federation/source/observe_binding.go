package source

import (
	federationcontract "ucupa/ucupa-core/contract/federation"
	federationprojection "ucupa/ucupa-core/internal/query/federation/projection"

	"ucupa/shared/values"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/geometry"
)

// Read observes its disclosed structural chain, not unrelated elements in the
// source closure. Intermediate owners bind only path facts; the terminal owner
// additionally binds the exact record/attachment. Binding cannot miss writes
// between capture and discovery: extending interest requires the original cut.
func (core *Operations) bindFederationObservationLocked(basis accessregistry.ReadUse, state *federationprojection.Projection, request federationcontract.FederationProjectionRequest, result federationcontract.FederationProjectionPage) error {
	if !basis.HasPreparation || basis.Source.Request.Intent != "read" || request.Kind != "path" && request.Kind != "read" {
		return nil
	}
	prep, present := core.registry.PreparationLocked(basis.Preparation)
	if !present || !prep.HasQueue || prep.Gap {
		return values.PublicGap
	}
	ids := make(map[values.ElementID]bool)
	if request.Kind == "path" {
		if result.Path == nil {
			return values.PublicInvalidContext
		}
		for _, node := range result.Path.Spine {
			ids[node.ID] = true
		}
	} else {
		if prep.Selection.Exact != "" && prep.Selection.Exact != request.Element {
			return values.PublicInvalidContext
		}
		path, err := state.PathIDs(request.Element, basis.Boundary, geometry.MaxExactProjectionEdges)
		if err != nil {
			return err
		}
		ids[basis.Boundary] = true
		for _, id := range path {
			ids[id] = true
		}
	}
	bindExact := request.Kind == "read" && prep.Selection.Exact == ""
	return core.registry.BindSourceObservationLocked(basis.Ref, ids, request.Element, bindExact, core.owner.CurrentCutLocked(), geometry.MaxExactProjectionEdges+1)
}
