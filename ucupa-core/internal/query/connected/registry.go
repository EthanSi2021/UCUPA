package connected

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"sync"
	"time"
	querypkg "ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/query/exchange"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	coretoken "ucupa/ucupa-core/internal/token"
)

type connectedSourcePlan struct {
	authority values.FactAuthorityRef
	hop       values.NextHop
	actor     values.PrincipalRef
	grant     exchange.Grant
	intent    exchange.Intent
	cut       contract.CoreReadCut
	run       [32]byte
	snapshot  *contract.CoreReadSnapshot
	contact   mountfact.Contact
}

type connectedContext struct {
	cut           contract.ConnectedCut
	mode          contract.QueryReadMode
	intent        exchange.Intent
	scope         values.ElementID
	depth         uint64
	scopePath     contract.Path
	originToMount contract.Path
	sources       []connectedSourcePlan
	key           contract.MountKey
	excluded      bool
	excludedAt    values.ElementID
	requiredB     bool
	terminal      bool
	retired       bool
	borrows       uint32
	bytes         uint64
	expires       time.Time
}

type connectedLeaseEntry struct {
	borrow *querypkg.Lease
	actor  values.PrincipalRef
	source values.FactAuthorityRef
	grant  exchange.Grant
}

type connectedRegistry struct {
	mu       sync.Mutex
	closed   bool
	serial   uint64
	bytes    uint64
	active   uint32
	contexts map[uint64]*connectedContext
	leases   map[uint64]connectedLeaseEntry
}

func (core *Operations) connectedRegistry() (*connectedRegistry, error) {
	core.owner.Lock()
	defer core.owner.Unlock()
	if core.owner.ClosedLocked() || !core.owner.AvailableLocked() {
		return nil, contract.ErrStore
	}
	if core.registry == nil {
		core.registry = &connectedRegistry{contexts: make(map[uint64]*connectedContext), leases: make(map[uint64]connectedLeaseEntry)}
	}
	return core.registry, nil
}

// Close performs local invalidation only. Remote retained pins already have
// owner-local deadlines; closing never invokes another Core under this lock.
func CloseLocked(core *Operations) {
	if core.registry == nil {
		return
	}
	r := core.registry
	r.mu.Lock()
	r.closed = true
	for id, context := range r.contexts {
		context.retired = true
		if context.borrows == 0 {
			delete(r.contexts, id)
			r.bytes -= context.bytes
		}
	}
	for id, lease := range r.leases {
		lease.borrow.Finish()
		delete(r.leases, id)
	}
	r.mu.Unlock()
}

