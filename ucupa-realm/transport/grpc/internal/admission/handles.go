package admission

import (
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
)

// handleKind distinguishes the three run-local reference roles. Handles are
// unguessable opaque references to kernel-held evidence, never serialized
// proof and never an offline permission token.
type Kind uint8

const (
	Caller Kind = iota + 1
	View
	Denial
)

// handleEntry binds one reference to this Realm run, the receiving owner, the
// original caller and — for views — the exact requirement set it covers.
type Entry struct {
	Kind         Kind
	Owner        values.FactAuthorityRef
	Caller       values.CallerContext
	Requirements []values.Requirement
	View         values.AuthorizationView
	Denial       values.AuthorizationDenialView
	Parent       [32]byte
	Expires      time.Time
}

// handleRegistry is the single bounded registry at the admission layer. It is
// run-local: a restart drops every reference and nothing is persisted.
type Registry struct {
	gateMu     sync.Mutex
	config     Config
	challenges map[[32]byte]challengeEntry
	stopped    bool
	active     chan struct{}
	mu         sync.Mutex
	entries    map[[32]byte]Entry
	now        func() time.Time
}

func New(config Config, now func() time.Time) *Registry {
	return &Registry{config: config, entries: make(map[[32]byte]Entry), challenges: make(map[[32]byte]challengeEntry), active: make(chan struct{}, config.MaxConcurrentRPCs), now: now}
}

func (r *Registry) expireLocked(now time.Time) {
	for key, entry := range r.entries {
		if !now.Before(entry.Expires) {
			delete(r.entries, key)
		}
	}
}

// issue stores one entry under a fresh unguessable token and enforces the
// total and per-caller capacity bounds. The caller keeps the token; only its
// SHA-256 is indexed.
func (r *Registry) Issue(entry Entry, maxTotal, maxPerCaller int) ([32]byte, error) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(now)
	if len(r.entries) >= maxTotal {
		return [32]byte{}, status.Error(codes.ResourceExhausted, "handle capacity")
	}
	count := 0
	for _, existing := range r.entries {
		if existing.Owner == entry.Owner && existing.Caller.Subject == entry.Caller.Subject {
			count++
		}
	}
	if count >= maxPerCaller {
		return [32]byte{}, status.Error(codes.ResourceExhausted, "handle capacity")
	}
	var token [32]byte
	for {
		_, _ = rand.Read(token[:])
		key := sha256.Sum256(token[:])
		if _, exists := r.entries[key]; !exists {
			r.entries[key] = entry
			return token, nil
		}
	}
}

// lookup returns the live entry bound to the presented reference. Expired,
// unknown and post-restart references are indistinguishable.
func (r *Registry) Lookup(token [32]byte) (Entry, error) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(now)
	entry, ok := r.entries[sha256.Sum256(token[:])]
	if !ok {
		return Entry{}, status.Error(codes.Unauthenticated, "handle unavailable")
	}
	return entry, nil
}

// release removes one reference. Releasing an already absent handle succeeds;
// a failed release never grants or preserves offline permission.
func (r *Registry) Release(token [32]byte) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(now)
	delete(r.entries, sha256.Sum256(token[:]))
}

// reset drops every reference at shutdown. Nothing issued by one run is
// valid for another; a fresh registry is a fresh run-local namespace.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.entries)
}

func (r *Registry) Size() int {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(now)
	return len(r.entries)
}
