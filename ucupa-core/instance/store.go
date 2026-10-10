package instance

import (
	"errors"
	"fmt"
	"strings"
	querypkg "ucupa/ucupa-core/internal/query"
	connectedpkg "ucupa/ucupa-core/internal/query/connected"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
)

// CreateDurable creates one owned Store whose initial transaction binds the
// generated Core identity to the verified Realm contact. The connections port
// must prove it reaches exactly that Realm before the identity exists; the
// optional ExpectedRealm assertion must agree with the trusted initialization.
func CreateDurable(connections corecontract.RealmConnections, initial corecontract.RealmInitialization, path string, config corecontract.Config) (*Core, error) {
	if connections == nil || !initial.Valid() || strings.TrimSpace(path) == "" {
		return nil, corecontract.ErrInvalid
	}
	if config.ExpectedRealm != "" && config.ExpectedRealm != initial.Realm {
		return nil, corecontract.ErrInvalid
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
		return nil, commitpkg.WrapStoreError("create token scope", err)
	}
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		return nil, commitpkg.WrapStoreError("create identity", err)
	}
	if err := connections.Check(startupContext, initial.NextHop, initial.Realm); err != nil {
		return nil, fmt.Errorf("%w: Realm contact is not verifiable: %w", corecontract.ErrStore, err)
	}

	owner, err := commitpkg.CreateDurable(path, authority, scope, initial, config.Writer)
	if err != nil {
		return nil, err
	}
	return newCore(connections, owner, run, settings), nil
}

// OpenDurable reopens one existing Store. The contact is recovered from the
// original Store only: an ExpectedRealm assertion must match the recovered
// contact, the connections port must verify it, and configuration can never
// supply or replace it.
func OpenDurable(connections corecontract.RealmConnections, path string, config corecontract.Config) (*Core, error) {
	if connections == nil || strings.TrimSpace(path) == "" {
		return nil, corecontract.ErrInvalid
	}
	settings, err := querypkg.NormalizeConfig(config)
	if err != nil {
		return nil, err
	}
	run, err := newTokenScope()
	if err != nil {
		return nil, commitpkg.WrapStoreError("create read run", err)
	}

	owner, err := commitpkg.OpenDurable(path, config.Writer)
	if err != nil {
		return nil, err
	}
	contact, ok := owner.CurrentContact()
	if !ok {
		closeErr := owner.CloseStore()
		return nil, fmt.Errorf("%w: durable Realm contact is missing: %w", corecontract.ErrStore, closeErr)
	}
	if config.ExpectedRealm != "" && config.ExpectedRealm != contact.Realm {
		closeErr := owner.CloseStore()
		return nil, fmt.Errorf("%w: durable Realm contact belongs to Realm %q, not %q: %w", corecontract.ErrStore, contact.Realm, config.ExpectedRealm, closeErr)
	}
	if err := connections.Check(startupContext, contact.NextHop, contact.Realm); err != nil {
		closeErr := owner.CloseStore()
		return nil, fmt.Errorf("%w: Realm contact is not verifiable: %w", corecontract.ErrStore, errors.Join(err, closeErr))
	}
	return newCore(connections, owner, run, settings), nil
}

// Close retires this Core run, including in-memory read resources. It does not
// wait for query computation; active borrowers cannot disclose after retirement.
// Ordinary subsequent operations fail ErrStore; valid old read handles expire.
func (core *Core) Close() error {
	if core == nil || core.owner == nil {
		return nil
	}
	core.owner.Lock()
	if !core.owner.BeginCloseLocked() {
		core.owner.Unlock()
		return nil
	}
	if core.resources != nil {
		core.resources.CloseLocked()
	}
	connectedpkg.CloseLocked(core.connected)
	core.owner.Unlock()
	if core.resources != nil {
		core.resources.DrainRetired()
	}
	if core.queries != nil {
		core.queries.Close()
	}

	return core.owner.CloseStore()
}
