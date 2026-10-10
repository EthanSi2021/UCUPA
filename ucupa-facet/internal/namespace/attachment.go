package namespace

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/token"
)

func (f *Owner) AcceptAttachment(ctx context.Context, actor values.PrincipalRef, key values.FacetAcceptanceKey, address values.NextHop, op contract.OperationID) (contract.MutationResult, error) {
	if ctx == nil || !f.validContext(key) || !address.Valid() || !validOperation(op) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := f.authorize(actor, contract.PermissionAccept, key, values.FacetTypeRef{})
	if err != nil {
		return contract.MutationResult{}, err
	}
	c := f.base(actor, key, op, "accept")
	c.Address = address
	f.mu.Lock()
	if !f.settledContact(approval) {
		f.mu.Unlock()
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	if r, err, found := f.lookupLocked(c); found {
		f.mu.Unlock()
		return r, err
	}
	if _, found := f.state.Acceptances[c.Key.id()]; found {
		r, err := f.commitLocked(c, proof{}, departureData{}, transferPlan{})
		f.mu.Unlock()
		return r, err
	}
	f.mu.Unlock()
	evidence, err := f.cores.ReadProposal(ctx, address, key)
	if err != nil || ctx.Err() != nil {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	// The plain proposal shape keeps its persistent decision inside the commit,
	// including a recorded rejection when the pending key does not match; only
	// evidence outside every known shape fails before the journal.
	p := packProof(evidence)
	var departure departureData
	if p.validReaccept(c.Key) {
		departure, err = f.reacceptanceDeparture(ctx, p)
		if err != nil {
			return contract.MutationResult{}, err
		}
	} else if !p.validRecovery(c.Key) && !p.valid(c.Key, true) {
		// A local detach's recovery needs no departure statement: the same
		// Core's DETACHED disclosure is the revocation evidence.
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.settledContact(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	return f.commitLocked(c, p, departure, transferPlan{})
}

// reacceptanceDeparture reads the previous Core's durable successor statement
// for the predecessor key. A missing recorded contact returns no departure and
// the decision itself rejects persistently; an unreachable previous Core stays
// a temporary failure for a later retry, while a reachable proven absence
// returns a statement the decision rejects persistently.
func (f *Owner) reacceptanceDeparture(ctx context.Context, p proof) (departureData, error) {
	f.mu.Lock()
	conn, connected := f.state.Contacts[p.Active.id()]
	f.mu.Unlock()
	if !connected || !conn.Address.Valid() {
		return departureData{}, nil
	}
	previous := p.Active.value()
	statement, err := f.cores.ReadSuccessor(ctx, conn.Address, previous.Core, previous.Element)
	if err != nil || ctx.Err() != nil {
		return departureData{}, contract.ErrUnavailable
	}
	return packDeparture(statement), nil
}
func (f *Owner) ReadAcceptance(actor values.PrincipalRef, key values.FacetAcceptanceKey) (contract.AcceptanceRead, error) {
	if !f.validContext(key) {
		return contract.AcceptanceRead{}, contract.ErrInvalid
	}
	approval, err := f.authorize(actor, contract.PermissionReadAcceptance, key, values.FacetTypeRef{})
	if err != nil {
		return contract.AcceptanceRead{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.settledContact(approval) {
		return contract.AcceptanceRead{}, contract.ErrUnavailable
	}
	return f.readAcceptanceLocked(key)
}

func (f *Owner) readAcceptanceLocked(key values.FacetAcceptanceKey) (contract.AcceptanceRead, error) {
	if err := f.available(); err != nil {
		return contract.AcceptanceRead{}, err
	}
	r := contract.AcceptanceRead{Cut: cutValue(f.scope, f.state.Cut)}
	a, ok := f.state.Acceptances[packKey(key).id()]
	if !ok {
		return r, nil
	}
	c, ok := f.state.Contacts[packKey(key).id()]
	if !ok || !c.Address.Valid() || c.Serial == 0 || c.Serial > f.state.Cut || a.At == 0 || a.At > f.state.Cut {
		return contract.AcceptanceRead{}, contract.ErrUnavailable
	}
	r.Found = true
	r.Acceptance = contract.Acceptance{Key: key, AcceptedAt: cutValue(f.scope, a.At)}
	r.CoreNextHop = c.Address
	r.NextHopRevision = token.NewContactRevision(f.scope, key, c.Serial)
	return r, nil
}
func (f *Owner) UpdateCoreNextHop(actor values.PrincipalRef, op contract.OperationID, key values.FacetAcceptanceKey, expected contract.ContactRevision, address values.NextHop) (contract.MutationResult, error) {
	if !f.validContext(key) || !validOperation(op) || !token.ValidContactRevision(expected) || !address.Valid() {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := f.authorize(actor, contract.PermissionCoreContactUpdate, key, values.FacetTypeRef{})
	if err != nil {
		return contract.MutationResult{}, err
	}
	c := f.base(actor, key, op, "contact")
	c.Address = address
	scope, expectedKey, serial := token.ContactRevisionParts(expected)
	c.ContactScope = scope
	c.ContactKey = packKey(expectedKey)
	c.ContactSerial = serial
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.settledContact(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	return f.commitLocked(c, proof{}, departureData{}, transferPlan{})
}
