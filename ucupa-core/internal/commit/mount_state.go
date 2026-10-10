package commit

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

type MountBinding struct {
	Actor     values.PrincipalRef
	PeerActor values.PrincipalRef
	Command   contract.MountCommand
}

type MountOperation struct {
	Parent            transferfact.ParentControl
	CarrierBinding    transferfact.CarrierBinding
	CarrierSettlement transferfact.CarrierSettlement
	Carrier           transferfact.CarrierControl // exclusive parent-owned replacement variant
	Binding           MountBinding
	Request           contract.MountPrepareRequest
	Decision          contract.MountDecision
	Settled           bool
	DecisionCut       contract.CoreReadCut
	Changed           bool
	AbortCause        contract.MountAbortCause
	Record            elementfact.Anchor
	HasRecord         bool
	Prepared          contract.MountPrepared
	Closed            contract.MountClosed
}

func (op MountOperation) transferControl() bool {
	return op.Carrier != (transferfact.CarrierControl{}) || op.Parent != (transferfact.ParentControl{})
}

type PeerAttempt struct {
	Request  contract.MountPrepareRequest
	Prepared contract.MountPrepared
	Closed   contract.MountClosed
	IsClosed bool
}

type NextHopBinding struct {
	Actor    values.PrincipalRef
	Element  values.ElementID
	Key      contract.MountKey
	Expected contract.NextHopRevision
	NextHop  values.NextHop
}

type nextHopOperation struct {
	binding   NextHopBinding
	result    contract.NextHopUpdateResult
	errorKind elementfact.Rejection
}

type mountState struct {
	facts      mountfact.State
	operations map[contract.OperationID]MountOperation
	addresses  map[contract.OperationID]nextHopOperation
	attempts   map[contract.MountAttemptKey]PeerAttempt
	events     []MountEvent
}

type MountEventKind uint8

const (
	MountEventReserve MountEventKind = iota + 1
	MountEventPrepare
	MountEventDecide
	MountEventFinish
	MountEventSettle
	MountEventAddress
)

// The journal includes no-cut outcomes. beforeCut/afterCut give their exact
// position relative to Element commits; external decisions are replay inputs.
type MountEvent struct {
	Kind       MountEventKind
	BeforeCut  contract.CoreReadCut
	AfterCut   contract.CoreReadCut
	Operation  contract.OperationID
	Binding    MountBinding
	Request    contract.MountPrepareRequest
	Prepared   contract.MountPrepared
	Closed     contract.MountClosed
	Finish     contract.MountFinishRequest
	Address    NextHopBinding
	Source     values.FactAuthorityRef
	Authorized bool
}

func newMountState() mountState {
	return mountState{
		facts:      mountfact.New(),
		operations: make(map[contract.OperationID]MountOperation),
		addresses:  make(map[contract.OperationID]nextHopOperation),
		attempts:   make(map[contract.MountAttemptKey]PeerAttempt),
	}
}

func (mount mountState) clone() mountState {
	cloned := mountState{
		facts:      mount.facts.Clone(),
		operations: make(map[contract.OperationID]MountOperation),
		addresses:  make(map[contract.OperationID]nextHopOperation),
		attempts:   make(map[contract.MountAttemptKey]PeerAttempt),
	}
	for key, record := range mount.operations {
		cloned.operations[key] = record
	}
	for key, record := range mount.addresses {
		cloned.addresses[key] = record
	}
	for key, record := range mount.attempts {
		cloned.attempts[key] = record
	}
	cloned.events = append([]MountEvent(nil), mount.events...)
	return cloned
}

func (state *coreState) mountOperationClaimed(id contract.OperationID) bool {
	_, control := state.mount.operations[id]
	_, address := state.mount.addresses[id]
	return control || address || state.attachmentClaimed(id) || state.transferClaimed(id) || state.realmContact.claimed(id)
}

func (operation MountOperation) Result(id contract.OperationID) contract.MountControlResult {
	result := contract.MountControlResult{
		OperationID: id, MountAnchorUUID: operation.Binding.Command.MountAnchorUUID,
		MountKey: operation.Request.Key, Decision: operation.Decision, Settled: operation.Settled,
		DecisionCut: operation.DecisionCut, Changed: operation.Changed, AbortCause: operation.AbortCause,
	}
	if operation.HasRecord {
		record := operation.Record.Record()
		result.Record = &record
	}
	return result
}

// These lookups return one copied receipt, never the owner's retained maps.
func (owner *Owner) MountOperationLocked(id contract.OperationID) (MountOperation, bool) {
	value, ok := owner.state.mount.operations[id]
	if value.transferControl() {
		return MountOperation{}, false
	}
	return value, ok
}
func (owner *Owner) PeerAttemptLocked(key contract.MountAttemptKey) (PeerAttempt, bool) {
	value, ok := owner.state.mount.attempts[key]
	return value, ok
}
func (owner *Owner) PendingMountsLocked() ([]contract.OperationID, []contract.MountAttemptKey) {
	var operations []contract.OperationID
	var attempts []contract.MountAttemptKey
	for id, op := range owner.state.mount.operations {
		if !op.Settled && !op.transferControl() {
			operations = append(operations, id)
		}
	}
	for key, attempt := range owner.state.mount.attempts {
		if attempt.Prepared.Prepared && !attempt.IsClosed {
			attempts = append(attempts, key)
		}
	}
	return operations, attempts
}
