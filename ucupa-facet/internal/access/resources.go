// Package access owns the bounded, in-memory public access resources of a
// Facet.  It deliberately contains no Facet facts, journal, Store, or owner
// decision logic.
package access

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"ucupa/shared/values"
)

const (
	contextBytes = uint64(2048)
	groupBytes   = uint64(512)
)

// Manager owns finite public contexts and observation streams.  Callers must
// hold the locker supplied to New before invoking any *Locked method.
type Manager struct {
	owner    values.FactAuthorityRef
	limits   values.PublicLimits
	now      func() time.Time
	locker   sync.Locker
	contexts map[string]*Context
	bytes    uint64
	active   atomic.Uint64
	closed   bool
	retired  []func(context.Context)
}

// Context is the real public context record used by the access resource
// manager.  The fields below are exported for the owning Facet's internal
// workflow; timer and accounting details remain private to this package.
type Context struct {
	Token        values.PublicToken
	Caller       values.CallerContext
	Expires      time.Time
	Parent       string
	View         values.AuthorizationView
	Requirements []values.Requirement
	Key          values.FacetAcceptanceKey
	Typ          values.FacetTypeRef
	Cut          uint64
	Stream       *Stream
	Position     uint64
	Release      func(context.Context)

	timer *time.Timer
	bytes uint64
}

// Stream is the real finite observation resource.  Its contact serial is a
// captured fact stamp; it does not import or mirror the namespace contact
// record.
type Stream struct {
	Gap             bool
	Active          bool
	Address         values.NextHop
	Proposal        values.PublicToken
	CorePreparation values.PublicToken
	CoreCursor      values.PublicToken
	Groups          []Group
	Base            uint64
	Initial         values.PublicToken
	ContactSerial   uint64
}

// Group is one bounded public change projection.
type Group struct {
	ID       values.PublicToken
	Basis    values.PublicToken
	Category string
}

// New creates the bounded access manager.  A zero ContextTTL disables public
// contexts and therefore returns nil, matching the existing Facet behavior.
func New(owner values.FactAuthorityRef, limits values.PublicLimits, locker sync.Locker) *Manager {
	if limits.ContextTTL == 0 {
		return nil
	}
	return &Manager{
		owner:    owner,
		limits:   limits,
		now:      time.Now,
		locker:   locker,
		contexts: map[string]*Context{},
	}
}

// Enter reserves one concurrent public operation slot.
func (m *Manager) Enter() bool {
	if m == nil {
		return false
	}
	if m.active.Add(1) > m.limits.ConcurrentReads {
		m.active.Add(^uint64(0))
		return false
	}
	return true
}

// Leave releases a slot previously accepted by Enter.
func (m *Manager) Leave() {
	if m == nil {
		return
	}
	m.active.Add(^uint64(0))
}

// Limits returns the immutable limits configured for this manager.
func (m *Manager) Limits() values.PublicLimits {
	if m == nil {
		return values.PublicLimits{}
	}
	return m.limits
}

// ClosedLocked reports whether the access manager has been retired.
func (m *Manager) ClosedLocked() bool {
	return m == nil || m.closed
}

// ReserveLocked charges transient request memory against the bounded access
// resource. The caller owns public availability and request validation; this
// method only performs the resource admission step.
func (m *Manager) ReserveLocked(bytes uint64) error {
	if m == nil {
		return values.PublicUnavailable
	}
	m.ExpireLocked()
	if bytes > m.limits.Bytes || m.bytes > m.limits.Bytes-bytes {
		return values.PublicResourceExhausted
	}
	m.bytes += bytes
	return nil
}

// ReleaseBytesLocked releases a previous transient charge.
func (m *Manager) ReleaseBytesLocked(bytes uint64) {
	if m == nil {
		return
	}
	m.bytes -= bytes
}

