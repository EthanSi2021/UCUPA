package namespace

import (
	"context"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/token"
)

// UpdateCoreContactControl changes only the accepted relation's reverse
// address. It works while Core is unavailable and never resolves by bare Ref.
func (f *Owner) UpdateCoreContactControl(ctx context.Context, caller values.CallerContext, id facetcontract.OperationID, ref facetcontract.PublicAcceptanceRef, expected values.PublicToken, address values.NextHop) (facetcontract.PublicAcceptanceOutcome, error) {
	if f == nil || f.ownerState == nil || !validAcceptanceRef(ref) || !validOperation(id) || !address.Valid() || !expected.Valid() || expected.Kind != "contact-revision" || expected.Owner != f.authority {
		return facetcontract.PublicAcceptanceOutcome{}, values.PublicInvalid
	}
	guard, release, err := f.acceptanceGuard(ctx, caller, ref.Element, facetcontract.PermissionCoreContactUpdate)
	if err != nil {
		return facetcontract.PublicAcceptanceOutcome{}, err
	}
	defer release()
	request := publicAcceptanceRequest("contact", ref, address, id, expected)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err = f.acceptanceAvailableLocked(ctx, guard); err != nil {
		return facetcontract.PublicAcceptanceOutcome{}, err
	}
	if result, err, found := f.acceptanceReplayLocked(caller, id, request, ref); found {
		return result, err
	}
	key, found, err := f.publicAcceptanceKeyLocked(ref)
	if err != nil {
		return facetcontract.PublicAcceptanceOutcome{}, err
	}
	if !found {
		return facetcontract.PublicAcceptanceOutcome{}, values.PublicNotFound
	}
	r, err := f.readAcceptanceLocked(key)
	if err != nil {
		return facetcontract.PublicAcceptanceOutcome{}, publicFacetError(err)
	}
	if !r.Found || f.publicContactRevision(r.NextHopRevision) != expected {
		return facetcontract.PublicAcceptanceOutcome{}, values.PublicConflict
	}
	c := f.base(caller.Subject, key, id, "contact")
	c.PublicRequest, c.Address = request, address
	scope, _, serial := token.ContactRevisionParts(r.NextHopRevision)
	c.ContactScope, c.ContactKey, c.ContactSerial = scope, packKey(key), serial
	result, err := f.commitLocked(c, proof{}, departureData{}, transferPlan{})
	if err == nil {
		err = f.acceptanceAvailableLocked(ctx, guard)
	}
	return f.finishPublicAcceptance(ref, result, err)
}
