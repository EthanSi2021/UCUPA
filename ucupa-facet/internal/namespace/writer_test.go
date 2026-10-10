package namespace

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	bolt "go.etcd.io/bbolt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

type writerAuth struct {
	*testAuthorization
	decision values.AuthorizationDecision
	validate *values.AuthorizationDecision
}

func TestWriterChangedDuringUncertainFacetCommitStopsDisclosure(t *testing.T) {
	x, _, _, _ := newWriterFixture(t)
	i := values.WriterInstallation{Operation: "fence", Secret: [32]byte{9}}
	x.f.store.failAfterCommit = func() error {
		if err := x.f.store.db.Update(func(tx *bolt.Tx) error {
			j := x.f.store.writer
			j.Records = append(j.Records, writerRecord{Request: values.WriterHandoffRequest{Operation: i.Operation, ExpectedEpoch: 1, Target: i.Commitment()}, Caller: testActor, FinalCut: sha256.Sum256(tx.Bucket(recordBucketName).Get(documentKey))})
			return putWriter(tx, j)
		}); err != nil {
			t.Fatal(err)
		}
		return errors.New("lost response with changed durable writer")
	}
	_, err := x.f.Put(context.Background(), testActor, x.key, x.typ, fixturePayload("truth"), contract.ExpectAbsent(), "write")
	if err == nil {
		t.Fatal("uncertain write reported success")
	}
	if _, err = x.f.Get(context.Background(), testActor, x.key, x.typ); err == nil {
		t.Fatal("business reload revived the fenced writer")
	}
}

func (a *writerAuth) AuthorizeView(context.Context, values.NextHop, values.RealmID, values.CallerContext, ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	return a.decision, values.AuthorizationView{1}
}
func (a *writerAuth) ValidateView(context.Context, values.NextHop, values.RealmID, values.CallerContext, values.AuthorizationView, ...values.Requirement) values.AuthorizationDecision {
	if a.validate != nil {
		return *a.validate
	}
	return a.decision
}

func TestWriterFacetRechecksAuthorizationAndInFlightDisclosure(t *testing.T) {
	x, auth, _, _ := newWriterFixture(t)
	x.put(t, "before", "truth", contract.ExpectAbsent())
	ctx := context.Background()
	caller := values.CallerContext{Subject: testActor}
	i := values.WriterInstallation{Operation: "handoff", Secret: [32]byte{22}}
	request := values.WriterHandoffRequest{Operation: i.Operation, ExpectedEpoch: 1, Target: i.Commitment()}
	changed := values.AuthorizationChanged
	auth.validate = &changed
	if _, err := x.f.HandoffWriter(ctx, caller, request); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal("changed view admitted", err)
	}
	if x.f.fenced || len(x.f.store.writer.Records) != 0 {
		t.Fatal("failed permission check retained decision")
	}
	auth.validate = nil
	entered, release := make(chan struct{}), make(chan struct{})
	x.source.mu.Lock()
	x.source.hook = func() { close(entered); <-release }
	x.source.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		out, err := x.f.Get(ctx, testActor, x.key, x.typ)
		if len(out.Payload) != 0 {
			done <- errors.New("in-flight read leaked payload")
			return
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("read did not reach external check")
	}
	_, err := x.f.HandoffWriter(ctx, caller, request)
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("in-flight read succeeded after fence")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("read did not finish")
	}
}

