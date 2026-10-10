package commit

import (
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/geometry"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
)

type storedBinding struct {
	AccessMode     string                      `json:"accessMode"`
	Boundary       values.ElementID            `json:"boundary,omitempty"`
	PublicRequest  string                      `json:"publicRequest,omitempty"`
	ActorRealm     []byte                      `json:"actorRealm"`
	ActorPrincipal []byte                      `json:"actorPrincipal"`
	Kind           string                      `json:"kind"`
	Element        string                      `json:"element"`
	Final          *storerecord.StoredTuple    `json:"final,omitempty"`
	Expected       *storerecord.StoredRevision `json:"expected,omitempty"`
}

type storedOutcome struct {
	Status         string  `json:"status"`
	Error          string  `json:"error,omitempty"`
	RevisionSerial *uint64 `json:"revision,omitempty"`
	CutSerial      uint64  `json:"cut"`
}

type storedOperation struct {
	Admission uint64        `json:"admissionCut"`
	Binding   storedBinding `json:"binding"`
	Outcome   storedOutcome `json:"outcome"`
}

type storedCommit struct {
	Kind         string `json:"kind"`
	Operation    []byte `json:"operation,omitempty"`
	ControlIndex uint64 `json:"controlIndex,omitempty"`
}

func storeOperation(operation retainedOperation) (storedOperation, error) {
	binding, err := storeBinding(operation.binding)
	if err != nil {
		return storedOperation{}, err
	}
	outcome, err := storeOutcome(operation.binding, operation.outcome)
	if err != nil {
		return storedOperation{}, err
	}
	return storedOperation{Admission: token.Serial(operation.admission), Binding: binding, Outcome: outcome}, nil
}

func storeBinding(binding commitbinding.Binding) (storedBinding, error) {
	if !commitbinding.CanonicalBindingForStore(binding) {
		return storedBinding{}, errors.New("operation binding is invalid")
	}
	stored := storedBinding{
		AccessMode: "exact", Boundary: binding.Boundary, PublicRequest: binding.PublicRequest,
		ActorRealm:     append([]byte(nil), []byte(binding.Actor.Realm)...),
		ActorPrincipal: append([]byte(nil), []byte(binding.Actor.Principal)...),
		Kind:           commitbinding.KindName(binding.Kind), Element: string(binding.Element),
	}
	if binding.Boundary != "" {
		stored.AccessMode = "closure"
	}
	switch binding.Kind {
	case elementfact.OperationCreateAnchor, elementfact.OperationCreateEntity, elementfact.OperationUpdateAnchor, elementfact.OperationUpdateEntity:
		final := storerecord.StoreTuple(binding.Final)
		stored.Final = &final
	}
	switch binding.Kind {
	case elementfact.OperationUpdateAnchor, elementfact.OperationUpdateEntity, elementfact.OperationDeleteAnchor, elementfact.OperationDeleteEntity:
		expected := storerecord.StoreRevision(binding.Expected)
		stored.Expected = &expected
	}
	return stored, nil
}

func (stored storedBinding) binding() (commitbinding.Binding, error) {
	if (stored.AccessMode != "exact" && stored.AccessMode != "closure") || (stored.AccessMode == "exact") != (stored.Boundary == "") {
		return commitbinding.Binding{}, errors.New("invalid access binding")
	}
	binding := commitbinding.Binding{
		Boundary: stored.Boundary, PublicRequest: stored.PublicRequest,
		Actor: values.PrincipalRef{Realm: values.RealmID(string(stored.ActorRealm)), Principal: values.PrincipalID(string(stored.ActorPrincipal))},
		Kind:  commitbinding.KindFromName(stored.Kind), Element: values.ElementID(stored.Element),
	}
	if stored.Final != nil {
		final, err := stored.Final.Tuple()
		if err != nil {
			return commitbinding.Binding{}, err
		}
		binding.Final = final
	}
	if stored.Expected != nil {
		expected, err := stored.Expected.Revision()
		if err != nil {
			return commitbinding.Binding{}, err
		}
		binding.Expected = expected
	}
	if !commitbinding.CanonicalBindingForStore(binding) {
		return commitbinding.Binding{}, errors.New("operation binding shape is invalid")
	}
	return binding, nil
}

func storeOutcome(binding commitbinding.Binding, outcome retainedOutcome) (storedOutcome, error) {
	stored := storedOutcome{CutSerial: token.Serial(outcome.cut)}
	switch outcome.Status {
	case contract.MutationCommitted:
		stored.Status = "committed"
		var serial uint64
		switch binding.Kind {
		case elementfact.OperationCreateRoot, elementfact.OperationCreateAnchor, elementfact.OperationUpdateAnchor:
			serial = token.RevisionSerial(outcome.Anchor.Revision)
		case elementfact.OperationCreateEntity, elementfact.OperationUpdateEntity:
			serial = token.RevisionSerial(outcome.Entity.Revision)
		case elementfact.OperationDeleteAnchor, elementfact.OperationDeleteEntity:
			serial = token.RevisionSerial(outcome.Tombstone)
		default:
			return storedOutcome{}, errors.New("committed operation kind is invalid")
		}
		stored.RevisionSerial = &serial
	case contract.MutationUnchanged:
		stored.Status = "unchanged"
	case contract.MutationRejected:
		stored.Status = "rejected"
		stored.Error = retainedErrorName(outcome.Rejection)
	default:
		return storedOutcome{}, errors.New("operation outcome status is invalid")
	}
	if stored.Error == "" && outcome.Status == contract.MutationRejected {
		return storedOutcome{}, errors.New("rejected outcome error is invalid")
	}
	return stored, nil
}

