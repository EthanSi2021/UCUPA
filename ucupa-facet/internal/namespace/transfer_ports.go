package namespace

import (
	"bytes"
	"context"
	"crypto/sha256"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// The namespace replacement peer ports. Each method validates its caller and
// authorizes through the Realm, then advances or reads durable owner state
// under the single owner lock; the lock is never held across peer reads.

// contactLocked reads the current core address of a valid acceptance. It is
// the inline variant for transfer steps whose element may be fenced or have a
// pending install, where captureLocked must refuse.
func (f *Owner) contactLocked(k keyData) (values.NextHop, error) {
	a, ok := f.state.Acceptances[k.id()]
	c, connected := f.state.Contacts[k.id()]
	if !ok || !connected || !c.Address.Valid() || c.Serial == 0 || c.Serial > f.state.Cut || a.At == 0 || a.At > f.state.Cut || a.Key != k {
		return "", contract.ErrUnavailable
	}
	return c.Address, nil
}

func (f *Owner) fencedResultLocked(k keyData, op byteString) contract.NamespaceFenced {
	fenced, ok := f.state.Fenced[string(k.Element)]
	if !ok || fenced.Operation != op {
		return contract.NamespaceFenced{}
	}
	key := fenced.SourceKey.value()
	return contract.NamespaceFenced{Key: key, Manifest: manifestValue(fenced, key)}
}

func (f *Owner) CheckNamespaceExport(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID) (contract.NamespaceSourceState, error) {
	if ctx == nil || !f.validContext(key) || !validOperation(op) {
		return contract.NamespaceSourceState{}, contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceExport)
	if err != nil {
		return contract.NamespaceSourceState{}, err
	}
	defer permit.release(ctx)
	k := packKey(key)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.available(); err != nil {
		return contract.NamespaceSourceState{}, err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.NamespaceSourceState{}, contract.ErrUnavailable
	}
	state := contract.NamespaceSourceState{Key: key}
	if address, err := f.contactLocked(k); err == nil {
		state.Found = true
		_ = address
	}
	if fenced, ok := f.state.Fenced[string(k.Element)]; ok && fenced.Operation == byteString(op) {
		state.Fenced = true
		state.Settled = fenced.Settled
		manifest := manifestValue(fenced, fenced.SourceKey.value())
		state.Manifest = &manifest
	}
	if _, refused := f.state.Refused[string(op)]; refused {
		state.Refused = true
	}
	return state, nil
}

func (f *Owner) FenceNamespace(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, target values.FactAuthorityRef) (contract.NamespaceFenced, error) {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !target.Valid() || target == key.Facet {
		return contract.NamespaceFenced{}, contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceExport)
	if err != nil {
		return contract.NamespaceFenced{}, err
	}
	defer permit.release(ctx)
	c := f.transferBase(caller.Subject, key, op, "fence")
	c.Target = target
	f.mu.Lock()
	if err := f.available(); err != nil {
		f.mu.Unlock()
		return contract.NamespaceFenced{}, err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		f.mu.Unlock()
		return contract.NamespaceFenced{}, contract.ErrUnavailable
	}
	if _, err, found := f.lookupLocked(c); found {
		if err != nil {
			f.mu.Unlock()
			return contract.NamespaceFenced{}, err
		}
		result := f.fencedResultLocked(c.Key, byteString(op))
		f.mu.Unlock()
		return result, nil
	}
	address, err := f.contactLocked(c.Key)
	if err != nil {
		f.mu.Unlock()
		return contract.NamespaceFenced{}, err
	}
	f.mu.Unlock()
	plan, err := f.readPlan(ctx, address, key, op)
	if err != nil {
		return contract.NamespaceFenced{}, err
	}
	if permit.recheck(ctx, false) != nil {
		return contract.NamespaceFenced{}, contract.ErrUnavailable
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.NamespaceFenced{}, contract.ErrUnavailable
	}
	if _, err = f.commitLocked(c, proof{}, departureData{}, plan); err != nil {
		return contract.NamespaceFenced{}, err
	}
	return f.fencedResultLocked(c.Key, byteString(op)), nil
}

func (f *Owner) RefuseNamespaceFence(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, target values.FactAuthorityRef) error {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !target.Valid() || target == key.Facet {
		return contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceExport)
	if err != nil {
		return err
	}
	defer permit.release(ctx)
	c := f.transferBase(caller.Subject, key, op, "refuse")
	c.Target = target
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.ErrUnavailable
	}
	_, err = f.commitLocked(c, proof{}, departureData{}, transferPlan{})
	return err
}

func (f *Owner) SettleNamespace(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, target values.FactAuthorityRef) error {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !target.Valid() || target == key.Facet {
		return contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceExport)
	if err != nil {
		return err
	}
	defer permit.release(ctx)
	c := f.transferBase(caller.Subject, key, op, "settle")
	c.Target = target
	f.mu.Lock()
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		f.mu.Unlock()
		return contract.ErrUnavailable
	}
	if _, err, found := f.lookupLocked(c); found {
		f.mu.Unlock()
		return err
	}
	address, err := f.contactLocked(c.Key)
	if err != nil {
		f.mu.Unlock()
		return err
	}
	f.mu.Unlock()
	plan, err := f.readPlan(ctx, address, key, op)
	if err != nil {
		return err
	}
	if permit.recheck(ctx, false) != nil {
		return contract.ErrUnavailable
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.ErrUnavailable
	}
	_, err = f.commitLocked(c, proof{}, departureData{}, plan)
	return err
}

func (f *Owner) ReadNamespaceSource(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, typ values.FacetTypeRef) (contract.NamespaceSourceEntry, error) {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !typ.Valid() {
		return contract.NamespaceSourceEntry{}, contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceExport)
	if err != nil {
		return contract.NamespaceSourceEntry{}, err
	}
	defer permit.release(ctx)
	k := packKey(key)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.available(); err != nil {
		return contract.NamespaceSourceEntry{}, err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.NamespaceSourceEntry{}, contract.ErrUnavailable
	}
	fenced, ok := f.state.Fenced[string(k.Element)]
	if !ok || fenced.Operation != byteString(op) {
		return contract.NamespaceSourceEntry{}, contract.ErrUnavailable
	}
	for _, e := range fenced.Manifest {
		if !e.Live || !bytes.Equal(e.Type, typ.Bytes()) {
			continue
		}
		r, found := f.state.Records[recordID(k.Element, e.Type)]
		if !found || uint64(len(r.Payload)) != e.Length || sha256.Sum256(r.Payload) != e.Digest {
			return contract.NamespaceSourceEntry{}, contract.ErrStore
		}
		return contract.NamespaceSourceEntry{Type: typ, Payload: append([]byte(nil), r.Payload...), Revision: e.Revision.value()}, nil
	}
	return contract.NamespaceSourceEntry{}, contract.ErrKeyNotFound
}
