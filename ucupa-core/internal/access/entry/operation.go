// Package entry owns the bounded public JSON envelope and space-handle entry.
// It dispatches to the actual local, federated and Element operations. It does
// not own another fact set, registry, snapshot, admission counter or mutex.
package entry

import (
	"ucupa/shared/values"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/access/registry"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
	"ucupa/ucupa-core/internal/query/federation/coordinator"
	"ucupa/ucupa-core/internal/query/local"
)

type Operations struct {
	owner      *commit.Owner
	admission  *access.Admission
	access     *registry.Registry
	local      *local.Operations
	federation *coordinator.Operations
	mutations  *element.PublicOperations
}

func New(owner *commit.Owner, admission *access.Admission, resources *registry.Registry, local *local.Operations, federation *coordinator.Operations, mutations *element.PublicOperations) *Operations {
	return &Operations{owner: owner, admission: admission, access: resources, local: local, federation: federation, mutations: mutations}
}

// AuthorityRef binds this captured public port to the same stable Core owner.
// Equality is not remote authentication, availability or authorization.
func (core *Operations) AuthorityRef() values.FactAuthorityRef {
	if core == nil {
		return ""
	}
	return core.owner.AuthorityRef()
}

// CurrentRealmContact discloses this owner's current durable Realm contact in
// shared value shape for trusted local network assembly. It is not a
// permission check.
func (core *Operations) CurrentRealmContact() (values.RealmID, values.NextHop, error) {
	if core == nil {
		return "", "", values.PublicUnavailable
	}
	core.owner.RLock()
	defer core.owner.RUnlock()
	contact, err := core.owner.RealmContactLocked()
	if err != nil {
		return "", "", err
	}
	return contact.Realm, contact.NextHop, nil
}
