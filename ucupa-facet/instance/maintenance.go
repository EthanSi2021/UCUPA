package instance

import (
	"context"
	"errors"
	"fmt"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/namespace"
)

// Maintenance is the minimal Facet maintenance handle: it exposes only Realm
// contact maintenance and Close on the exclusive original Store. It has no
// public surface, Core source or half-initialized business instance behind it.
type Maintenance struct {
	namespace *namespace.Owner
}

// OpenRealmMaintenance reopens one existing Store exclusively for contact
// maintenance. The persisted contact is recovered from the original Store
// only: an ExpectedRealm assertion must match it, but the persisted address
// need not be verifiable — repair authorizes through an explicit same-Realm
// candidate. PublicAccessPolicy is not used because a maintenance owner has no
// public surface; Writer keeps the ordinary writer and installation rules.
func OpenRealmMaintenance(connections contract.RealmConnections, path string, config Config) (*Maintenance, error) {
	if connections == nil {
		return nil, contract.ErrInvalid
	}
	owner, err := namespace.OpenRealmMaintenance(connections, path, config.Writer)
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
	return &Maintenance{namespace: owner}, nil
}

func (m *Maintenance) InspectRealmContact(ctx context.Context, caller values.CallerContext, candidate values.NextHop) (contract.RealmAttachment, error) {
	if m == nil || m.namespace == nil {
		return contract.RealmAttachment{}, contract.ErrStore
	}
	return m.namespace.InspectRealmContact(ctx, caller, candidate)
}

func (m *Maintenance) UpdateRealmContact(ctx context.Context, caller values.CallerContext, request contract.RealmContactUpdate) (contract.RealmContactResult, error) {
	if m == nil || m.namespace == nil {
		return contract.RealmContactResult{}, contract.ErrStore
	}
	return m.namespace.UpdateRealmContact(ctx, caller, request)
}

func (m *Maintenance) RecoverRealmContact(ctx context.Context, caller values.CallerContext, operation contract.OperationID, candidate values.NextHop) (contract.RealmContactResult, error) {
	if m == nil || m.namespace == nil {
		return contract.RealmContactResult{}, contract.ErrStore
	}
	return m.namespace.RecoverRealmContact(ctx, caller, operation, candidate)
}

func (m *Maintenance) Close() error {
	if m == nil || m.namespace == nil {
		return nil
	}
	return namespace.Close(m.namespace)
}
