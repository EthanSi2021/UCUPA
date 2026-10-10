package commit

import (
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"path/filepath"
	"reflect"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
)

func writerFixture(t *testing.T) (*Owner, string, *values.WriterStartup, values.WriterInstallation) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.db")
	ref, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &values.WriterStartup{Handoff: true}
	o, err := CreateDurable(path, ref, [32]byte{1}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.CloseStore() })
	return o, path, cfg, values.WriterInstallation{Operation: "handoff", Secret: [32]byte{17}}
}
func writerRequest(i values.WriterInstallation, epoch uint64) values.WriterHandoffRequest {
	return values.WriterHandoffRequest{Operation: i.Operation, ExpectedEpoch: epoch, Target: i.Commitment()}
}
func TestWriterHandoffPreservesFactsAndOriginalOutcome(t *testing.T) {
	o, path, cfg, install := writerFixture(t)
	id := foundationStoreID(990)
	original, err := applyFoundationStore(o, elementfact.OperationCreateRoot, foundationStoreActor, id, contract.SpatialTuple{}, contract.Revision{}, "root")
	if err != nil {
		t.Fatal(err)
	}
	before := o.state.clone()
	o.Lock()
	status, err := o.HandoffWriterLocked(foundationStoreActor, writerRequest(install, 1))
	o.Unlock()
	if err != nil || !status.Pending || status.Epoch != 1 || status.Decision.FinalCut == [32]byte{} {
		t.Fatalf("%+v %v", status, err)
	}
	if _, err := applyFoundationStore(o, elementfact.OperationCreateRoot, foundationStoreActor, id, contract.SpatialTuple{}, contract.Revision{}, "root"); !errors.Is(err, contract.ErrStore) {
		t.Fatalf("old writer replay disclosed: %v", err)
	}
	if err := o.store.persist(before); err == nil {
		t.Fatal("actual transaction bypassed fence")
	}
	o.Lock()
	again, err := o.HandoffWriterLocked(foundationStoreActor, writerRequest(install, 1))
	o.Unlock()
	if err != nil || !reflect.DeepEqual(status, again) {
		t.Fatalf("exact retry: %+v %v", again, err)
	}
	wrong := writerRequest(install, 1)
	wrong.Target[0] ^= 1
	o.Lock()
	_, err = o.HandoffWriterLocked(foundationStoreActor, wrong)
	o.Unlock()
	if !errors.Is(err, values.PublicConflict) {
		t.Fatalf("ID rebound: %v", err)
	}
	closeFoundationStore(t, o)
	if opened, e := OpenDurable(path, cfg); opened != nil || !errors.Is(e, values.PublicUnavailable) {
		t.Fatalf("ordinary Open took closed writer: %v", e)
	}
	bad := *cfg
	bad.Install = &values.WriterInstallation{Operation: install.Operation, Secret: [32]byte{18}}
	if opened, e := OpenDurable(path, &bad); opened != nil || !errors.Is(e, values.PublicDenied) {
		t.Fatalf("wrong target: %v", e)
	}
	bad = *cfg
	bad.Handoff = false
	bad.Install = &install
	if opened, e := OpenDurable(path, &bad); opened != nil || !errors.Is(e, values.PublicDenied) {
		t.Fatalf("authorization switch: %v", e)
	}
	nextConfig := *cfg
	nextConfig.Install = &install
	next, err := OpenDurable(path, &nextConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer next.CloseStore()
	if next.store.epoch != 2 || !sameCompleteCoreState(before, next.state) {
		t.Fatal("handoff changed facts, history or signing material")
	}
	replay, err := applyFoundationStore(next, elementfact.OperationCreateRoot, foundationStoreActor, id, contract.SpatialTuple{}, contract.Revision{}, "root")
	if err != nil || !reflect.DeepEqual(replay, original) {
		t.Fatalf("original result lost: %+v %v", replay, err)
	}
	next.Lock()
	done, e := next.WriterStatusLocked(install.Operation)
	next.Unlock()
	if e != nil || done.Pending || !done.Decision.Installed || done.Decision.FinalCut != status.Decision.FinalCut {
		t.Fatalf("installed receipt %+v %v", done, e)
	}
	closeFoundationStore(t, next)
	reopened, e := OpenDurable(path, &nextConfig)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.store.epoch != 2 {
		t.Fatal("install retry advanced epoch")
	}
	closeFoundationStore(t, reopened)
	reopened, e = OpenDurable(path, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.store.epoch != 2 {
		t.Fatal("normal restart advanced epoch")
	}
	second := values.WriterInstallation{Operation: "second", Secret: [32]byte{19}}
	reopened.Lock()
	_, e = reopened.HandoffWriterLocked(foundationStoreActor, writerRequest(second, 2))
	reopened.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	closeFoundationStore(t, reopened)
	if old, e := OpenDurable(path, &nextConfig); old != nil || !errors.Is(e, values.PublicDenied) {
		t.Fatalf("old installation claimed successor: %v", e)
	}
}

func TestWriterFenceCommitFaultReconcilesWithoutReopening(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-lost"}[after], func(t *testing.T) {
			o, path, cfg, i := writerFixture(t)
			fail := func() error { return errors.New("injected") }
			if after {
				o.store.failAfterCommit = fail
			} else {
				o.store.failBeforeCommit = fail
			}
			o.Lock()
			_, err := o.HandoffWriterLocked(foundationStoreActor, writerRequest(i, 1))
			o.Unlock()
			if !errors.Is(err, values.PublicIndeterminate) || o.fenced != after {
				t.Fatalf("reconcile fenced=%v %v", o.fenced, err)
			}
			if after && o.AvailableLocked() {
				t.Fatal("uncertain commit opened old facts")
			}
			o.Lock()
			s, err := o.HandoffWriterLocked(foundationStoreActor, writerRequest(i, 1))
			o.Unlock()
			if err != nil || !s.Pending {
				t.Fatalf("recover original request: %+v %v", s, err)
			}
			closeFoundationStore(t, o)
			cfgCopy := *cfg
			cfgCopy.Install = &i
			next, err := OpenDurable(path, &cfgCopy)
			if err != nil {
				t.Fatal(err)
			}
			defer next.CloseStore()
			if len(next.store.writer.Records) != 1 || next.store.epoch != 2 {
				t.Fatal("duplicate decision")
			}
		})
	}
}

