package commit

import (
	"errors"
	"go.etcd.io/bbolt"
	"math"
	"ucupa/shared/values"
)

// WriterStatusLocked deliberately survives ordinary fact fencing. It is called
// only through the independently authorized lifecycle control entry.
func (o *Owner) WriterStatusLocked(operation string) (values.WriterStatus, error) {
	if o == nil || o.closed || o.state == nil || o.store == nil || o.store.db == nil {
		return values.WriterStatus{}, values.PublicUnavailable
	}
	return o.store.writer.status(operation)
}
func (o *Owner) WriterFencedLocked() bool { return o != nil && o.fenced }

func (o *Owner) HandoffWriterLocked(caller values.PrincipalRef, r values.WriterHandoffRequest) (values.WriterStatus, error) {
	if !caller.Valid() || !r.Valid() {
		return values.WriterStatus{}, values.PublicInvalid
	}
	if _, err := o.WriterStatusLocked(""); err != nil {
		return values.WriterStatus{}, err
	}
	s := o.store
	before := s.writer
	for _, record := range before.Records {
		if record.Request.Operation == r.Operation {
			if record.Request != r || record.Caller != caller {
				return values.WriterStatus{}, values.PublicConflict
			}
			return before.status(r.Operation)
		}
	}
	if !before.Handoff {
		return values.WriterStatus{}, values.PublicUnavailable
	}
	if before.pending() || before.Epoch != r.ExpectedEpoch {
		return values.WriterStatus{}, values.PublicConflict
	}
	if before.Epoch == math.MaxUint64 || len(before.Records) >= maxWriterDecisions {
		return values.WriterStatus{}, values.PublicResourceExhausted
	}
	for _, record := range before.Records {
		if record.Request.Target == r.Target {
			return values.WriterStatus{}, values.PublicConflict
		}
	}
	next := before
	next.Records = append([]writerRecord(nil), before.Records...)
	err := s.db.Update(func(tx *bbolt.Tx) error {
		if e := s.writerGate(tx); e != nil {
			return e
		}
		cut, e := writerCut(tx)
		if e != nil {
			return e
		}
		next.Records = append(next.Records, writerRecord{Request: r, Caller: caller, FinalCut: cut})
		if e = putWriter(tx, next); e != nil {
			return e
		}
		if s.failBeforeCommit != nil {
			fail := s.failBeforeCommit
			s.failBeforeCommit = nil
			return fail()
		}
		return nil
	})
	if err == nil && s.failAfterCommit != nil {
		fail := s.failAfterCommit
		s.failAfterCommit = nil
		err = fail()
	}
	if err != nil {
		recovered, e := loadWriter(s.db, s.authority)
		state, se := s.reload()
		if e != nil || se != nil || !sameCompleteCoreState(state, o.state) || (!sameWriter(recovered, before) && !sameWriter(recovered, next)) {
			o.state = nil
			o.fenced = true
			return values.WriterStatus{}, errors.Join(values.PublicUnavailable, err, e, se)
		}
		s.writer = recovered
		o.fenced = recovered.pending()
		if errors.Is(err, values.PublicResourceExhausted) {
			return values.WriterStatus{}, values.PublicResourceExhausted
		}
		return values.WriterStatus{}, errors.Join(values.PublicIndeterminate, err)
	}
	s.writer = next
	o.fenced = true
	return next.status(r.Operation)
}
