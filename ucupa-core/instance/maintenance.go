package instance

import (
	"context"
	"fmt"
	"strings"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	commitpkg "ucupa/ucupa-core/internal/commit"
	querypkg "ucupa/ucupa-core/internal/query"
	"ucupa/ucupa-core/internal/realmcontact"
)

// RealmMaintenance is the minimal Core maintenance handle: it exposes only
// Realm contact maintenance and Close on the exclusive original Store. No
// query, element or public surface is assembled behind it.
type RealmMaintenance struct {
	owner *commitpkg.Owner
	realm *realmcontact.Operations
}

// OpenRealmMaintenance reopens one existing Store exclusively for contact
// maintenance. The persisted contact is recovered from the original Store
// only: an ExpectedRealm assertion must match it, but the persisted address
// need not be verifiable — repair authorizes through an explicit same-Realm
// candidate. Writer keeps the ordinary writer and installation rules; the
// remaining config fields are validated but not used because a maintenance
// owner runs no queries.
func OpenRealmMaintenance(connections corecontract.RealmConnections, path string, config corecontract.Config) (*RealmMaintenance, error) {
	if connections == nil || strings.TrimSpace(path) == "" {
		return nil, corecontract.ErrInvalid
	}
	if _, err := querypkg.NormalizeConfig(config); err != nil {
		return nil, err
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
	return &RealmMaintenance{owner: owner, realm: realmcontact.New(owner, connections)}, nil
}

// Inspect discloses the current contact only after the independent read
// permission; candidate may repair an unavailable current address. The caller
// is an established Realm session authorized online by the serving port.
func (m *RealmMaintenance) Inspect(ctx context.Context, caller values.CallerContext, candidate values.NextHop) (corecontract.RealmAttachment, error) {
	if m == nil || m.realm == nil {
		return corecontract.RealmAttachment{}, corecontract.ErrStore
	}
	return m.realm.Inspect(ctx, caller, candidate)
}

// Update consumes one authorized address-maintenance decision at the original
// owner transaction.
func (m *RealmMaintenance) Update(ctx context.Context, caller values.CallerContext, request corecontract.RealmContactUpdate) (corecontract.RealmContactResult, error) {
	if m == nil || m.realm == nil {
		return corecontract.RealmContactResult{}, corecontract.ErrStore
	}
	return m.realm.Update(ctx, caller, request)
}

// Recover reads only a retained original decision; it never re-executes.
func (m *RealmMaintenance) Recover(ctx context.Context, caller values.CallerContext, operation corecontract.OperationID, candidate values.NextHop) (corecontract.RealmContactResult, error) {
	if m == nil || m.realm == nil {
		return corecontract.RealmContactResult{}, corecontract.ErrStore
	}
	return m.realm.Recover(ctx, caller, operation, candidate)
}

// Close releases the exclusive original Store.
func (m *RealmMaintenance) Close() error {
	if m == nil || m.owner == nil {
		return nil
	}
	return m.owner.CloseStore()
}
