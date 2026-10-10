// Package instance assembles and opens one local Core and owns its lifecycle.
// Its handles refer to real operations sharing one commit owner.
package instance

import (
	"context"
	"fmt"
	accessentry "ucupa/ucupa-core/internal/access/entry"
	"ucupa/ucupa-core/internal/handoff"
	federationcoordinator "ucupa/ucupa-core/internal/query/federation/coordinator"
	federationsource "ucupa/ucupa-core/internal/query/federation/source"
	querylocal "ucupa/ucupa-core/internal/query/local"
	"ucupa/ucupa-core/internal/realmcontact"
	"ucupa/ucupa-core/internal/transfer"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	accesspkg "ucupa/ucupa-core/internal/access"
	observationpkg "ucupa/ucupa-core/internal/access/observation"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	commitpkg "ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	facetpkg "ucupa/ucupa-core/internal/facet"
	mountpkg "ucupa/ucupa-core/internal/mount"
	querypkg "ucupa/ucupa-core/internal/query"
	connectedpkg "ucupa/ucupa-core/internal/query/connected"
)

// Startup verification has no originating request to derive a context from;
// the connections port bounds its own wait with the configured RPC deadline.
var startupContext = context.Background()

// Core owns one spatial fact set with a local root or an accepted external
// parent. Its bounded local mount profile has no Ref routing, network
// federation, or authorization-free existence surface. Every authorization
// flows through the one durable Realm contact of its original Store.
type Core struct {
	owner     *commitpkg.Owner
	authority values.FactAuthorityRef
	contact   contract.RealmConnections
	elements  *element.Operations
	mounts    *mountpkg.Operations
	queries   *querypkg.Operations
	connected *connectedpkg.Operations
	facets    *facetpkg.Operations
	sources   *federationsource.Operations
	access    *accessentry.Operations
	resources *accessregistry.Registry
	writer    *handoff.Operations
	transfers *transfer.Operations
	realm     *realmcontact.Operations
}

// New creates one in-memory Core from trusted initialization: the connections
// port must first prove it reaches exactly the named Realm before the new
// owner identity is bound to that contact in its initial state. No request
// input can supply or replace the contact.
func New(connections contract.RealmConnections, initial contract.RealmInitialization, config contract.Config) (*Core, error) {
	if config.Writer != nil {
		return nil, contract.ErrInvalid
	}
	if connections == nil || !initial.Valid() {
		return nil, contract.ErrInvalid
	}
	settings, err := querypkg.NormalizeConfig(config)
	if err != nil {
		return nil, err
	}
	run, err := newTokenScope()
	if err != nil {
		return nil, commitpkg.WrapStoreError("create read run", err)
	}
	scope, err := newTokenScope()
	if err != nil {
		return nil, fmt.Errorf("%w: establish Core token scope: %w", contract.ErrStore, err)
	}
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		return nil, fmt.Errorf("%w: establish Core identity: %w", contract.ErrStore, err)
	}
	if err := connections.Check(startupContext, initial.NextHop, initial.Realm); err != nil {
		return nil, fmt.Errorf("%w: Realm contact is not verifiable: %w", contract.ErrStore, err)
	}
	owner, err := commitpkg.New(authority, scope, initial)
	if err != nil {
		return nil, err
	}
	return newCore(connections, owner, run, settings), nil
}

