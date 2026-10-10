package record

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

type StoredMountCommand struct {
	Kind            contract.MountCommandKind `json:"kind"`
	Anchor          string                    `json:"anchor"`
	ExpectedElement *StoredRevision           `json:"expectedElement,omitempty"`
	ExpectedMount   *StoredRevision           `json:"expectedMount,omitempty"`
	Carrier         string                    `json:"carrier,omitempty"`
	CarrierNextHop  string                    `json:"carrierNextHop,omitempty"`
	ParentNextHop   string                    `json:"parentNextHop,omitempty"`
	Final           *StoredTuple              `json:"final,omitempty"`
}

type StoredMountRequest struct {
	Key          *StoredMountKey       `json:"key,omitempty"`
	Operation    []byte                `json:"operation"`
	Command      StoredMountCommand    `json:"command"`
	ParentActor  StoredMountPrincipal  `json:"parentActor"`
	CarrierActor StoredMountPrincipal  `json:"carrierActor"`
	Previous     *StoredMountAdmission `json:"previous,omitempty"`
	Reserved     *StoredRevision       `json:"reserved,omitempty"`
	Tuple        *StoredTuple          `json:"tuple,omitempty"`
}

type StoredMountPrepared struct {
	Request   *StoredMountRequest      `json:"request"`
	Prepared  bool                     `json:"prepared"`
	Rejection contract.MountAbortCause `json:"rejection"`
	Cut       *StoredMountCut          `json:"cut"`
}

type StoredMountEvidence struct {
	Request    *StoredMountRequest      `json:"request"`
	Decision   contract.MountDecision   `json:"decision"`
	Cut        *StoredMountCut          `json:"cut"`
	AbortCause contract.MountAbortCause `json:"abortCause"`
	Admission  *StoredMountAdmission    `json:"admission,omitempty"`
}

type StoredMountClosed struct {
	Evidence *StoredMountEvidence `json:"evidence"`
	Cut      *StoredMountCut      `json:"cut"`
}

func StoreMountCommand(command contract.MountCommand) StoredMountCommand {
	return StoredMountCommand{
		Kind: command.Kind, Anchor: string(command.MountAnchorUUID),
		ExpectedElement: optionalStoreRevision(command.ExpectedElementRevision),
		ExpectedMount:   StoreMountRevision(command.ExpectedMountRevision),
		Carrier:         string(command.CarrierAuthority), CarrierNextHop: string(command.CarrierNextHop),
		ParentNextHop: string(command.ParentNextHop), Final: optionalStoreTuple(command.Final),
	}
}

func (stored StoredMountCommand) Command() (contract.MountCommand, error) {
	command := contract.MountCommand{
		Kind: stored.Kind, MountAnchorUUID: values.ElementID(stored.Anchor),
		CarrierAuthority: values.FactAuthorityRef(stored.Carrier),
		CarrierNextHop:   values.NextHop(stored.CarrierNextHop), ParentNextHop: values.NextHop(stored.ParentNextHop),
	}
	var err error
	if command.ExpectedElementRevision, err = optionalLoadRevision(stored.ExpectedElement); err != nil {
		return contract.MountCommand{}, err
	}
	if command.ExpectedMountRevision, err = LoadMountRevision(stored.ExpectedMount); err != nil {
		return contract.MountCommand{}, err
	}
	if command.Final, err = optionalLoadTuple(stored.Final); err != nil {
		return contract.MountCommand{}, err
	}
	return command, nil
}

func StoreMountRequest(request contract.MountPrepareRequest) *StoredMountRequest {
	if request == (contract.MountPrepareRequest{}) {
		return nil
	}
	return &StoredMountRequest{
		Key: StoreMountKey(request.Key), Operation: []byte(request.OperationID),
		Command: StoreMountCommand(request.Command), ParentActor: StoreMountPrincipal(request.ParentActor),
		CarrierActor: StoreMountPrincipal(request.CarrierActor), Previous: StoreMountAdmission(request.PreviousAdmission),
		Reserved: StoreMountRevision(request.ReservedRevision), Tuple: optionalStoreTuple(request.Tuple),
	}
}

