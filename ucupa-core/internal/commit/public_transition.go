package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/geometry"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// PublicTransitionInputValid checks the bounded public shape before any lookup.
// It does not parse a MAC into private state or authenticate its issuer.
func PublicTransitionInputValid(source values.FactAuthorityRef, command contract.TransitionCommand) bool {
	placement := command.Kind == contract.TransitionSlice && command.Final.Parent.Valid() &&
		geometry.CanonicalPose(command.Final.LocalPose) && geometry.ValidSphere(command.Final.Range)
	if command.Kind == contract.TransitionRoot {
		placement = command.Final == (contract.SpatialTuple{})
	}
	return placement && source.Valid() && command.Target.Valid() && source != command.Target && command.Boundary.Valid() &&
		command.Expected.Valid() && command.Expected.Owner == source && command.Expected.Kind == "record-revision" &&
		command.SourceAddress.Valid() && command.TargetAddress.Valid()
}

// BeginPublicTransitionLocked fixes only an intent. No source fence, target
// promise or content export occurs here. The later driver must obtain target-
// limited approval and revalidate original caller views before each decision.
func (o *Owner) BeginPublicTransitionLocked(actor values.PrincipalRef, id contract.OperationID, command contract.TransitionCommand) (contract.TransitionStatus, error) {
	if !actor.Valid() || len(id) == 0 || len(id) > 128 || !PublicTransitionInputValid(o.authority, command) {
		return contract.TransitionStatus{}, contract.ErrInvalid
	}
	if err := o.transferPreflightReadyLocked(); err != nil {
		return contract.TransitionStatus{}, err
	}
	if o.store == nil {
		return contract.TransitionStatus{}, values.PublicUnavailable
	}
	expected := o.state.publicExpectedRevision(command.Boundary, command.Expected)
	if old, exists := o.state.transfers.records[id]; exists {
		// A retry compares the original public CAS even after the source facts
		// changed or departed; it must not silently bind the current Revision.
		if old.Input.Actor != actor || old.Input.Boundary != command.Boundary || o.SigningLocked().ElementRevision(old.Input.Expected) != command.Expected {
			return contract.TransitionStatus{}, contract.ErrOperationConflict
		}
		expected = old.Input.Expected
	}
	profile := transferfact.SliceTransfer
	if command.Kind == contract.TransitionRoot {
		profile = transferfact.RootSuccessor
	}
	input := transferfact.Intent{Profile: profile, Operation: id, Actor: actor, Source: o.authority, Target: command.Target,
		Boundary: command.Boundary, Expected: expected, Final: command.Final, SourceAddress: command.SourceAddress, TargetAddress: command.TargetAddress}
	r, err := o.BeginTransferIntentLocked(input, transferencoding.MaxTransferManifestRecords)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	return o.publicTransitionStatus(r)
}

// Inspection is an exact, original-actor lookup, never a task enumeration or a
// dump of manifests, credentials, private control sequences or other outcomes.
func (o *Owner) InspectPublicTransitionLocked(actor values.PrincipalRef, id contract.OperationID, boundary values.ElementID) (contract.TransitionStatus, error) {
	r, err := o.publicTransitionRecord(actor, id, boundary)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	return o.publicTransitionStatus(r)
}

func (o *Owner) CancelPublicTransitionLocked(actor values.PrincipalRef, id contract.OperationID, boundary values.ElementID, expected values.PublicToken) (contract.TransitionStatus, error) {
	if !expected.Valid() || expected.Owner != o.authority || expected.Kind != "transition-revision" {
		return contract.TransitionStatus{}, contract.ErrInvalid
	}
	r, err := o.publicTransitionRecord(actor, id, boundary)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	comparison := r.Revision
	if r.Progress == transferfact.IntentCancelled {
		comparison = o.state.transfers.events[r.Revision-1].Expected
	}
	if o.SigningLocked().TransitionRevision(r.Admission, string(id), boundary, comparison) != expected {
		return contract.TransitionStatus{}, contract.ErrRevisionConflict
	}
	r, err = o.CancelTransferIntentLocked(actor, id, comparison)
	if err != nil {
		return contract.TransitionStatus{}, err
	}
	return o.publicTransitionStatus(r)
}

func (o *Owner) publicTransitionRecord(actor values.PrincipalRef, id contract.OperationID, boundary values.ElementID) (transferfact.IntentRecord, error) {
	if !actor.Valid() || len(id) == 0 || len(id) > 128 || !boundary.Valid() {
		return transferfact.IntentRecord{}, contract.ErrInvalid
	}
	r, err := o.TransferIntentLocked(id)
	if err != nil {
		return transferfact.IntentRecord{}, err
	}
	if r.Input.Actor != actor || r.Input.Boundary != boundary {
		return transferfact.IntentRecord{}, values.PublicDenied
	}
	return r, nil
}

func (o *Owner) publicTransitionStatus(r transferfact.IntentRecord) (contract.TransitionStatus, error) {
	kind := contract.TransitionSlice
	if r.Input.Profile == transferfact.RootSuccessor {
		kind = contract.TransitionRoot
	} else if r.Input.Profile != transferfact.SliceTransfer {
		return contract.TransitionStatus{}, contract.ErrStore
	}
	var progress contract.TransitionProgress
	switch r.Progress {
	case transferfact.IntentPending:
		progress = contract.TransitionIntent
	case transferfact.IntentCancelled:
		progress = contract.TransitionCancelled
	case transferfact.IntentFenced:
		progress = contract.TransitionFenced
	case transferfact.IntentCommitted:
		progress = contract.TransitionCommitted
	case transferfact.IntentSettled:
		progress = contract.TransitionSettled
	default:
		return contract.TransitionStatus{}, contract.ErrStore
	}
	revision := o.SigningLocked().TransitionRevision(r.Admission, string(r.Input.Operation), r.Input.Boundary, r.Revision)
	if !revision.Valid() {
		return contract.TransitionStatus{}, contract.ErrStore
	}
	return contract.TransitionStatus{Source: o.authority, Operation: r.Input.Operation, Kind: kind, Target: r.Input.Target,
		Boundary: r.Input.Boundary, Progress: progress,
		Revision: revision}, nil
}