func TestWriterJournalRejectsCorruption(t *testing.T) {
	for _, kind := range []string{"missing", "epoch", "cut", "duplicate", "no-source"} {
		t.Run(kind, func(t *testing.T) {
			o, path, cfg, i := writerFixture(t)
			o.Lock()
			_, err := o.HandoffWriterLocked(foundationStoreActor, writerRequest(i, 1))
			o.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			closeFoundationStore(t, o)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				if kind == "missing" {
					_ = b.Delete(writerJournalKey)
					return
				}
				j, e := decodeWriter(b.Get(writerJournalKey), o.authority)
				if e != nil {
					t.Fatal(e)
				}
				switch kind {
				case "epoch":
					j.Epoch++
				case "cut":
					j.Records[0].FinalCut[0] ^= 1
				case "duplicate":
					j.Records = append(j.Records, j.Records[0])
				case "no-source":
					j.Records[0].Request = values.WriterHandoffRequest{}
				}
				putFoundationStoreJSON(t, b, writerJournalKey, j)
			})
			cfgCopy := *cfg
			cfgCopy.Install = &i
			if next, err := OpenDurable(path, &cfgCopy); next != nil || err == nil {
				t.Fatal("accepted damaged writer history")
			}
		})
	}
}

func TestWriterChangedDuringUncertainBusinessCommitStopsDisclosure(t *testing.T) {
	o, _, _, install := writerFixture(t)
	o.store.failAfterCommit = func() error {
		if err := o.store.db.Update(func(tx *bbolt.Tx) error {
			cut, err := writerCut(tx)
			if err != nil {
				return err
			}
			j := o.store.writer
			j.Records = append(j.Records, writerRecord{Request: writerRequest(install, 1), Caller: foundationStoreActor, FinalCut: cut})
			return putWriter(tx, j)
		}); err != nil {
			t.Fatal(err)
		}
		return errors.New("lost response with changed durable writer")
	}
	_, err := applyFoundationStore(o, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(991), contract.SpatialTuple{}, contract.Revision{}, "root")
	if err == nil {
		t.Fatal("uncertain write reported success")
	}
	if o.AvailableLocked() {
		t.Fatal("business reload revived the fenced writer")
	}
}

