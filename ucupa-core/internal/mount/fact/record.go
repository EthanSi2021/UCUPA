// Package fact owns the current deterministic mount relationship facts.
//
// It deliberately does not own retained operations, peer attempts, address
// receipts, the control journal, Core cuts, authorization, Store, or I/O.
package fact

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// Contact is the current address and its owner-local CAS revision.
// NextHopRevision remains opaque; only the mount owner may construct or
// inspect its private representation.
type Contact struct {
	NextHop  values.NextHop
	Revision contract.NextHopRevision
}

// Record is one current outgoing mount relation. Slot and Admission are
// current facts, even though their operation identities are retained by the
// Core commit owner.
type Record struct {
	State     contract.MountState
	Key       contract.MountKey
	Revision  contract.MountRevision
	Admission contract.MountAdmissionKey
	Slot      contract.OperationID
	Contact   Contact
}

// Gate is the current write-gate state of an incoming relation.
// The zero value is closed, matching the former mountGate representation.
type Gate uint8

const (
	GateClosed Gate = iota
	GatePreparing
	GateOpen
	GateFenced      // source withdrawal; not an ordinary detach or prepared update
	GateReparenting // content stays local; its departed external parent cannot reopen this gate
)

// Admission is the current accepted range evidence for an incoming relation.
type Admission struct {
	Key         contract.MountAdmissionKey
	RangeValue  contract.BoundedSphere
	DecisionCut contract.CoreReadCut
}

// Incoming is the current incoming relation and gate state. Acceptance is an
// exact protocol value; it is not a second copy of Element facts.
type Incoming struct {
	Key        contract.MountKey
	Acceptance contract.MountPrepareRequest
	AcceptedAt contract.CoreReadCut
	Admission  Admission
	Gate       Gate
	Preparing  contract.MountAttemptKey
	Contact    Contact
	Detached   bool
}