func newCore(connections contract.RealmConnections, owner *commitpkg.Owner, run [32]byte, settings querypkg.Settings) *Core {
	authorizer := &contactAuthorizer{owner: owner, port: connections}
	admission := accesspkg.New(authorizer, owner, settings.Public)
	queries := querypkg.New(owner, authorizer, run, settings)
	registry := accessregistry.New(owner.AuthorityRef(), settings.Public, owner, queries)
	observations := observationpkg.New(owner, admission, registry)
	core := &Core{mounts: mountpkg.New(owner, authorizer, admission), facets: facetpkg.New(owner, authorizer, admission, registry), elements: element.New(owner, authorizer), authority: owner.AuthorityRef(), owner: owner, contact: connections, queries: queries, connected: connectedpkg.New(owner, authorizer, queries), resources: registry, realm: realmcontact.New(owner, connections)}
	local := querylocal.New(owner, admission, registry, queries)
	mutations := element.NewPublic(owner, admission, registry)
	core.sources = federationsource.New(owner, admission, registry, queries, observations)
	federation := federationcoordinator.New(owner, admission, registry, core.sources, observations)
	core.access = accessentry.New(owner, admission, registry, local, federation, mutations)
	core.writer = handoff.New(owner, admission, registry)
	core.transfers = transfer.New(owner, admission)
	owner.Lock()
	err := owner.SetPublicationSinkLocked(registry)
	owner.Unlock()
	if err != nil {
		panic("Core publication is already bound")
	}
	return core
}

// AuthorityRef identifies this logical Core to trusted local assembly. It is
// immutable, including after Close; it does not assert availability or authorize
// access, and its equality alone is not authentication of a remote service.
func (core *Core) AuthorityRef() values.FactAuthorityRef {
	if core == nil {
		return ""
	}
	return core.authority
}

// Elements returns the assembled local Element read/write entry, not a second Core.
// The returned handle shares this instance's lifetime and commit owner.
func (core *Core) Elements() *element.Operations {
	if core == nil {
		return nil
	}
	return core.elements
}

// Queries exposes querying and snapshot lifecycle, not internal read leases.
func (core *Core) Queries() contract.QueryAccess {
	if core == nil {
		return (*querypkg.Operations)(nil)
	}
	return core.queries
}

// Facets returns the actual Core-side relationship operations. It is not a
// Facet content service and shares this Core's commit and admission lifetime.
func (core *Core) Facets() *facetpkg.Operations {
	if core == nil {
		return nil
	}
	return core.facets
}

// Mounts returns the actual mount operations sharing this Core's commit and
// admission lifetime. The instance does not proxy individual mount methods.
func (core *Core) Mounts() *mountpkg.Operations {
	if core == nil {
		return nil
	}
	return core.mounts
}

// Connected returns the actual trusted-local cross-Core query operations.
// They share the local snapshot owner and this instance's Close lifecycle.
func (core *Core) Connected() *connectedpkg.Operations {
	if core == nil {
		return nil
	}
	return core.connected
}

// Federation returns the actual owner-local source operations. The handle
// shares the instance's facts, admission, snapshots and observation registry.
func (core *Core) Federation() *federationsource.Operations {
	if core == nil {
		return nil
	}
	return core.sources
}

// Access returns the actual bounded public JSON entry. Its port shares this
// instance's identity, admission, commit, read resources and Close lifecycle.
func (core *Core) Access() *accessentry.Operations {
	if core == nil {
		return nil
	}
	return core.access
}

// Writer returns actual control operations; it is not a second writer owner.
func (core *Core) Writer() *handoff.Operations {
	if core == nil {
		return nil
	}
	return core.writer
}

// Transfers returns actual source and parent control operations sharing this
// instance's admission and durable owner, not a separate transition service.
func (core *Core) Transfers() *transfer.Operations {
	if core == nil {
		return nil
	}
	return core.transfers
}

// RealmContact returns the actual owner-local Realm contact maintenance
// operations. They share this Core's sole commit owner and durable Store;
// the handle is not a second contact owner.
func (core *Core) RealmContact() *realmcontact.Operations {
	if core == nil {
		return nil
	}
	return core.realm
}

// CurrentRealmContact exposes the current durable contact for trusted local
// assembly wiring. False means no complete contact is currently bound.
func (core *Core) CurrentRealmContact() (contract.RealmAttachment, bool) {
	if core == nil || core.owner == nil {
		return contract.RealmAttachment{}, false
	}
	return core.owner.CurrentContact()
}