func TestWriterInstallationFaultRetryAdvancesOnlyOnce(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost-install-response"}[after], func(t *testing.T) {
			o, path, cfg, i := writerFixture(t)
			o.Lock()
			_, err := o.HandoffWriterLocked(foundationStoreActor, writerRequest(i, 1))
			o.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			fail := func() error { return errors.New("installation fault") }
			if after {
				o.store.failAfterCommit = fail
			} else {
				o.store.failBeforeCommit = fail
			}
			target := *cfg
			target.Install = &i
			if _, err = o.store.installWriter(o.store.writer, &target); !errors.Is(err, values.PublicIndeterminate) {
				t.Fatal(err)
			}
			if o.AvailableLocked() {
				t.Fatal("install transaction revived old owner")
			}
			closeFoundationStore(t, o)
			next, err := OpenDurable(path, &target)
			if err != nil {
				t.Fatal(err)
			}
			defer next.CloseStore()
			if next.store.epoch != 2 || len(next.store.writer.Records) != 1 || next.store.writer.pending() {
				t.Fatal("installation duplicated or incomplete")
			}
		})
	}
}

func TestWriterFenceSerializesConcurrentBusinessCommits(t *testing.T) {
	o, path, cfg, i := writerFixture(t)
	root := foundationStoreID(970)
	if _, err := applyFoundationStore(o, elementfact.OperationCreateRoot, foundationStoreActor, root, contract.SpatialTuple{}, contract.Revision{}, "root"); err != nil {
		t.Fatal(err)
	}
	tuple := foundationStoreTuple(t, root, 0, 1)
	type result struct {
		index int
		out   contract.MutationResult
		err   error
	}
	start, done := make(chan struct{}), make(chan result, 16)
	for n := 0; n < 16; n++ {
		go func(n int) {
			<-start
			out, err := applyFoundationStore(o, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(uint64(1000+n)), tuple, contract.Revision{}, contract.OperationID(fmt.Sprintf("parallel-%d", n)))
			done <- result{n, out, err}
		}(n)
	}
	fenced := make(chan error, 1)
	go func() {
		<-start
		o.Lock()
		_, err := o.HandoffWriterLocked(foundationStoreActor, writerRequest(i, 1))
		o.Unlock()
		fenced <- err
	}()
	close(start)
	if err := <-fenced; err != nil {
		t.Fatal(err)
	}
	outcomes := make([]result, 0, 16)
	for n := 0; n < 16; n++ {
		r := <-done
		if r.err != nil && !errors.Is(r.err, contract.ErrStore) {
			t.Fatal(r.err)
		}
		outcomes = append(outcomes, r)
	}
	// Possessing the target secret never bypasses the original Store's lock.
	target := *cfg
	target.Install = &i
	if next, err := OpenDurable(path, &target); next != nil || err == nil {
		if next != nil {
			_ = next.CloseStore()
		}
		t.Fatal("simultaneous target acquired original Store")
	}
	closeFoundationStore(t, o)
	next, err := OpenDurable(path, &target)
	if err != nil {
		t.Fatal(err)
	}
	defer next.CloseStore()
	for _, r := range outcomes {
		_, exists := next.state.elements.View.Entity(foundationStoreID(uint64(1000 + r.index)))
		if exists != (r.err == nil) {
			t.Fatal("final cut lost or invented a concurrent commit", r.index)
		}
		if r.err == nil {
			replay, err := applyFoundationStore(next, elementfact.OperationCreateEntity, foundationStoreActor, foundationStoreID(uint64(1000+r.index)), tuple, contract.Revision{}, contract.OperationID(fmt.Sprintf("parallel-%d", r.index)))
			if err != nil || !reflect.DeepEqual(replay, r.out) {
				t.Fatal("concurrent original outcome changed", r.index, err)
			}
		}
	}
}
