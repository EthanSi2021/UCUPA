// Package collect owns one finite cross-source Federation read.
//
// It retains source receipts and an immutable query recipe, never Core facts.
// Public handles, JSON representation, authorization continuity, quota and
// final disclosure validation remain with the Core access owner.
package collect

import (
	"context"
	"sync"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
)

// RecipeBytes is the fixed access-owner reservation for one retained recipe.
// Transient result rows are separately bounded and never retained by Read.
const RecipeBytes uint64 = 8 << 20

// Address is the typed form of a public spatial address after JSON validation.
type Address struct {
	Origin values.ElementID
	Up     uint64
	Down   []values.ElementID
}

// Query is the typed, caller-requested spatial recipe. Resolve derives Scope
// and coordinate steps only from authenticated source projections.
type Query struct {
	Start    Address
	Basis    Address
	UpDepth  int64
	Sphere   corecontract.QuerySphere
	Selector corecontract.QuerySelector
}

// Request is a closed construction input. Read/List use Address; Query uses
// Query. It contains no public token, JSON value or owner callback.
type Request struct {
	Operation string
	Address   Address
	Query     *Query
}

// Config supplies the already-opened local source and the authenticated port
// used to follow only mount frontiers projected by an admitted source.
type Config struct {
	Caller      values.CallerContext
	Source      values.FactAuthorityRef
	PartRequest federationcontract.FederationReadRequest
	Local       federationcontract.FederationReadConnection
	Connections federationcontract.FederationConnections
	SourceLimit int
	Request     Request
}

// Page is detached collection output. Paths are relative to the public
// boundary and are encoded by the outer public adapter.
type Page struct {
	Record        *federationcontract.FederationRecord
	Records       []federationcontract.FederationRecord
	Matches       []federationcontract.FederationMatch
	Partial       bool
	Gap           corecontract.QueryGap
	Scope         values.ElementID
	ActualUpDepth uint64
	Last          values.ElementID
	Closure       [32]byte
	SourceGraph   SourceGraph
	ReadMode      corecontract.QueryReadMode
}

// ExactResult is one detached record read through an existing sealed Read.
type ExactResult struct {
	Record      federationcontract.FederationRecord
	Closure     [32]byte
	SourceGraph SourceGraph
	ReadMode    corecontract.QueryReadMode
}

// ObservationPage contains private invalidation evidence and the next local
// numeric position. Public cursor identity remains outside this package.
type ObservationPage struct {
	Groups   []federationcontract.FederationChangeGroup
	Position uint64
	Base     uint64
}

// SourceGraph is the finite disclosure projection, not the private graph.
type SourceGraph struct {
	Purpose   [32]byte
	Operation string
	Sources   []SourceEvidence
}

type SourceEvidence struct {
	Owner              values.FactAuthorityRef
	Boundary           values.ElementID
	Subject            values.PrincipalRef
	CutStamp           [32]byte
	AuthorizationStamp [32]byte
	ReadMode           corecontract.QueryReadMode
	Parent             *ParentEvidence
	Query              *QueryEvidence
}

type ParentEvidence struct {
	Owner      values.FactAuthorityRef
	Anchor     values.ElementID
	Delegation [32]byte
	Acceptance [32]byte
}

type QueryEvidence struct {
	Scope    values.ElementID
	Coverage [32]byte
	Parents  []federationcontract.FederationParentEvidence
}

// ExactBasis is opaque outside collect. It can only be replayed by ReadAt on
// the Read which issued it.
type ExactBasis struct {
	read    *Read
	part    federationcontract.FederationReadConnection
	element values.ElementID
}

// Lease keeps one completed collector operation serialized until the outer
// access owner has performed its final context/authorization validation. It
// exposes neither the mutex nor any collector state and can only be released.
type Lease struct {
	once sync.Once
	read *Read
}

func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() { l.read.mu.Unlock() })
}

// Read owns one sealed source graph, recipe and observation stream. Its mutex
// serializes reuse with retirement; no Core owner lock may be held on entry.
type Read struct {
	mu          sync.Mutex
	closed      bool
	sources     *sources
	prefix      map[string][]values.ElementID
	required    map[string]bool
	plans       []projection
	target      location
	boundary    values.ElementID
	scope       values.ElementID
	actual      uint64
	gap         corecontract.QueryGap
	operation   string
	used        uint64
	observation *observation
	delivery    *observationDelivery
}

type projection struct {
	part     federationcontract.FederationReadConnection
	request  federationcontract.FederationProjectionRequest
	coverage [32]byte
	parents  []federationcontract.FederationParentEvidence
}

// Retire closes the recipe and releases every source outside the Core owner
// lock. It is idempotent and never reopens or refreshes a source.
func (r *Read) Retire(ctx context.Context) error {
	if r == nil || ctx == nil {
		return values.PublicInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.sources.release(ctx)
}

func (r *Read) charge(cost uint64) error {
	if cost > RecipeBytes || r.used > RecipeBytes-cost {
		return values.PublicResourceExhausted
	}
	r.used += cost
	return nil
}
