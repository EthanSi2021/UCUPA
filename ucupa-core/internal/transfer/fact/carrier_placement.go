package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

// PlaceCarrier checks an unactivated receiver of a complete carried space.
// M is an external parent, not a local root or a member to be created. Prior
// incoming relations require their own continuity proof and cannot be replaced
// merely because their current Element set happens to be empty.
func PlaceCarrier(source Slice, target values.FactAuthorityRef, cut contract.CoreReadCut,
	elements elementfact.State, mounts mountfact.View, maxRecords int, returns map[values.ElementID]elementfact.Departure,
	previous CarrierSettlement, permit CarrierControl) error {
	if source.Profile() != CarrierReplacement || !source.source.Valid() || !target.Valid() || !token.Valid(cut) || maxRecords <= 0 {
		return contract.ErrInvalid
	}
	// Admission requires each external carrier's exact durable hold. It does
	// not activate the target or substitute for later matching delegation.
	if !source.edgesClosedFor(target) {
		return contract.ErrMountCoordinationRequired
	}
	if source.source == target || elements.Counts().Anchors != 0 || elements.Counts().Entities != 0 {
		return contract.ErrConflict
	}
	if _, retired := elements.RetiredRoot(); retired {
		return contract.ErrConflict
	}
	if mounts.Incoming().Key.Valid() {
		in := mounts.Incoming()
		old := previous.Installation.Binding.Acceptance.Fence.Permit.Reservation
		next := permit.Reservation
		if !previous.Valid(target) || old.Input.Source != target || in.Key != old.Previous.MountKey ||
			in.Gate != mountfact.GateFenced || in.Detached || in.Preparing != (contract.MountAttemptKey{}) ||
			!permit.Valid() || permit.PermitAt == (contract.CoreReadCut{}) || permit.CancelledAt != (contract.CoreReadCut{}) ||
			next.Input.Parent != old.Input.Parent || next.Input.Mount != old.Input.Mount || next.Input.Target != target || next.Input.Source != source.source ||
			token.Scope(next.At) != token.Scope(previous.Installation.Binding.At) || token.Serial(next.At) <= token.Serial(previous.Installation.Binding.At) ||
			source.boundary != next.Input.Mount {
			return contract.ErrMountCoordinationRequired
		}
	} else if elements.Counts().Claims != len(returns) {
		return contract.ErrConflict
	}
	for _, outgoing := range mounts.Outgoings() {
		if outgoing.State != contract.MountNone || outgoing.Slot != "" {
			return contract.ErrMountCoordinationRequired
		}
	}
	if len(source.members) > maxRecords {
		return values.PublicResourceExhausted
	}
	if err := checkInstallClaims(source, elements, returns); err != nil {
		return err
	}
	for _, member := range source.members {
		if _, err := facetfact.TransferPredecessor(member.Facet, member.ID(), cut); err != nil {
			return err
		}
	}
	return nil
}
