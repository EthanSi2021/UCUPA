package namespace

import (
	"bytes"
	"context"
	"crypto/sha256"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

func (f *Owner) CheckNamespaceImport(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, manifest contract.NamespaceManifest, address values.NextHop) (contract.NamespaceImportState, error) {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !address.Valid() {
		return contract.NamespaceImportState{}, contract.ErrInvalid
	}
	entries, ok := unpackManifest(manifest)
	if !ok || manifest.Element != key.Element || manifest.Source == key.Facet || manifest.Source == key.Core {
		return contract.NamespaceImportState{}, contract.ErrInvalid
	}
	k := packKey(key)
	if !validManifestData(entries, k) {
		return contract.NamespaceImportState{}, contract.ErrInvalid
	}
	digest := manifestDigest(entries)
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceImport)
	if err != nil {
		return contract.NamespaceImportState{}, err
	}
	defer permit.release(ctx)
	f.mu.Lock()
	if err := f.available(); err != nil {
		f.mu.Unlock()
		return contract.NamespaceImportState{}, err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		f.mu.Unlock()
		return contract.NamespaceImportState{}, contract.ErrUnavailable
	}
	if p, ok := f.state.Pending[k.id()]; ok && p.Operation == byteString(op) {
		state := contract.NamespaceImportState{Accepted: true, Key: key, Digest: p.Digest, FinalCut: p.FinalCut}
		f.mu.Unlock()
		return state, nil
	}
	if st, ok := f.staged[stagedID(k)]; ok && st.operation == byteString(op) {
		if st.finalCut != manifest.FinalCut || st.digest != digest {
			f.mu.Unlock()
			return contract.NamespaceImportState{}, contract.ErrOperationConflict
		}
		state := contract.NamespaceImportState{Key: key, Digest: digest, FinalCut: manifest.FinalCut, Live: liveEntries(st.manifest)}
		f.mu.Unlock()
		return state, nil
	}
	if len(f.staged) >= maxStagedImports {
		f.mu.Unlock()
		return contract.NamespaceImportState{}, contract.ErrCapacity
	}
	f.mu.Unlock()
	plan, err := f.readPlan(ctx, address, key, op)
	if err != nil {
		return contract.NamespaceImportState{}, err
	}
	if permit.recheck(ctx, false) != nil {
		return contract.NamespaceImportState{}, contract.ErrUnavailable
	}
	if !plan.present() || plan.Authority != key.Core || plan.Element != key.Element || plan.Operation != byteString(op) ||
		plan.Source != manifest.Source || plan.Target != f.authority || plan.Proposal != k.Proposal ||
		plan.FinalCut != manifest.FinalCut || plan.ManifestDigest != digest ||
		!plan.Permitted || !plan.Fenced || plan.Accepted || plan.Committed || plan.Settled || plan.Cancelled {
		return contract.NamespaceImportState{}, contract.ErrUnavailable
	}
	live := 0
	for _, e := range entries {
		if e.Live {
			live++
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.available(); err != nil {
		return contract.NamespaceImportState{}, err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.NamespaceImportState{}, contract.ErrUnavailable
	}
	if p, ok := f.state.Pending[k.id()]; ok && p.Operation == byteString(op) {
		return contract.NamespaceImportState{Accepted: true, Key: key, Digest: p.Digest, FinalCut: p.FinalCut}, nil
	}
	if f.staged == nil {
		f.staged = map[string]*stagedImport{}
	}
	f.staged[stagedID(k)] = &stagedImport{
		key: k, operation: byteString(op), source: manifest.Source,
		finalCut: manifest.FinalCut, digest: digest, manifest: entries, live: live,
		staged: map[int][]byte{},
	}
	return contract.NamespaceImportState{Key: key, Digest: digest, FinalCut: manifest.FinalCut, Live: liveEntries(entries)}, nil
}

func (f *Owner) StageNamespaceEntry(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, typ values.FacetTypeRef, payload []byte) error {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !typ.Valid() {
		return contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceImport)
	if err != nil {
		return err
	}
	defer permit.release(ctx)
	k := packKey(key)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.available(); err != nil {
		return err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.ErrUnavailable
	}
	st, ok := f.staged[stagedID(k)]
	if !ok || st.operation != byteString(op) {
		return contract.ErrInvalid
	}
	index := -1
	for i, e := range st.manifest {
		if e.Live && bytes.Equal(e.Type, typ.Bytes()) {
			index = i
			break
		}
	}
	if index < 0 {
		return contract.ErrInvalid
	}
	e := st.manifest[index]
	if uint64(len(payload)) != e.Length || sha256.Sum256(payload) != e.Digest {
		return contract.ErrInvalid
	}
	if existing, ok := st.staged[index]; ok {
		if bytes.Equal(existing, payload) {
			return nil
		}
		return contract.ErrOperationConflict
	}
	if st.bytes+uint64(len(payload)) > maxStagedBytes {
		return contract.ErrCapacity
	}
	st.staged[index] = append([]byte(nil), payload...)
	st.bytes += uint64(len(payload))
	return nil
}

// AcceptNamespace durably accepts a complete replacement namespace and, in
// the same transaction, establishes the target relation to the controlling
// Core. The supplied address is the Core contact recorded with the acceptance.
func (f *Owner) AcceptNamespace(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, address values.NextHop) (contract.NamespaceAccepted, error) {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !address.Valid() {
		return contract.NamespaceAccepted{}, contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceImport)
	if err != nil {
		return contract.NamespaceAccepted{}, err
	}
	defer permit.release(ctx)
	k := packKey(key)
	c := f.transferBase(caller.Subject, key, op, "accept-namespace")
	c.Address = address
	f.mu.Lock()
	if err := f.available(); err != nil {
		f.mu.Unlock()
		return contract.NamespaceAccepted{}, err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		f.mu.Unlock()
		return contract.NamespaceAccepted{}, contract.ErrUnavailable
	}
	if _, err, found := f.lookupLocked(c); found {
		delete(f.staged, stagedID(k))
		f.mu.Unlock()
		if err != nil {
			return contract.NamespaceAccepted{}, err
		}
		return contract.NamespaceAccepted{Key: key}, nil
	}
	st, ok := f.staged[stagedID(k)]
	if !ok || st.operation != byteString(op) || !st.complete() {
		f.mu.Unlock()
		return contract.NamespaceAccepted{}, contract.ErrInvalid
	}
	c.Source = st.source
	c.FinalCut = st.finalCut
	c.Manifest = st.manifest
	c.Transfer = make([]transferEntry, 0, st.live)
	for i, e := range st.manifest {
		if !e.Live {
			continue
		}
		c.Transfer = append(c.Transfer, transferEntry{Type: e.Type, Payload: st.staged[i]})
	}
	f.mu.Unlock()
	plan, err := f.readPlan(ctx, address, key, op)
	if err != nil {
		return contract.NamespaceAccepted{}, err
	}
	if permit.recheck(ctx, false) != nil {
		return contract.NamespaceAccepted{}, contract.ErrUnavailable
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.NamespaceAccepted{}, contract.ErrUnavailable
	}
	result, err := f.commitLocked(c, proof{}, departureData{}, plan)
	if err == nil || result.Status == contract.Rejected {
		delete(f.staged, stagedID(k))
	}
	if err != nil {
		return contract.NamespaceAccepted{}, err
	}
	return contract.NamespaceAccepted{Key: key}, nil
}

func (f *Owner) InstallNamespace(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID, source values.FactAuthorityRef, finalCut uint64) error {
	if ctx == nil || !f.validContext(key) || !validOperation(op) || !source.Valid() || source == key.Facet || finalCut == 0 {
		return contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceImport)
	if err != nil {
		return err
	}
	defer permit.release(ctx)
	k := packKey(key)
	c := f.transferBase(caller.Subject, key, op, "install")
	c.Source = source
	c.FinalCut = finalCut
	f.mu.Lock()
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		f.mu.Unlock()
		return contract.ErrUnavailable
	}
	if _, err, found := f.lookupLocked(c); found {
		f.mu.Unlock()
		return err
	}
	p, ok := f.state.Pending[k.id()]
	if !ok || p.Operation != c.Operation || p.Source != c.Source || p.FinalCut != c.FinalCut {
		f.mu.Unlock()
		return contract.ErrInvalid
	}
	address, err := f.contactLocked(k)
	if err != nil {
		f.mu.Unlock()
		return err
	}
	f.mu.Unlock()
	e, err := f.cores.ReadAttachment(ctx, address, key)
	if err != nil || ctx.Err() != nil || !packProof(e).valid(k, false) {
		return contract.ErrUnavailable
	}
	if permit.recheck(ctx, false) != nil {
		return contract.ErrUnavailable
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.ErrUnavailable
	}
	_, err = f.commitLocked(c, packProof(e), departureData{}, transferPlan{})
	return err
}

func (f *Owner) ReadNamespaceImport(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, op contract.OperationID) (contract.NamespaceImportState, error) {
	if ctx == nil || !f.validContext(key) || !validOperation(op) {
		return contract.NamespaceImportState{}, contract.ErrInvalid
	}
	permit, err := f.authorizeNamespacePeer(ctx, caller, key, contract.PermissionNamespaceImport)
	if err != nil {
		return contract.NamespaceImportState{}, err
	}
	defer permit.release(ctx)
	k := packKey(key)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.available(); err != nil {
		return contract.NamespaceImportState{}, err
	}
	if f.namespacePeerGuardLocked(ctx, permit) != nil {
		return contract.NamespaceImportState{}, contract.ErrUnavailable
	}
	if p, ok := f.state.Pending[k.id()]; ok && p.Operation == byteString(op) {
		return contract.NamespaceImportState{Accepted: true, Key: key, Digest: p.Digest, FinalCut: p.FinalCut}, nil
	}
	if index, ok := f.operations[stepID(op, "accept-namespace")]; ok && index < len(f.events) {
		acceptance := f.events[index]
		if acceptance.Input.Kind == "accept-namespace" && acceptance.Outcome.Status == contract.Committed {
			digest := manifestDigest(acceptance.Input.Manifest)
			if installed, ok := f.operations[stepID(op, "install")]; ok && installed < len(f.events) {
				installation := f.events[installed]
				if installation.Input.Kind == "install" && installation.Outcome.Status == contract.Committed {
					return contract.NamespaceImportState{Installed: true, Key: key, Digest: digest, FinalCut: acceptance.Input.FinalCut}, nil
				}
			}
			return contract.NamespaceImportState{Key: key, Digest: digest, FinalCut: acceptance.Input.FinalCut}, nil
		}
	}
	return contract.NamespaceImportState{Key: key}, nil
}