func newWriterFixture(t *testing.T) (*fixture, *writerAuth, *values.PublicAccessPolicy, *values.WriterStartup) {
	t.Helper()
	cfg := &values.WriterStartup{Handoff: true}
	limits := &values.PublicAccessPolicy{ContextTTL: time.Minute, ObservationTTL: time.Minute}
	x := &fixture{auth: &testAuthorization{}, source: &testSource{address: "local:core"}, typ: fixtureType("example"), path: filepath.Join(t.TempDir(), "facet.db")}
	auth := &writerAuth{testAuthorization: x.auth, decision: values.AuthorizationValid}
	var err error
	x.f, err = CreateDurable(auth, x.source, testInitial, x.path, limits, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(x.f) })
	x.key = testKey(x.f.AuthorityRef(), testCore, testElement, 2)
	x.source.current = testProof(x.key, contract.CoreNone, 2)
	x.source.current.Pending = x.key
	x.accepted, err = x.f.AcceptAttachment(context.Background(), testActor, x.key, "local:core", "accept")
	if err != nil {
		t.Fatal(err)
	}
	x.source.current = testProof(x.key, contract.CoreActive, 3)
	return x, auth, limits, cfg
}
func TestWriterHandoffKeepsFacetContentAndRetainedResults(t *testing.T) {
	x, auth, limits, cfg := newWriterFixture(t)
	ctx := context.Background()
	caller := values.CallerContext{Subject: testActor}
	first := x.put(t, "first", "one", contract.ExpectAbsent())
	unchanged := x.put(t, "unchanged", "one", contract.ExpectRevision(first.Revision))
	before := x.f.state.clone()
	events := append([]event(nil), x.f.events...)
	install := values.WriterInstallation{Operation: "handoff", Secret: [32]byte{1}}
	request := values.WriterHandoffRequest{Operation: install.Operation, ExpectedEpoch: 1, Target: install.Commitment()}
	s, err := x.f.HandoffWriter(ctx, caller, request)
	if err != nil || !s.Pending || s.Epoch != 1 {
		t.Fatalf("fence %+v %v", s, err)
	}
	if _, err := x.f.Get(ctx, testActor, x.key, x.typ); err == nil {
		t.Fatal("old content disclosed")
	}
	if _, err := x.f.Put(ctx, testActor, x.key, x.typ, fixturePayload("one"), contract.ExpectAbsent(), "first"); err == nil {
		t.Fatal("old writer replayed facts")
	}
	if err := x.f.store.save(x.f.authority, x.f.scope, x.f.state, x.f.events); err == nil {
		t.Fatal("transaction bypassed writer gate")
	}
	again, e := x.f.HandoffWriter(ctx, caller, request)
	if e != nil || !reflect.DeepEqual(s, again) {
		t.Fatalf("retry %+v %v", again, e)
	}
	if _, e := x.f.InspectWriter(ctx, caller, install.Operation); e != nil {
		t.Fatalf("fence locked inspection: %v", e)
	}
	_ = Close(x.f)
	if f, e := OpenDurable(auth, x.source, x.path, limits, cfg); f != nil || !errors.Is(e, values.PublicUnavailable) {
		t.Fatalf("ordinary Open: %v", e)
	}
	wrong := *cfg
	wrong.Handoff = false
	wrong.Install = &install
	if f, e := OpenDurable(auth, x.source, x.path, limits, &wrong); f != nil || !errors.Is(e, values.PublicDenied) {
		t.Fatalf("changed Realm assembly: %v", e)
	}
	next := *cfg
	next.Install = &install
	x.f, err = OpenDurable(auth, x.source, x.path, limits, &next)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, x.f.state) || !reflect.DeepEqual(events, x.f.events) {
		t.Fatal("content/history changed")
	}
	if x.f.store.epoch != 2 {
		t.Fatal("epoch not installed")
	}
	got := x.put(t, "unchanged", "one", contract.ExpectRevision(first.Revision))
	if !reflect.DeepEqual(got, unchanged) {
		t.Fatal("unchanged result lost")
	}
	got = x.put(t, "first", "one", contract.ExpectAbsent())
	if !reflect.DeepEqual(got, first) {
		t.Fatal("original write result lost")
	}
	status, err := x.f.InspectWriter(ctx, caller, install.Operation)
	if err != nil || status.Pending || !status.Decision.Installed || status.Decision.FinalCut != s.Decision.FinalCut {
		t.Fatalf("install receipt %+v %v", status, err)
	}
	_ = Close(x.f)
	x.f, err = OpenDurable(auth, x.source, x.path, limits, &next)
	if err != nil {
		t.Fatal(err)
	}
	if x.f.store.epoch != 2 {
		t.Fatal("exact installation advanced epoch again")
	}
	second := values.WriterInstallation{Operation: "second", Secret: [32]byte{2}}
	_, err = x.f.HandoffWriter(ctx, caller, values.WriterHandoffRequest{Operation: second.Operation, ExpectedEpoch: 2, Target: second.Commitment()})
	if err != nil {
		t.Fatal(err)
	}
	_ = Close(x.f)
	if old, e := OpenDurable(auth, x.source, x.path, limits, &next); old != nil || !errors.Is(e, values.PublicDenied) {
		t.Fatalf("old secret took later generation: %v", e)
	}
}
func TestWriterFaultRecoveryAndAuthorization(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "committed-no-response"}[after], func(t *testing.T) {
			x, auth, limits, cfg := newWriterFixture(t)
			ctx := context.Background()
			caller := values.CallerContext{Subject: testActor}
			x.put(t, "payload", "truth", contract.ExpectAbsent())
			i := values.WriterInstallation{Operation: "handoff", Secret: [32]byte{4}}
			r := values.WriterHandoffRequest{Operation: i.Operation, ExpectedEpoch: 1, Target: i.Commitment()}
			auth.decision = values.AuthorizationDenied
			if _, err := x.f.HandoffWriter(ctx, caller, r); !errors.Is(err, values.PublicDenied) {
				t.Fatalf("unauthorized %v", err)
			}
			if x.f.fenced || len(x.f.store.writer.Records) != 0 {
				t.Fatal("denial retained work")
			}
			auth.decision = values.AuthorizationValid
			fail := func() error { return errors.New("injected") }
			if after {
				x.f.store.failAfterCommit = fail
			} else {
				x.f.store.failBeforeCommit = fail
			}
			_, err := x.f.HandoffWriter(ctx, caller, r)
			if !errors.Is(err, values.PublicIndeterminate) || x.f.fenced != after {
				t.Fatalf("fault fenced=%v %v", x.f.fenced, err)
			}
			if _, err = x.f.HandoffWriter(ctx, caller, r); err != nil {
				t.Fatal(err)
			}
			_ = Close(x.f)
			next := *cfg
			next.Install = &i
			x.f, err = OpenDurable(auth, x.source, x.path, limits, &next)
			if err != nil {
				t.Fatal(err)
			}
			got, err := x.f.Get(ctx, testActor, x.key, x.typ)
			if err != nil || fixtureText(got.Payload) != "truth" {
				t.Fatalf("lost contents %+v %v", got, err)
			}
		})
	}
}

