package fact

import "ucupa/shared/values"

// State owns only the current outgoing and incoming mount relations.
// Operations, attempts, address receipts, and the event journal stay with the
// Core commit/history owner.
type State struct{ View }

// New returns an initialized, empty relationship state.
func New() State {
	return State{View: View{outgoing: make(map[values.ElementID]Record)}}
}

// Initialized reports whether the outgoing table has the same initialization
// state as a freshly constructed mount state. A zero State remains a valid
// read-only representation for malformed/projection candidates.
func (s State) Initialized() bool {
	return s.View.outgoing != nil
}

// Clone returns an independent candidate. As with the former mountState clone,
// cloning a zero state initializes its outgoing table.
func (s State) Clone() State {
	clone := New()
	clone.View.incoming = s.View.incoming
	for id, record := range s.View.outgoing {
		clone.View.outgoing[id] = record
	}
	return clone
}

// Equal preserves the old value/map comparison semantics, including treating
// nil and empty outgoing maps as equal while comparing every key and value.
func (s State) Equal(other State) bool {
	if s.View.incoming != other.View.incoming || len(s.View.outgoing) != len(other.View.outgoing) {
		return false
	}
	for id, record := range s.View.outgoing {
		otherRecord, ok := other.View.outgoing[id]
		if !ok || otherRecord != record {
			return false
		}
	}
	return true
}

// IncomingOnly builds the original parent-boundary query projection. It does
// not retain or allocate outgoing records or any operation history.
func IncomingOnly(incoming Incoming) State { return State{View: View{incoming: incoming}} }

// ReplaceOutgoing replaces one relation in a transaction candidate. The
// caller must use New or Clone before mutation; a zero State deliberately
// retains the former nil-map assignment behavior.
func (s *State) ReplaceOutgoing(id values.ElementID, record Record) {
	s.View.outgoing[id] = record
}

// ReplaceIncoming replaces the complete incoming relation in a transaction
// candidate. Callers must first mutate a local value returned by Incoming,
// then replace it once the transition has passed all checks.
func (s *State) ReplaceIncoming(incoming Incoming) {
	s.View.incoming = incoming
}
