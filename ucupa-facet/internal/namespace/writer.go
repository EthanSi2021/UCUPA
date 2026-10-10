package namespace

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"reflect"

	bolt "go.etcd.io/bbolt"
	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func (f *Owner) InspectWriter(ctx context.Context, c values.CallerContext, id string) (values.WriterStatus, error) {
	return f.writerCall(ctx, c, id, nil)
}
func (f *Owner) HandoffWriter(ctx context.Context, c values.CallerContext, r values.WriterHandoffRequest) (values.WriterStatus, error) {
	if !r.Valid() {
		return values.WriterStatus{}, values.PublicInvalid
	}
	return f.writerCall(ctx, c, r.Operation, &r)
}
func (f *Owner) writerCall(ctx context.Context, c values.CallerContext, id string, r *values.WriterHandoffRequest) (values.WriterStatus, error) {
	if f == nil || f.ownerState == nil || ctx == nil || !c.Subject.Valid() || len(id) > 128 {
		return values.WriterStatus{}, values.PublicInvalid
	}
	if f.public == nil {
		return values.WriterStatus{}, values.PublicUnavailable
	}
	// The existing public counter bounds control too. Fencing expires contexts,
	// but must not disable the independent, authorized inspection path.
	if !f.public.Enter() {
		return values.WriterStatus{}, values.PublicResourceExhausted
	}
	defer f.public.Leave()
	permission := contract.PermissionInspectWriter
	if r != nil {
		permission = contract.PermissionHandoffWriter
	}
	req := []values.Requirement{{Permission: permission, Scope: values.AuthorityScope(f.authority)}}
	permit, err := f.authorizePublic(ctx, c, req)
	if err != nil {
		return values.WriterStatus{}, err
	}
	defer permit.release(ctx)
	defer f.public.DrainRetired(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil || f.corrupt || f.store == nil || f.store.db == nil {
		return values.WriterStatus{}, values.PublicUnavailable
	}
	if err = permit.guardLocked(ctx, false); err != nil {
		return values.WriterStatus{}, err
	}
	if r == nil {
		return f.store.writer.status(id)
	}
	result, err := f.handoffWriterLocked(c.Subject, *r)
	if f.fenced {
		f.public.CloseLocked()
	}
	return result, err
}
func (f *Owner) handoffWriterLocked(caller values.PrincipalRef, r values.WriterHandoffRequest) (values.WriterStatus, error) {
	s := f.store
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
	err := s.db.Update(func(tx *bolt.Tx) error {
		if e := s.writerGate(tx, f.authority); e != nil {
			return e
		}
		cut := sha256.Sum256(tx.Bucket(recordBucketName).Get(documentKey))
		next.Records = append(next.Records, writerRecord{Request: r, Caller: caller, FinalCut: cut})
		if e := putWriter(tx, next); e != nil {
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
		j, e := loadWriter(s.db, f.authority)
		doc, de := loadStore(s.db)
		if e != nil || de != nil || !reflect.DeepEqual(doc.Facts, f.state) || !reflect.DeepEqual(doc.Events, f.events) ||
			(!sameWriter(j, before) && !sameWriter(j, next)) {
			f.corrupt = true
			f.fenced = true
			return values.WriterStatus{}, errors.Join(values.PublicUnavailable, err, e, de)
		}
		s.writer = j
		f.fenced = j.pending()
		if errors.Is(err, values.PublicResourceExhausted) {
			return values.WriterStatus{}, values.PublicResourceExhausted
		}
		return values.WriterStatus{}, errors.Join(values.PublicIndeterminate, err)
	}
	s.writer = next
	f.fenced = true
	return next.status(r.Operation)
}