func TestWriterFacetInstallationFaultRetryAdvancesOnlyOnce(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost-install-response"}[after], func(t *testing.T) {
			x, auth, limits, cfg := newWriterFixture(t)
			x.put(t, "before", "truth", contract.ExpectAbsent())
			i := values.WriterInstallation{Operation: "install-fault", Secret: [32]byte{71}}
			_, err := x.f.HandoffWriter(context.Background(), values.CallerContext{Subject: testActor}, values.WriterHandoffRequest{Operation: i.Operation, ExpectedEpoch: 1, Target: i.Commitment()})
			if err != nil {
				t.Fatal(err)
			}
			fail := func() error { return errors.New("installation fault") }
			if after {
				x.f.store.failAfterCommit = fail
			} else {
				x.f.store.failBeforeCommit = fail
			}
			target := *cfg
			target.Install = &i
			if _, err = x.f.store.installWriter(x.f.store.writer, &target); !errors.Is(err, values.PublicIndeterminate) {
				t.Fatal(err)
			}
			if err = x.f.available(); err == nil {
				t.Fatal("old owner reopened")
			}
			if err = Close(x.f); err != nil {
				t.Fatal(err)
			}
			x.f, err = OpenDurable(auth, x.source, x.path, limits, &target)
			if err != nil {
				t.Fatal(err)
			}
			if x.f.store.epoch != 2 || len(x.f.store.writer.Records) != 1 || x.f.store.writer.pending() {
				t.Fatal("installation duplicated or incomplete")
			}
			if out, err := x.f.Get(context.Background(), testActor, x.key, x.typ); err != nil || fixtureText(out.Payload) != "truth" {
				t.Fatalf("content lost %#v %v", out, err)
			}
		})
	}
}

func TestWriterFacetJournalRejectsCorruption(t *testing.T) {
	for _, kind := range []string{"missing", "epoch", "cut", "duplicate", "source", "owner"} {
		t.Run(kind, func(t *testing.T) {
			x, auth, limits, cfg := newWriterFixture(t)
			i := values.WriterInstallation{Operation: "damaged", Secret: [32]byte{61}}
			_, err := x.f.HandoffWriter(context.Background(), values.CallerContext{Subject: testActor}, values.WriterHandoffRequest{Operation: i.Operation, ExpectedEpoch: 1, Target: i.Commitment()})
			if err != nil {
				t.Fatal(err)
			}
			journal := x.f.store.writer
			if err = Close(x.f); err != nil {
				t.Fatal(err)
			}
			db, err := bolt.Open(x.path, 0600, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = db.Update(func(tx *bolt.Tx) error {
				b := tx.Bucket(recordBucketName)
				if kind == "missing" {
					return b.Delete(writerJournalKey)
				}
				switch kind {
				case "epoch":
					journal.Epoch++
				case "cut":
					journal.Records[0].FinalCut[0] ^= 1
				case "duplicate":
					journal.Records = append(journal.Records, journal.Records[0])
				case "source":
					journal.Records[0].Request = values.WriterHandoffRequest{}
				case "owner":
					journal.Owner = testCore
				}
				raw, err := json.Marshal(journal)
				if err != nil {
					return err
				}
				return b.Put(writerJournalKey, raw)
			})
			closeErr := db.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
			target := *cfg
			target.Install = &i
			if next, err := OpenDurable(auth, x.source, x.path, limits, &target); next != nil || err == nil {
				if next != nil {
					_ = Close(next)
				}
				t.Fatal("damaged journal admitted")
			}
		})
	}
}