// IssueLocked creates one public context, applying the parent lifetime and
// all context/per-caller/byte quotas in their existing order.
func (m *Manager) IssueLocked(caller values.CallerContext, kind string, parent *Context) (*Context, error) {
	if m == nil || m.closed {
		return nil, values.PublicUnavailable
	}
	m.ExpireLocked()
	if parent != nil && m.contexts[parent.Token.Value] != parent {
		return nil, values.PublicGap
	}
	count := uint64(0)
	for _, c := range m.contexts {
		if c.Caller.Subject == caller.Subject {
			count++
		}
	}
	if uint64(len(m.contexts)) >= m.limits.Contexts || count >= m.limits.PerCaller || m.limits.Bytes < contextBytes || m.bytes > m.limits.Bytes-contextBytes {
		return nil, values.PublicResourceExhausted
	}
	token, err := values.NewPublicToken(m.owner, kind)
	if err != nil {
		return nil, values.PublicUnavailable
	}
	expires := m.now().Add(m.limits.ContextTTL)
	if kind == "observation-preparation" {
		deadline := m.now().Add(m.limits.ObservationTTL)
		if deadline.Before(expires) {
			expires = deadline
		}
	}
	c := &Context{Token: token, Caller: caller, Expires: expires, bytes: contextBytes}
	if parent != nil {
		c.Parent = parent.Token.Value
		if parent.Expires.Before(c.Expires) {
			c.Expires = parent.Expires
		}
	}
	m.contexts[token.Value] = c
	m.bytes += c.bytes
	c.timer = time.AfterFunc(c.Expires.Sub(m.now()), func() {
		m.locker.Lock()
		m.ExpireLocked()
		m.locker.Unlock()
		m.DrainRetired(context.Background())
	})
	return c, nil
}

// LookupLocked returns the context currently indexed by its public token
// value.  It intentionally does not parse or authenticate a token.
func (m *Manager) LookupLocked(value string) *Context {
	if m == nil {
		return nil
	}
	return m.contexts[value]
}

// ResolveLocked applies the existing owner, token, caller, GAP and DENIED
// checks to an already-decoded public token.  It does not parse JSON.
func (m *Manager) ResolveLocked(caller values.CallerContext, token values.PublicToken) (*Context, error) {
	if m == nil || m.closed {
		return nil, values.PublicUnavailable
	}
	if !token.Valid() || token.Owner != m.owner {
		return nil, values.PublicInvalid
	}
	m.ExpireLocked()
	c := m.contexts[token.Value]
	if c == nil {
		return nil, values.PublicGap
	}
	if c.Token != token || c.Caller != caller {
		return nil, values.PublicDenied
	}
	return c, nil
}

// ExpireLocked removes all contexts whose absolute lease has elapsed.
func (m *Manager) ExpireLocked() {
	if m == nil {
		return
	}
	for id, c := range m.contexts {
		if !m.now().Before(c.Expires) {
			m.RemoveLocked(id)
		}
	}
}

// RemoveLocked retires one context and recursively retires its children.
// Expiry never waits for another owner. Its absolute lease independently
// releases the Core service preparation even while disconnected.
func (m *Manager) RemoveLocked(value string) {
	if m == nil {
		return
	}
	c := m.contexts[value]
	if c == nil {
		return
	}
	delete(m.contexts, value)
	if c.Release != nil {
		m.retired = append(m.retired, c.Release)
		c.Release = nil
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	m.bytes -= c.bytes
	if c.Token.Kind == "observation-preparation" && c.Stream != nil {
		m.releaseGroupsLocked(c.Stream)
		c.Stream.Groups = nil
		c.Stream.Gap = true
	}
	for id, child := range m.contexts {
		if child.Parent == c.Token.Value {
			m.RemoveLocked(id)
		}
	}
}

// DrainRetired runs bounded remote cleanup only after the owning fact lock is
// released. Removal, expiry and shutdown share the one context lifecycle.
func (m *Manager) DrainRetired(ctx context.Context) {
	if m == nil || ctx == nil {
		return
	}
	m.locker.Lock()
	retired := m.retired
	m.retired = nil
	m.locker.Unlock()
	for _, release := range retired {
		release(context.WithoutCancel(ctx))
	}
}

// GapLocked invalidates one observation stream and releases its queued group
// charge.  It is idempotent.
func (m *Manager) GapLocked(stream *Stream) {
	if m == nil || stream == nil || stream.Gap {
		return
	}
	stream.Gap = true
	m.releaseGroupsLocked(stream)
	stream.Groups = nil
}

// GapAllLocked invalidates every stream currently retained by the manager.
func (m *Manager) GapAllLocked() {
	if m == nil {
		return
	}
	for _, c := range m.contexts {
		if c.Stream != nil {
			m.GapLocked(c.Stream)
		}
	}
}

// CloseLocked retires the manager and all finite contexts.  It does not close
// a database or alter any namespace facts.
func (m *Manager) CloseLocked() {
	if m == nil {
		return
	}
	m.closed = true
	for id := range m.contexts {
		m.RemoveLocked(id)
	}
}

func (m *Manager) releaseGroupsLocked(stream *Stream) {
	charge := uint64(len(stream.Groups)) * groupBytes
	m.bytes -= charge
}
