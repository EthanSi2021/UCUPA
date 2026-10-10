package commit

import (
	"fmt"

	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/token"
)

// Only the matching finite origin follows a read view. Later ordinary range,
// address or revoke controls remain legal; they cannot predate its new parent.
func validateEdgeIncomingOrigin(facts mountSnapshotFacts) error {
	in, installed := facts.mount.Incoming(), facts.edgeInstallation
	key, err := installed.Parent.Delegation(installed.Closure)
	if err != nil || !installed.Valid() || in.Acceptance != (contract.MountPrepareRequest{}) ||
		key.MountKey.CarrierAuthority != facts.authority || in.Key != key.MountKey || in.AcceptedAt != installed.Closure.At ||
		token.Scope(installed.At) != facts.scope || token.Serial(installed.At) > facts.cutSerial || token.NextHopSerial(in.Contact.Revision) < token.Serial(installed.At) {
		return fmt.Errorf("incoming relation lacks exact transferred-parent installation")
	}
	if token.Serial(in.Admission.DecisionCut) < token.Serial(installed.Parent.Cut()) ||
		in.Admission.DecisionCut == installed.Parent.Cut() && (in.Admission.Key != key || in.Admission.RangeValue != installed.Closure.Range) {
		return fmt.Errorf("incoming admission contradicts transferred parent")
	}
	return nil
}