func (stored storedOutcome) outcome(scope [32]byte, binding commitbinding.Binding) (retainedOutcome, error) {
	cut := token.FromParts(scope, stored.CutSerial)
	if !token.Valid(cut) {
		return retainedOutcome{}, errors.New("outcome cut is invalid")
	}
	switch stored.Status {
	case "committed":
		if stored.Error != "" || stored.RevisionSerial == nil || *stored.RevisionSerial == 0 {
			return retainedOutcome{}, errors.New("committed outcome shape is invalid")
		}
		revision := token.RevisionFromParts(scope, binding.Element, *stored.RevisionSerial)
		switch binding.Kind {
		case elementfact.OperationCreateRoot:
			return committedAnchorOutcome(elementfact.Anchor{ID: binding.Element, Range: geometry.UnboundedRootRange(), Revision: revision}, cut), nil
		case elementfact.OperationCreateAnchor, elementfact.OperationUpdateAnchor:
			return committedAnchorOutcome(elementfact.Anchor{ID: binding.Element, Parent: binding.Final.Parent, LocalPose: binding.Final.LocalPose, Range: geometry.BoundedRange(binding.Final.Range), Revision: revision}, cut), nil
		case elementfact.OperationCreateEntity, elementfact.OperationUpdateEntity:
			return committedEntityOutcome(elementfact.Entity{ID: binding.Element, Parent: binding.Final.Parent, LocalPose: binding.Final.LocalPose, Range: binding.Final.Range, Revision: revision}, cut), nil
		case elementfact.OperationDeleteAnchor, elementfact.OperationDeleteEntity:
			return committedDeleteOutcome(revision, cut), nil
		default:
			return retainedOutcome{}, errors.New("committed operation kind is invalid")
		}
	case "unchanged":
		if stored.Error != "" || stored.RevisionSerial != nil {
			return retainedOutcome{}, errors.New("unchanged outcome shape is invalid")
		}
		switch binding.Kind {
		case elementfact.OperationUpdateAnchor:
			return unchangedAnchorOutcome(elementfact.Anchor{ID: binding.Element, Parent: binding.Final.Parent, LocalPose: binding.Final.LocalPose, Range: geometry.BoundedRange(binding.Final.Range), Revision: binding.Expected}, cut), nil
		case elementfact.OperationUpdateEntity:
			return unchangedEntityOutcome(elementfact.Entity{ID: binding.Element, Parent: binding.Final.Parent, LocalPose: binding.Final.LocalPose, Range: binding.Final.Range, Revision: binding.Expected}, cut), nil
		default:
			return retainedOutcome{}, errors.New("UNCHANGED is only valid for Update")
		}
	case "rejected":
		if stored.RevisionSerial != nil {
			return retainedOutcome{}, errors.New("rejected outcome contains a Revision")
		}
		errorKind := retainedErrorFromName(stored.Error)
		if errorKind == elementfact.NoError {
			return retainedOutcome{}, errors.New("rejected outcome error is invalid")
		}
		return rejectedOutcome(errorKind, cut), nil
	default:
		return retainedOutcome{}, errors.New("operation outcome status is unrecognized")
	}
}

func (stored storedOperation) operation(scope [32]byte) (retainedOperation, error) {
	binding, err := stored.Binding.binding()
	if err != nil {
		return retainedOperation{}, err
	}
	outcome, err := stored.Outcome.outcome(scope, binding)
	if err != nil {
		return retainedOperation{}, err
	}
	return retainedOperation{admission: token.FromParts(scope, stored.Admission), binding: binding, outcome: outcome}, nil
}

func retainedErrorName(kind elementfact.Rejection) string {
	switch kind {
	case elementfact.NotFound:
		return "not-found"
	case elementfact.Conflict:
		return "conflict"
	case elementfact.RevisionConflict:
		return "revision-conflict"
	default:
		return ""
	}
}

func retainedErrorFromName(name string) elementfact.Rejection {
	switch name {
	case "not-found":
		return elementfact.NotFound
	case "conflict":
		return elementfact.Conflict
	case "revision-conflict":
		return elementfact.RevisionConflict
	default:
		return elementfact.NoError
	}
}

func encodeOperationID(operationID contract.OperationID) ([]byte, error) {
	if !commitbinding.ValidOperationID(operationID) {
		return nil, errors.New("Core OperationID length is invalid")
	}
	return append([]byte(nil), []byte(operationID)...), nil
}
