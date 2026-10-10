package namespace

import (
	"context"
	"sort"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/token"
)

type captured struct {
	cut      uint64
	accepted accepted
	contact  contact
}

func (f *Owner) captureLocked(k keyData) (captured, error) {
	if err := f.available(); err != nil {
		return captured{}, err
	}
	// A fenced element never serves new captures again; an accepted but not
	// installed namespace stays unavailable until the unique Core switch is
	// verified and installed here.
	if _, fenced := f.state.Fenced[string(k.Element)]; fenced {
		return captured{}, contract.ErrUnavailable
	}
	if _, pending := f.state.Pending[k.id()]; pending {
		return captured{}, contract.ErrUnavailable
	}
	a, ok := f.state.Acceptances[k.id()]
	c, connected := f.state.Contacts[k.id()]
	if !ok || !connected || a.Key != k || a.At == 0 || a.At > f.state.Cut || !c.Address.Valid() || c.Serial == 0 || c.Serial > f.state.Cut {
		return captured{}, contract.ErrUnavailable
	}
	return captured{f.state.Cut, a, c}, nil
}
func (f *Owner) Get(ctx context.Context, actor values.PrincipalRef, key values.FacetAcceptanceKey, typ values.FacetTypeRef) (contract.ContentRead, error) {
	return f.read(ctx, actor, key, typ, false)
}
func (f *Owner) ListTypes(ctx context.Context, actor values.PrincipalRef, key values.FacetAcceptanceKey) (contract.ContentRead, error) {
	return f.read(ctx, actor, key, values.FacetTypeRef{}, true)
}
func (f *Owner) read(ctx context.Context, actor values.PrincipalRef, key values.FacetAcceptanceKey, typ values.FacetTypeRef, list bool) (contract.ContentRead, error) {
	if ctx == nil || !f.validContext(key) || (!list && !typ.Valid()) {
		return contract.ContentRead{}, contract.ErrInvalid
	}
	permission := contract.PermissionGet
	if list {
		permission = contract.PermissionListTypes
	}
	approval, err := f.authorize(actor, permission, key, typ)
	if err != nil {
		return contract.ContentRead{}, err
	}
	k := packKey(key)
	f.mu.Lock()
	snapshot, err := f.captureLocked(k)
	f.mu.Unlock()
	if err != nil {
		return contract.ContentRead{}, err
	}
	e, sourceErr := f.cores.ReadAttachment(ctx, snapshot.contact.Address, key)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err = f.available(); err != nil {
		return contract.ContentRead{}, err
	}
	if !f.settledContact(approval) {
		return contract.ContentRead{}, contract.ErrUnavailable
	}
	if f.state.Cut != snapshot.cut {
		return contract.ContentRead{}, contract.ErrSnapshotChanged
	}
	if sourceErr != nil || ctx.Err() != nil || !packProof(e).valid(k, false) {
		return contract.ContentRead{}, contract.ErrUnavailable
	}
	if code := coreRejection(packProof(e), k); code != "" {
		return contract.ContentRead{Core: e}, rejectionErrors[code]
	}
	r := contract.ContentRead{Cut: cutValue(f.scope, snapshot.cut), Acceptance: contract.Acceptance{Key: key, AcceptedAt: cutValue(f.scope, snapshot.accepted.At)}, Core: e}
	if list {
		r.Types = []values.FacetTypeRef{}
		for _, v := range f.state.Records {
			if v.Element == key.Element {
				t, _ := values.NewFacetTypeRef(v.Type)
				r.Types = append(r.Types, t)
			}
		}
		sort.Slice(r.Types, func(i, j int) bool { return r.Types[i].Less(r.Types[j]) })
		return r, nil
	}
	v, found := f.state.Records[recordID(key.Element, typ.Bytes())]
	if !found {
		return r, contract.ErrKeyNotFound
	}
	r.Payload = append([]byte(nil), v.Payload...)
	r.Revision = v.Revision.value()
	return r, nil
}

func (f *Owner) Put(ctx context.Context, actor values.PrincipalRef, key values.FacetAcceptanceKey, typ values.FacetTypeRef, payload []byte, pre contract.PutPrecondition, op contract.OperationID) (contract.MutationResult, error) {
	if ctx == nil || !f.validContext(key) || !typ.Valid() || !token.ValidPutPrecondition(pre) || !validOperation(op) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	if len(payload) > maxPayloadBytes {
		return contract.MutationResult{}, contract.ErrCapacity
	}
	approval, err := f.authorize(actor, contract.PermissionPut, key, typ)
	if err != nil {
		return contract.MutationResult{}, err
	}
	c := f.base(actor, key, op, "put")
	c.Type = typ.Bytes()
	c.Payload = append([]byte(nil), payload...)
	absent, preRevision := token.PutPreconditionParts(pre)
	c.Absent = absent
	c.Expected = packRevision(preRevision)
	return f.write(ctx, c, nil, approval)
}
func (f *Owner) Delete(ctx context.Context, actor values.PrincipalRef, key values.FacetAcceptanceKey, typ values.FacetTypeRef, expected contract.Revision, op contract.OperationID) (contract.MutationResult, error) {
	if ctx == nil || !f.validContext(key) || !typ.Valid() || !token.ValidRevision(expected) || !validOperation(op) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := f.authorize(actor, contract.PermissionDelete, key, typ)
	if err != nil {
		return contract.MutationResult{}, err
	}
	c := f.base(actor, key, op, "delete")
	c.Type = typ.Bytes()
	c.Expected = packRevision(expected)
	return f.write(ctx, c, nil, approval)
}
func (f *Owner) write(ctx context.Context, c command, gate *facetPermit, approval contract.RealmAttachment) (contract.MutationResult, error) {
	f.mu.Lock()
	if ctx.Err() != nil || !f.settledContact(approval) {
		f.mu.Unlock()
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	if gate != nil {
		if err := gate.guardLocked(ctx, false); err != nil {
			f.mu.Unlock()
			return contract.MutationResult{}, err
		}
	}
	if r, err, found := f.lookupLocked(c); found {
		f.mu.Unlock()
		return r, err
	}
	snapshot, err := f.captureLocked(c.Key)
	f.mu.Unlock()
	if err != nil {
		return contract.MutationResult{}, err
	}
	e, err := f.cores.ReadAttachment(ctx, snapshot.contact.Address, c.Key.value())
	p := packProof(e)
	if err != nil || ctx.Err() != nil || !p.valid(c.Key, false) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	if gate != nil {
		if err := gate.recheck(ctx, false); err != nil {
			return contract.MutationResult{}, err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil || !f.settledContact(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	if gate != nil {
		if err := gate.guardLocked(ctx, false); err != nil {
			return contract.MutationResult{}, err
		}
	}
	return f.commitLocked(c, p, departureData{}, transferPlan{})
}
