package commit

import (
	"fmt"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Carrier installation is its own origin evidence, never a fabricated ordinary
// mount Prepare. Subsequent range, contact and revoke operations remain legal;
// current admission is checked separately from this immutable origin receipt.
func validateCarrierIncomingOrigin(facts mountSnapshotFacts) error {
	if facts.edgeInstallation != (transferfact.EdgeInstallation{}) {
		if facts.carrierInstallation != (transferfact.CarrierInstallation{}) {
			return fmt.Errorf("incoming relation has two transfer origins")
		}
		return validateEdgeIncomingOrigin(facts)
	}
	in, installed := facts.mount.Incoming(), facts.carrierInstallation
	if !in.Key.Valid() || in.Acceptance != (contract.MountPrepareRequest{}) {
		if installed != (transferfact.CarrierInstallation{}) {
			return fmt.Errorf("carrier installation does not bind current incoming relation")
		}
		return nil
	}
	a := installed.Binding.Acceptance
	r := a.Fence.Permit.Reservation
	if !installed.Valid() || r.Input.Target != facts.authority || r.NextKey != in.Key || in.AcceptedAt != a.At ||
		token.Scope(installed.At) != facts.scope || token.Serial(installed.At) > facts.cutSerial || token.NextHopSerial(in.Contact.Revision) < token.Serial(installed.At) {
		return fmt.Errorf("incoming relation lacks exact carrier installation")
	}
	if token.Serial(in.Admission.DecisionCut) < token.Serial(installed.Binding.At) ||
		in.Admission.DecisionCut == installed.Binding.At && (in.Admission.Key.OperationID != r.Input.Operation || in.Admission.RangeValue != r.Tuple.Range) {
		return fmt.Errorf("incoming admission predates or contradicts carrier binding")
	}
	return nil
}
