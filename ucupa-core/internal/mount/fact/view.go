package fact

import (
	"iter"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// View reads the current outgoing and incoming mount relations. It has no
// candidate replacement, lifecycle, or initialization methods.
type View struct {
	outgoing map[values.ElementID]Record
	incoming Incoming
}

// Outgoing looks up one current outgoing relation by Element ID.
func (view View) Outgoing(id values.ElementID) (Record, bool) {
	record, ok := view.outgoing[id]
	return record, ok
}

// Record preserves a single-value map lookup: a missing relation yields zero.
// Use Outgoing when the caller needs to distinguish absence from a zero record.
func (view View) Record(id values.ElementID) Record { return view.outgoing[id] }

// OutgoingCount reports the number of current outgoing records.
func (view View) OutgoingCount() int { return len(view.outgoing) }

// Outgoings iterates value copies without exposing the backing map. Iteration
// order remains map order; callers that need ordering retain their existing
// explicit sort.
func (view View) Outgoings() iter.Seq2[values.ElementID, Record] {
	return func(yield func(values.ElementID, Record) bool) {
		for id, record := range view.outgoing {
			if !yield(id, record) {
				return
			}
		}
	}
}

// Incoming returns a value copy of the current incoming relation.
func (view View) Incoming() Incoming {
	return view.incoming
}

// HasContent preserves the current relationship predicate used by reads and write gates.
func HasContent(mounts View, id values.ElementID) bool {
	mount, exists := mounts.Outgoing(id)
	return exists && mount.State != contract.MountNone
}