func connectedCallerBinding(callers contract.LocalCallers) ([32]byte, bool) {
	if len(callers) == 0 || len(callers) > 2 {
		return [32]byte{}, false
	}
	keys := make([]string, 0, len(callers))
	for ref, actor := range callers {
		if !ref.Valid() || !actor.Valid() {
			return [32]byte{}, false
		}
		keys = append(keys, string(ref))
	}
	sort.Strings(keys)
	h := sha256.New()
	var size [8]byte
	for _, key := range keys {
		actor := callers[values.FactAuthorityRef(key)]
		// PrincipalRef is a pair of opaque strings. Its canonical formatting is
		// not a credential; length framing prevents concatenation collisions.
		for _, value := range []string{key, string(actor.Realm), string(actor.Principal)} {
			binary.BigEndian.PutUint64(size[:], uint64(len(value)))
			_, _ = h.Write(size[:])
			_, _ = h.Write([]byte(value))
		}
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, true
}

func (core *Operations) checkConnectedBinding(callers contract.LocalCallers, cut contract.ConnectedCut) error {
	binding, ok := connectedCallerBinding(callers)
	if !ok || !coretoken.ValidConnectedCut(cut) || coretoken.ConnectedOwner(cut) != core.authority || coretoken.ConnectedCallers(cut) != binding {
		return contract.ErrInvalid
	}
	if coretoken.ConnectedRun(cut) != core.queries.Run() {
		return contract.ErrReadCutExpired
	}
	return nil
}

func connectedContextBytes(context *connectedContext) uint64 {
	n := uint64(len(context.intent.Path.Down) + len(context.intent.Query.StartPath.Down) + len(context.intent.Query.BasisPath.Down) + len(context.scopePath.Down) + len(context.originToMount.Down))
	strings := uint64(0)
	for _, source := range context.sources {
		n += uint64(len(source.grant.Prefix) + len(source.grant.Adjacency) + len(source.intent.Path.Down))
		strings += uint64(len(source.actor.Realm) + len(source.actor.Principal) + len(source.hop))
	}
	return 4096 + n*128 + uint64(len(context.sources))*4096 + strings
}

func (core *Operations) publishConnectedContext(callers contract.LocalCallers, context *connectedContext) error {
	r, err := core.connectedRegistry()
	if err != nil {
		return err
	}
	binding, ok := connectedCallerBinding(callers)
	if !ok {
		return contract.ErrInvalid
	}
	context.bytes = connectedContextBytes(context)
	settings := core.queries.Settings()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return contract.ErrReadCutExpired
	}
	for id, old := range r.contexts {
		if old.borrows == 0 && (old.retired || (!old.expires.IsZero() && !core.queries.Now().Before(old.expires))) {
			delete(r.contexts, id)
			r.bytes -= old.bytes
		}
	}
	for uint32(len(r.contexts)) >= settings.Handles || context.bytes > settings.Bytes || r.bytes > settings.Bytes-context.bytes {
		var candidate uint64
		for id, old := range r.contexts {
			if old.mode == contract.QueryCurrentVerified && old.borrows == 0 && (candidate == 0 || id < candidate) {
				candidate = id
			}
		}
		if candidate == 0 {
			return contract.ErrSnapshotCapacity
		}
		r.bytes -= r.contexts[candidate].bytes
		r.contexts[candidate].retired = true
		delete(r.contexts, candidate)
	}
	if r.serial == math.MaxUint64 || r.active >= settings.Concurrent {
		return contract.ErrSnapshotCapacity
	}
	r.serial++
	context.cut = coretoken.ConnectedCutFromParts(core.authority, core.queries.Run(), r.serial, binding)
	if context.mode == contract.QueryRetained {
		context.expires = core.queries.Now().Add(settings.Retention)
	}
	context.borrows = 1
	r.active++
	r.bytes += context.bytes
	r.contexts[coretoken.ConnectedID(context.cut)] = context
	return nil
}

func (core *Operations) borrowConnectedContext(callers contract.LocalCallers, cut contract.ConnectedCut) (*connectedContext, error) {
	if err := core.checkConnectedBinding(callers, cut); err != nil {
		return nil, err
	}
	r, err := core.connectedRegistry()
	if err != nil {
		core.owner.RLock()
		closed := core.owner.ClosedLocked()
		core.owner.RUnlock()
		if err == contract.ErrStore && closed {
			return nil, contract.ErrReadCutExpired
		}
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.contexts[coretoken.ConnectedID(cut)]
	if entry == nil || entry.cut != cut || entry.retired || r.closed {
		return nil, contract.ErrReadCutExpired
	}
	if !entry.expires.IsZero() && !core.queries.Now().Before(entry.expires) {
		entry.retired = true
		return nil, contract.ErrReadCutExpired
	}
	if r.active >= core.queries.Settings().Concurrent {
		return nil, contract.ErrSnapshotCapacity
	}
	entry.borrows++
	r.active++
	return entry, nil
}

func (core *Operations) finishConnectedContext(context *connectedContext) {
	r := core.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	context.borrows--
	r.active--
	if context.retired && context.borrows == 0 {
		delete(r.contexts, coretoken.ConnectedID(context.cut))
		r.bytes -= context.bytes
	}
}

func (core *Operations) connectedTerminal(context *connectedContext, mark bool) (bool, error) {
	r := core.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || context.retired || r.contexts[coretoken.ConnectedID(context.cut)] != context {
		return false, contract.ErrReadCutExpired
	}
	if mark {
		context.terminal = true
	}
	return context.terminal, nil
}
