package access

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ucupa/shared/values"
)

const (
	accessTestOwner   values.FactAuthorityRef = "00000000-0000-4000-8000-00000000aa01"
	accessTestElement values.ElementID        = "00000000-0000-4000-8000-00000000aa02"
)

type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time { return c.now }

func testLimits() values.PublicLimits {
	return values.PublicLimits{
		ContextTTL:      time.Hour,
		ObservationTTL:  time.Hour,
		Contexts:        16,
		PerCaller:       8,
		Groups:          8,
		Bytes:           64 << 10,
		ConcurrentReads: 2,
	}
}

func testCaller(subject string, byteValue byte) values.CallerContext {
	return values.CallerContext{
		Subject:       values.PrincipalRef{Realm: "access-test", Principal: values.PrincipalID(subject)},
		Credential:    [16]byte{byteValue},
		Establishment: [32]byte{byteValue},
		Proof:         [32]byte{byteValue},
	}
}

func newTestManager(limits values.PublicLimits) (*Manager, *sync.Mutex, *testClock) {
	locker := &sync.Mutex{}
	clock := &testClock{now: time.Now()}
	m := New(accessTestOwner, limits, locker)
	m.now = clock.Now
	return m, locker, clock
}

func mustType(t *testing.T, value byte) values.FacetTypeRef {
	t.Helper()
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = value
	}
	typ, err := values.NewFacetTypeRef(raw)
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func mustToken(t *testing.T, owner values.FactAuthorityRef, kind string, value byte) values.PublicToken {
	t.Helper()
	var raw [32]byte
	raw[0] = value
	token, err := values.PublicTokenFromBytes(owner, kind, raw)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func issuePreparation(t *testing.T, m *Manager, caller values.CallerContext, typ values.FacetTypeRef) *Context {
	t.Helper()
	prep, err := m.IssueLocked(caller, "observation-preparation", nil)
	if err != nil {
		t.Fatal(err)
	}
	prep.Key.Element = accessTestElement
	prep.Typ = typ
	prep.Stream = &Stream{
		Active:        true,
		Address:       "local:core",
		Proposal:      mustToken(t, accessTestOwner, "attachment-revision", 21),
		ContactSerial: 7,
	}
	return prep
}

func TestNewDisabledAndConcurrentQuota(t *testing.T) {
	limits := testLimits()
	limits.ContextTTL = 0
	if m := New(accessTestOwner, limits, &sync.Mutex{}); m != nil {
		t.Fatal("zero ContextTTL must disable the manager")
	}

	limits = testLimits()
	limits.ConcurrentReads = 1
	m, locker, _ := newTestManager(limits)
	if !m.Enter() {
		t.Fatal("first concurrent read should enter")
	}
	if m.Enter() {
		t.Fatal("second concurrent read exceeded the limit")
	}
	m.Leave()
	if !m.Enter() {
		t.Fatal("slot was not released")
	}
	m.Leave()

	locker.Lock()
	defer locker.Unlock()
	m.CloseLocked()
}

func TestReserveAndIssueQuota(t *testing.T) {
	limits := testLimits()
	limits.Contexts = 4
	limits.PerCaller = 4
	limits.Bytes = 4096
	m, locker, _ := newTestManager(limits)
	locker.Lock()
	defer locker.Unlock()

	if err := m.ReserveLocked(4096); err != nil {
		t.Fatalf("reserve capacity: %v", err)
	}
	if err := m.ReserveLocked(1); err != values.PublicResourceExhausted {
		t.Fatalf("over-reserve: got %v, want %v", err, values.PublicResourceExhausted)
	}
	m.ReleaseBytesLocked(4096)
	if m.bytes != 0 {
		t.Fatalf("release left %d bytes", m.bytes)
	}

	caller := testCaller("writer", 1)
	if _, err := m.IssueLocked(caller, "observation-preparation", nil); err != nil {
		t.Fatalf("issue context: %v", err)
	}
	if err := m.ReserveLocked(2049); err != values.PublicResourceExhausted {
		t.Fatalf("reserve beyond remaining bytes: got %v, want %v", err, values.PublicResourceExhausted)
	}
	m.CloseLocked()
	if err := m.ReserveLocked(1); err != nil {
		t.Fatalf("closed manager changed resource-only reservation: %v", err)
	}
	m.ReleaseBytesLocked(1)
}

func TestIssuePerCallerAndParentLifetime(t *testing.T) {
	limits := testLimits()
	limits.ContextTTL = 10 * time.Minute
	limits.ObservationTTL = 5 * time.Minute
	limits.PerCaller = 2
	limits.Contexts = 8
	m, locker, clock := newTestManager(limits)
	locker.Lock()
	defer locker.Unlock()

	caller := testCaller("writer", 1)
	other := testCaller("reader", 2)
	lifecycleCaller := testCaller("observer", 3)
	first, err := m.IssueLocked(caller, "query-continuation", nil)
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	if _, err := m.IssueLocked(caller, "query-continuation", nil); err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if _, err := m.IssueLocked(caller, "query-continuation", nil); err != values.PublicResourceExhausted {
		t.Fatalf("same-subject quota: got %v, want %v", err, values.PublicResourceExhausted)
	}
	if _, err := m.IssueLocked(other, "query-continuation", nil); err != nil {
		t.Fatalf("different subject should have a separate quota: %v", err)
	}

	prep := issuePreparation(t, m, lifecycleCaller, values.FacetTypeRef{})
	child, err := m.IssueLocked(lifecycleCaller, "observation-cursor", prep)
	if err != nil {
		t.Fatalf("child issue: %v", err)
	}
	if child.Expires.After(prep.Expires) {
		t.Fatalf("child expiry %v exceeds parent expiry %v", child.Expires, prep.Expires)
	}
	if child.Parent != prep.Token.Value {
		t.Fatalf("child parent %q, want %q", child.Parent, prep.Token.Value)
	}

	if _, err := m.IssueLocked(other, "observation-cursor", &Context{}); err != values.PublicGap {
		t.Fatalf("unindexed parent: got %v, want %v", err, values.PublicGap)
	}
	m.PublishLocked(accessTestElement, nil, "contact")
	if len(prep.Stream.Groups) != 1 {
		t.Fatalf("published groups: got %d, want 1", len(prep.Stream.Groups))
	}

	clock.now = prep.Expires
	m.ExpireLocked()
	if m.LookupLocked(prep.Token.Value) != nil || m.LookupLocked(child.Token.Value) != nil {
		t.Fatal("parent expiry did not recursively remove contexts")
	}
	if !prep.Stream.Gap || prep.Stream.Groups != nil {
		t.Fatalf("parent expiry did not gap and clear stream: gap=%v groups=%v", prep.Stream.Gap, prep.Stream.Groups)
	}
	if want := uint64(3 * contextBytes); m.bytes != want {
		t.Fatalf("expiry accounting: got %d, want %d unrelated context bytes", m.bytes, want)
	}
	m.CloseLocked()
	_ = first
}

func TestResolveOwnerCallerAndGap(t *testing.T) {
	m, locker, _ := newTestManager(testLimits())
	locker.Lock()
	defer locker.Unlock()

	caller := testCaller("writer", 1)
	context, err := m.IssueLocked(caller, "query-continuation", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := m.ResolveLocked(caller, context.Token); err != nil || got != context {
		t.Fatalf("resolve own context: got %p, %v", got, err)
	}
	if _, err := m.ResolveLocked(testCaller("other", 2), context.Token); err != values.PublicDenied {
		t.Fatalf("wrong caller: got %v, want %v", err, values.PublicDenied)
	}
	if _, err := m.ResolveLocked(caller, mustToken(t, values.FactAuthorityRef("00000000-0000-4000-8000-00000000aa03"), "query-continuation", 3)); err != values.PublicInvalid {
		t.Fatalf("wrong owner: got %v, want %v", err, values.PublicInvalid)
	}
	if _, err := m.ResolveLocked(caller, values.PublicToken{Owner: accessTestOwner, Kind: "query-continuation", Value: "bad"}); err != values.PublicInvalid {
		t.Fatalf("invalid token: got %v, want %v", err, values.PublicInvalid)
	}
	if _, err := m.ResolveLocked(caller, mustToken(t, accessTestOwner, "query-continuation", 4)); err != values.PublicGap {
		t.Fatalf("missing token: got %v, want %v", err, values.PublicGap)
	}
	m.CloseLocked()
}

func TestPublishFiltersAndGaps(t *testing.T) {
	limits := testLimits()
	limits.Contexts = 8
	limits.PerCaller = 8
	m, locker, _ := newTestManager(limits)
	locker.Lock()
	defer locker.Unlock()

	caller := testCaller("writer", 1)
	typeOne := mustType(t, 1)
	typeTwo := mustType(t, 2)
	prepOne := issuePreparation(t, m, caller, typeOne)
	prepTwo := issuePreparation(t, m, caller, typeTwo)
	prepTwo.Stream.ContactSerial = 8
	prepUnfiltered := issuePreparation(t, m, caller, values.FacetTypeRef{})

	m.PublishLocked(accessTestElement, typeOne.Bytes(), "content")
	if len(prepOne.Stream.Groups) != 1 || len(prepTwo.Stream.Groups) != 0 || len(prepUnfiltered.Stream.Groups) != 1 {
		t.Fatalf("content type filter: got %d/%d/%d groups, want 1/0/1", len(prepOne.Stream.Groups), len(prepTwo.Stream.Groups), len(prepUnfiltered.Stream.Groups))
	}
	m.PublishLocked(accessTestElement, typeTwo.Bytes(), "attachment")
	if len(prepOne.Stream.Groups) != 2 || len(prepTwo.Stream.Groups) != 1 || len(prepUnfiltered.Stream.Groups) != 2 {
		t.Fatalf("attachment type handling: got %d/%d/%d groups, want 2/1/2", len(prepOne.Stream.Groups), len(prepTwo.Stream.Groups), len(prepUnfiltered.Stream.Groups))
	}
	m.PublishLocked(values.ElementID("00000000-0000-4000-8000-00000000aa09"), typeOne.Bytes(), "contact")
	if len(prepOne.Stream.Groups) != 2 || len(prepTwo.Stream.Groups) != 1 || len(prepUnfiltered.Stream.Groups) != 2 {
		t.Fatal("wrong element was published")
	}
	if prepOne.Stream.ContactSerial != 7 {
		t.Fatal("publication changed captured contact serial")
	}

	m.GapAllLocked()
	for _, prep := range []*Context{prepOne, prepTwo, prepUnfiltered} {
		if !prep.Stream.Gap || prep.Stream.Groups != nil {
			t.Fatalf("GapAllLocked did not clear stream: gap=%v groups=%v", prep.Stream.Gap, prep.Stream.Groups)
		}
	}
	if want := uint64(3 * contextBytes); m.bytes != want {
		t.Fatalf("gap group accounting: got %d, want %d", m.bytes, want)
	}
	m.CloseLocked()
}

func TestPublishCapacityGapsStream(t *testing.T) {
	limits := testLimits()
	limits.Contexts = 1
	limits.PerCaller = 1
	limits.Groups = 1
	limits.Bytes = contextBytes + groupBytes
	m, locker, _ := newTestManager(limits)
	locker.Lock()
	defer locker.Unlock()

	prep := issuePreparation(t, m, testCaller("writer", 1), values.FacetTypeRef{})
	m.PublishLocked(accessTestElement, nil, "contact")
	if len(prep.Stream.Groups) != 1 || prep.Stream.Gap {
		t.Fatalf("first publication: gap=%v groups=%d", prep.Stream.Gap, len(prep.Stream.Groups))
	}
	m.PublishLocked(accessTestElement, nil, "contact")
	if !prep.Stream.Gap || prep.Stream.Groups != nil {
		t.Fatalf("capacity publication did not gap stream: gap=%v groups=%v", prep.Stream.Gap, prep.Stream.Groups)
	}
	if m.bytes != contextBytes {
		t.Fatalf("capacity GAP left group charge: got %d, want %d", m.bytes, contextBytes)
	}
	m.CloseLocked()
}

func TestPageConsumesDeduplicatesAndEvicts(t *testing.T) {
	limits := testLimits()
	limits.Contexts = 8
	limits.PerCaller = 8
	m, locker, _ := newTestManager(limits)
	locker.Lock()
	defer locker.Unlock()

	caller := testCaller("writer", 1)
	prep := issuePreparation(t, m, caller, values.FacetTypeRef{})
	stream := prep.Stream
	for i := byte(1); i <= 3; i++ {
		m.PublishLocked(accessTestElement, []byte{i}, "contact")
	}
	entry, err := m.IssueLocked(caller, "observation-cursor", prep)
	if err != nil {
		t.Fatal(err)
	}
	entry.Stream = stream
	entry.Position = 0
	stream.Initial = entry.Token

	beforePageContexts := len(m.contexts)
	page, err := m.PageLocked(entry, prep, 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	groups := page.Groups
	if len(groups) != 2 {
		t.Fatalf("first page groups: %#v", groups)
	}
	if groups[0].(map[string]any)["owner"] != string(accessTestOwner) {
		t.Fatal("page group owner is not the manager owner")
	}
	if !page.NeedsCursor || page.Position != 2 || page.Next != (values.PublicToken{}) {
		t.Fatalf("first page cursor handoff: %+v", page)
	}
	if len(m.contexts) != beforePageContexts {
		t.Fatalf("PageLocked issued a cursor itself: before=%d after=%d", beforePageContexts, len(m.contexts))
	}
	nextContext, err := m.IssueLocked(caller, "observation-cursor", prep)
	if err != nil {
		t.Fatal(err)
	}
	nextContext.Stream = stream
	nextContext.Position = page.Position
	next := nextContext.Token

	repeated, err := m.PageLocked(entry, prep, 2)
	if err != nil {
		t.Fatalf("repeat page: %v", err)
	}
	if repeated.NeedsCursor || repeated.Next != next {
		t.Fatalf("same-position cursor was not reused: got %+v, want %v", repeated, next)
	}

	page, err = m.PageLocked(nextContext, prep, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	groups = page.Groups
	if len(groups) != 1 {
		t.Fatalf("second page groups: %#v", groups)
	}
	if m.LookupLocked(entry.Token.Value) != nil {
		t.Fatal("consumed old cursor was not evicted")
	}
	if stream.Base != 2 || len(stream.Groups) != 1 {
		t.Fatalf("second page state: base=%d groups=%d", stream.Base, len(stream.Groups))
	}

	if !page.NeedsCursor || page.Position != 3 || page.Next != (values.PublicToken{}) {
		t.Fatalf("second page cursor handoff: %+v", page)
	}
	lastContext, err := m.IssueLocked(caller, "observation-cursor", prep)
	if err != nil {
		t.Fatal(err)
	}
	lastContext.Stream = stream
	lastContext.Position = page.Position
	last := lastContext.Token
	empty, err := m.PageLocked(lastContext, prep, 2)
	if err != nil {
		t.Fatalf("empty terminal page: %v", err)
	}
	if got := empty.Groups; len(got) != 0 {
		t.Fatalf("terminal page returned %d groups", len(got))
	}
	if empty.NeedsCursor || empty.Next != last {
		t.Fatal("empty page changed its cursor")
	}
	m.CloseLocked()
}

type countingLocker struct {
	mu    sync.Mutex
	locks atomic.Int64
}

func (l *countingLocker) Lock() {
	l.locks.Add(1)
	l.mu.Lock()
}

func (l *countingLocker) Unlock() { l.mu.Unlock() }

func TestTimerAndCloseUseSuppliedLocker(t *testing.T) {
	limits := testLimits()
	limits.ContextTTL = 20 * time.Millisecond
	limits.ObservationTTL = time.Second
	limits.Contexts = 1
	limits.PerCaller = 1
	locker := &countingLocker{}
	m := New(accessTestOwner, limits, locker)
	caller := testCaller("writer", 1)

	locker.Lock()
	c, err := m.IssueLocked(caller, "query-continuation", nil)
	if err != nil {
		locker.Unlock()
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if m.LookupLocked(c.Token.Value) == nil {
		locker.Unlock()
		t.Fatal("timer changed contexts while the supplied lock was held")
	}
	locker.Unlock()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		locker.Lock()
		gone := m.LookupLocked(c.Token.Value) == nil
		locker.Unlock()
		if gone {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	locker.Lock()
	defer locker.Unlock()
	if m.LookupLocked(c.Token.Value) != nil {
		t.Fatal("timer did not expire context after acquiring the supplied lock")
	}
	if locker.locks.Load() < 2 {
		t.Fatalf("timer did not use supplied locker; lock count=%d", locker.locks.Load())
	}

	second := New(accessTestOwner, func() values.PublicLimits {
		l := testLimits()
		l.Contexts, l.PerCaller = 1, 1
		return l
	}(), locker)
	if _, err := second.IssueLocked(caller, "query-continuation", nil); err != nil {
		t.Fatal(err)
	}
	second.CloseLocked()
	if !second.ClosedLocked() || len(second.contexts) != 0 {
		t.Fatalf("close under supplied lock left manager live: closed=%v contexts=%d", second.ClosedLocked(), len(second.contexts))
	}
}