func (stored *StoredMountRequest) Request() (contract.MountPrepareRequest, error) {
	if stored == nil {
		return contract.MountPrepareRequest{}, nil
	}
	request := contract.MountPrepareRequest{OperationID: contract.OperationID(string(stored.Operation)), ParentActor: stored.ParentActor.DecodePrincipal(), CarrierActor: stored.CarrierActor.DecodePrincipal()}
	var err error
	if request.Key, err = stored.Key.Key(); err != nil {
		return contract.MountPrepareRequest{}, err
	}
	if request.Command, err = stored.Command.Command(); err != nil {
		return contract.MountPrepareRequest{}, err
	}
	if request.PreviousAdmission, err = stored.Previous.Admission(); err != nil {
		return contract.MountPrepareRequest{}, err
	}
	if request.ReservedRevision, err = LoadMountRevision(stored.Reserved); err != nil {
		return contract.MountPrepareRequest{}, err
	}
	if request.Tuple, err = optionalLoadTuple(stored.Tuple); err != nil {
		return contract.MountPrepareRequest{}, err
	}
	return request, nil
}

func StoreMountPrepared(prepared contract.MountPrepared) *StoredMountPrepared {
	if prepared == (contract.MountPrepared{}) {
		return nil
	}
	return &StoredMountPrepared{Request: StoreMountRequest(prepared.Request), Prepared: prepared.Prepared, Rejection: prepared.Rejection, Cut: StoreMountCut(prepared.Cut)}
}

func (stored *StoredMountPrepared) DecodePrepared() (contract.MountPrepared, error) {
	if stored == nil {
		return contract.MountPrepared{}, nil
	}
	request, err := stored.Request.Request()
	if err != nil {
		return contract.MountPrepared{}, err
	}
	cut, err := stored.Cut.Cut()
	if err != nil {
		return contract.MountPrepared{}, err
	}
	return contract.MountPrepared{Request: request, Prepared: stored.Prepared, Rejection: stored.Rejection, Cut: cut}, nil
}

func StoreMountEvidence(evidence contract.MountDecisionEvidence) *StoredMountEvidence {
	if evidence == (contract.MountDecisionEvidence{}) {
		return nil
	}
	return &StoredMountEvidence{Request: StoreMountRequest(evidence.Request), Decision: evidence.Decision, Cut: StoreMountCut(evidence.DecisionCut), AbortCause: evidence.AbortCause, Admission: StoreMountAdmission(evidence.AdmissionKey)}
}

func (stored *StoredMountEvidence) Evidence() (contract.MountDecisionEvidence, error) {
	if stored == nil {
		return contract.MountDecisionEvidence{}, nil
	}
	request, err := stored.Request.Request()
	if err != nil {
		return contract.MountDecisionEvidence{}, err
	}
	cut, err := stored.Cut.Cut()
	if err != nil {
		return contract.MountDecisionEvidence{}, err
	}
	admission, err := stored.Admission.Admission()
	if err != nil {
		return contract.MountDecisionEvidence{}, err
	}
	return contract.MountDecisionEvidence{Request: request, Decision: stored.Decision, DecisionCut: cut, AbortCause: stored.AbortCause, AdmissionKey: admission}, nil
}

func StoreMountClosed(closed contract.MountClosed) *StoredMountClosed {
	if closed == (contract.MountClosed{}) {
		return nil
	}
	return &StoredMountClosed{Evidence: StoreMountEvidence(closed.Evidence), Cut: StoreMountCut(closed.Cut)}
}

func (stored *StoredMountClosed) Closed() (contract.MountClosed, error) {
	if stored == nil {
		return contract.MountClosed{}, nil
	}
	evidence, err := stored.Evidence.Evidence()
	if err != nil {
		return contract.MountClosed{}, err
	}
	cut, err := stored.Cut.Cut()
	if err != nil {
		return contract.MountClosed{}, err
	}
	return contract.MountClosed{Evidence: evidence, Cut: cut}, nil
}
