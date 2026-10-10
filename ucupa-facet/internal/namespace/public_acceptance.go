package namespace

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/token"
)

func validAcceptanceRef(r contract.PublicAcceptanceRef) bool {
	return r.Core.Valid() && r.Element.Valid() && r.Proposal.Valid() && r.Proposal.Owner == r.Core && r.Proposal.Kind == "attachment-revision"
}

func (f *Owner) acceptanceGuard(ctx context.Context, caller values.CallerContext, element values.ElementID, permission values.PermissionID) (*facetPermit, func(), error) {
	if f == nil || f.ownerState == nil || f.public == nil {
		return nil, nil, values.PublicUnavailable
	}
	if ctx == nil || !element.Valid() {
		return nil, nil, values.PublicInvalid
	}
	if !f.public.Enter() {
		return nil, nil, values.PublicResourceExhausted
	}
	release := func() { f.public.Leave() }
	req := f.publicRequirements(element, values.FacetTypeRef{}, permission, false)
	permit, err := f.authorizePublic(ctx, caller, req)
	if err != nil {
		release()
		return nil, nil, err
	}
	return permit, func() { permit.release(ctx); f.public.DrainRetired(ctx); release() }, nil
}

// AcceptAttachmentControl retains the original authenticated authorization
// view across proposal acquisition. It shares the sole accept transition and
// operation namespace with local calls; no adapter can submit a subject name.
func (f *Owner) AcceptAttachmentControl(ctx context.Context, caller values.CallerContext, ref contract.PublicAcceptanceRef, address values.NextHop, id contract.OperationID) (contract.PublicAcceptanceOutcome, error) {
	if !validAcceptanceRef(ref) || !address.Valid() || !validOperation(id) {
		return contract.PublicAcceptanceOutcome{}, values.PublicInvalid
	}
	guard, release, err := f.acceptanceGuard(ctx, caller, ref.Element, contract.PermissionAccept)
	if err != nil {
		return contract.PublicAcceptanceOutcome{}, err
	}
	defer release()
	request := publicAcceptanceRequest("accept", ref, address, id, values.PublicToken{})
	f.mu.Lock()
	if err = f.acceptanceAvailableLocked(ctx, guard); err != nil {
		f.mu.Unlock()
		return contract.PublicAcceptanceOutcome{}, err
	}
	if result, err, found := f.acceptanceReplayLocked(caller, id, request, ref); found {
		f.mu.Unlock()
		return result, err
	}
	key, found, err := f.publicAcceptanceKeyLocked(ref)
	if err != nil {
		f.mu.Unlock()
		return contract.PublicAcceptanceOutcome{}, err
	}
	if found {
		c := f.base(caller.Subject, key, id, "accept")
		c.Address, c.PublicRequest = address, request
		result, err := f.commitLocked(c, proof{}, departureData{}, transferPlan{})
		if err == nil {
			err = f.acceptanceAvailableLocked(ctx, guard)
		}
		out, err := f.finishPublicAcceptance(ref, result, err)
		f.mu.Unlock()
		return out, err
	}
	f.mu.Unlock()
	source, ok := f.cores.(coreProposalSource)
	if !ok {
		return contract.PublicAcceptanceOutcome{}, values.PublicUnavailable
	}
	resolved, err := source.ResolveProposal(ctx, address, ref)
	if err != nil {
		return contract.PublicAcceptanceOutcome{}, facetControlError(err)
	}
	p := packProof(resolved.Evidence)
	k := packKey(resolved.Key)
	if !f.validContext(resolved.Key) || resolved.Key.Core != ref.Core || resolved.Key.Element != ref.Element ||
		resolved.Proposal != ref.Proposal {
		return contract.PublicAcceptanceOutcome{}, values.PublicUnavailable
	}
	// The plain proposal shape keeps its persistent decision inside the commit,
	// including a recorded rejection when the pending key does not match; only
	// evidence outside every known shape fails before the journal.
	var departure departureData
	if p.validReaccept(k) {
		departure, err = f.reacceptanceDeparture(ctx, p)
		if err != nil {
			return contract.PublicAcceptanceOutcome{}, publicFacetError(err)
		}
	} else if !p.validRecovery(k) && !p.valid(k, true) {
		// A local detach's recovery needs no departure statement: the same
		// Core's DETACHED disclosure is the revocation evidence.
		return contract.PublicAcceptanceOutcome{}, values.PublicUnavailable
	}
	if err := guard.recheck(ctx, false); err != nil {
		return contract.PublicAcceptanceOutcome{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err = f.acceptanceAvailableLocked(ctx, guard); err != nil {
		return contract.PublicAcceptanceOutcome{}, err
	}
	if result, err, found := f.acceptanceReplayLocked(caller, id, request, ref); found {
		return result, err
	}
	// A concurrent accept may have established the same relation. The common
	// transition returns UNCHANGED without replacing its initial contact.
	c := f.base(caller.Subject, resolved.Key, id, "accept")
	c.Address, c.PublicRequest = address, request
	result, err := f.commitLocked(c, p, departure, transferPlan{})
	if err == nil {
		err = f.acceptanceAvailableLocked(ctx, guard)
	}
	return f.finishPublicAcceptance(ref, result, err)
}

func (f *Owner) finishPublicAcceptance(ref contract.PublicAcceptanceRef, result contract.MutationResult, err error) (contract.PublicAcceptanceOutcome, error) {
	if err != nil {
		return contract.PublicAcceptanceOutcome{}, publicFacetError(err)
	}
	return f.publicAcceptanceOutcome(ref, result), nil
}

func (f *Owner) publicAcceptanceOutcome(ref contract.PublicAcceptanceRef, r contract.MutationResult) contract.PublicAcceptanceOutcome {
	inspection := contract.PublicAcceptanceInspection{Ref: ref, Found: r.Acceptance.Key.Valid()}
	if token.ValidContactRevision(r.NextHopRevision) {
		inspection.CoreNextHop = r.CoreNextHop
		inspection.ContactRevision = f.publicContactRevision(r.NextHopRevision)
	}
	return contract.PublicAcceptanceOutcome{Status: r.Status, Inspection: inspection}
}

func (f *Owner) acceptanceReplayLocked(caller values.CallerContext, id contract.OperationID, request string, ref contract.PublicAcceptanceRef) (contract.PublicAcceptanceOutcome, error, bool) {
	i, found := f.operations[byteString(id)]
	if !found {
		return contract.PublicAcceptanceOutcome{}, nil, false
	}
	e := f.events[i]
	actor := actorData{Realm: byteString(caller.Subject.Realm), Principal: byteString(caller.Subject.Principal)}
	if e.Input.Actor != actor || e.Input.PublicRequest != request {
		return contract.PublicAcceptanceOutcome{}, values.PublicConflict, true
	}
	out, err := f.finishPublicAcceptance(ref, e.Outcome.result(f.scope, e.Input.Key), rejectionErrors[e.Outcome.Error])
	return out, err, true
}

// InspectAcceptanceControl uses the retained public proposal binding. It does
// not contact Core or confuse an accepted historical proposal with activation.
func (f *Owner) InspectAcceptanceControl(ctx context.Context, caller values.CallerContext, ref contract.PublicAcceptanceRef) (contract.PublicAcceptanceInspection, error) {
	if !validAcceptanceRef(ref) {
		return contract.PublicAcceptanceInspection{}, values.PublicInvalid
	}
	guard, release, err := f.acceptanceGuard(ctx, caller, ref.Element, contract.PermissionReadAcceptance)
	if err != nil {
		return contract.PublicAcceptanceInspection{}, err
	}
	defer release()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err = f.acceptanceAvailableLocked(ctx, guard); err != nil {
		return contract.PublicAcceptanceInspection{}, err
	}
	key, found, err := f.publicAcceptanceKeyLocked(ref)
	if err != nil {
		return contract.PublicAcceptanceInspection{}, err
	}
	if !found {
		// A locally-created acceptance has no authenticated public proposal
		// comparison. Do not report it absent just because no binding exists.
		for _, a := range f.state.Acceptances {
			if a.Key.Core == ref.Core && a.Key.Element == ref.Element && !f.hasPublicAcceptanceBinding(a.Key) {
				return contract.PublicAcceptanceInspection{}, values.PublicUnavailable
			}
		}
		return contract.PublicAcceptanceInspection{Ref: ref}, nil
	}
	r, err := f.readAcceptanceLocked(key)
	if err != nil {
		return contract.PublicAcceptanceInspection{}, publicFacetError(err)
	}
	return contract.PublicAcceptanceInspection{Ref: ref, Found: r.Found, CoreNextHop: r.CoreNextHop, ContactRevision: f.publicContactRevision(r.NextHopRevision)}, nil
}

// ReadAcceptanceControl is a fixed-Core-service port. Its private evidence may
// be consumed only by the activation handshake, never returned to end users.
func (f *Owner) ReadAcceptanceControl(ctx context.Context, caller values.CallerContext, key values.FacetAcceptanceKey) (contract.AcceptanceRead, error) {
	if f == nil || !f.validContext(key) {
		return contract.AcceptanceRead{}, values.PublicInvalid
	}
	guard, release, err := f.acceptanceGuard(ctx, caller, key.Element, contract.PermissionReadAcceptance)
	if err != nil {
		return contract.AcceptanceRead{}, err
	}
	defer release()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err = f.acceptanceAvailableLocked(ctx, guard); err != nil {
		return contract.AcceptanceRead{}, err
	}
	return f.readAcceptanceLocked(key)
}

func (f *Owner) acceptanceAvailableLocked(ctx context.Context, permit *facetPermit) error {
	if err := f.publicAvailableLocked(); err != nil {
		return err
	}
	return permit.guardLocked(ctx, false)
}
