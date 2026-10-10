// Package instance constructs and opens a local Facet and owns its lifecycle.
// Content and relationship operations are provided directly by Namespace.
package instance

import (
	"context"
	"errors"
	"fmt"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/namespace"
)

// Config fixes this Facet run's public access policy and writer startup.
// ExpectedRealm is the open-time assembly assertion: it must equal the Realm
// of the durable contact in the original Store and never overrides it.
type Config struct {
	PublicAccessPolicy *values.PublicAccessPolicy
	Writer             *values.WriterStartup
	ExpectedRealm      values.RealmID
}

// Instance is a handle, not a second owner. Copying the handle keeps the same
// Namespace, commit lock, durable state and finite access resources.
type Instance struct {
	Namespace *namespace.Owner
}

// Startup verification has no originating request to derive a context from;
// the contact check still bounds itself by the connection port's own deadline.
var startupContext = context.Background()

// New creates one in-memory Facet from trusted initialization: the connections
// port must first prove it reaches exactly the named Realm before the new
// owner identity is bound to that contact in its initial state.
func New(connections contract.RealmConnections, cores contract.CoreSource, initial contract.RealmInitialization, config Config) (*Instance, error) {
	if config.Writer != nil {
		return nil, contract.ErrInvalid
	}
	if connections == nil || !initial.Valid() {
		return nil, contract.ErrInvalid
	}
	if err := connections.Check(startupContext, initial.NextHop, initial.Realm); err != nil {
		return nil, fmt.Errorf("%w: Realm contact is not verifiable: %w", contract.ErrStore, err)
	}
	owner, err := namespace.New(connections, cores, initial, config.PublicAccessPolicy)
	if err != nil {
		return nil, err
	}
	return &Instance{Namespace: owner}, nil
}

// CreateDurable creates one owned Store whose initial document binds the
// generated Facet identity to the verified Realm contact.
func CreateDurable(connections contract.RealmConnections, cores contract.CoreSource, initial contract.RealmInitialization, path string, config Config) (*Instance, error) {
	if connections == nil || !initial.Valid() {
		return nil, contract.ErrInvalid
	}
	if config.ExpectedRealm != "" && config.ExpectedRealm != initial.Realm {
		return nil, contract.ErrInvalid
	}
	if err := connections.Check(startupContext, initial.NextHop, initial.Realm); err != nil {
		return nil, fmt.Errorf("%w: Realm contact is not verifiable: %w", contract.ErrStore, err)
	}
	owner, err := namespace.CreateDurable(connections, cores, initial, path, config.PublicAccessPolicy, config.Writer)
	if err != nil {
		return nil, err
	}
	return &Instance{Namespace: owner}, nil
}

// OpenDurable reopens one existing Store. The contact is recovered from the
// original Store only: an ExpectedRealm assertion must match the recovered
// contact, the connections port must verify it, and configuration can never
// supply or replace it.
func OpenDurable(connections contract.RealmConnections, cores contract.CoreSource, path string, config Config) (*Instance, error) {
	if connections == nil {
		return nil, contract.ErrInvalid
	}
	owner, err := namespace.OpenDurable(connections, cores, path, config.PublicAccessPolicy, config.Writer)
	if err != nil {
		return nil, err
	}
	contact, err := owner.RealmContact()
	if err != nil {
		closeErr := namespace.Close(owner)
		return nil, fmt.Errorf("%w: durable Realm contact is missing: %w", contract.ErrStore, errors.Join(err, closeErr))
	}
	if config.ExpectedRealm != "" && config.ExpectedRealm != contact.Realm {
		closeErr := namespace.Close(owner)
		return nil, fmt.Errorf("%w: durable Realm contact belongs to Realm %q, not %q: %w", contract.ErrStore, contact.Realm, config.ExpectedRealm, closeErr)
	}
	if err := connections.Check(startupContext, contact.NextHop, contact.Realm); err != nil {
		closeErr := namespace.Close(owner)
		return nil, fmt.Errorf("%w: Realm contact is not verifiable: %w", contract.ErrStore, errors.Join(err, closeErr))
	}
	return &Instance{Namespace: owner}, nil
}

func (i *Instance) AuthorityRef() values.FactAuthorityRef {
	if i == nil {
		return ""
	}
	return i.Namespace.AuthorityRef()
}

func (i *Instance) Close() error {
	if i == nil {
		return nil
	}
	return namespace.Close(i.Namespace)
}
