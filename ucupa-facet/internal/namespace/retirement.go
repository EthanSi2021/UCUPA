package namespace

import (
	"bytes"
	"context"
	"encoding/binary"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
)

// validRetirement checks the disclosure that authorizes the explicit disposal
// of one element's content in this Facet: the controlling Core's record is
// DETACHED and names this very acceptance key — either as the stored lineage
// key after its revocation, or as the still-open proposal this Facet already
// accepted without a completed activation. The Core's release that consumes
// the disposition clears the open proposal; the disclosure itself never
// certifies content removal.
func (p proof) validRetirement(k keyData) bool {
	e := p.value()
	if !e.Cut.Valid() || e.Authority != k.Core || e.Element != k.Element || p.State != contract.CoreDetached {
		return false
	}
	if !e.AttachmentRevision.Valid() || !bytes.Equal(p.Revision[:32], p.Cut[:32]) ||
		string(p.Revision[32:68]) != string(k.Element) || binary.BigEndian.Uint64(p.Revision[68:]) > binary.BigEndian.Uint64(p.Cut[32:]) {
		return false
	}
	if p.Active == k {
		// The revoked-lineage shape: this very key is the stored predecessor.
		// An open proposal on the same DETACHED record may only be this
		// Facet's own recovery; retiring it closes that path durably.
		if p.Pending != (keyData{}) && (!p.Pending.value().Valid() || p.Pending.Core != k.Core ||
			p.Pending.Facet != k.Facet || p.Pending.Element != k.Element || p.Pending.Proposal != p.Revision) {
			return false
		}
		return bytes.Equal(p.Active.Proposal[:32], p.Revision[:32]) &&
			binary.BigEndian.Uint64(p.Active.Proposal[68:]) <= binary.BigEndian.Uint64(p.Revision[68:])
	}
	if p.Pending != k || !p.Pending.value().Valid() || p.Pending.Proposal != p.Revision {
		return false
	}
	// The activation-obligation shape: this Facet already holds the acceptance
	// of the still-open proposal, so the stored lineage key is its predecessor.
	return p.Active.value().Valid() && p.Active.Facet == k.Facet && p.Active.Element == k.Element
}

// validRetireDeparture checks the third disposal shape: the element left the
// acceptance's Core under a completed transfer, so that old Core proves the
// element gone and its own durable successor statement is the disposal
// evidence. The statement binds this retired lineage's departure to a definite
// successor; the new Core consumes the recorded disposition at its release.
func (p proof) validRetireDeparture(k keyData, d departureData) bool {
	return p.State == contract.CoreMissing && p.valid(k, false) && d.validDeparture(k)
}

// validDeparture checks the disposal evidence of a retired lineage whose
// element left the acceptance's Core under a completed transfer: the
// departed Core itself must state the element's durable departure to a
// definite successor under the original transfer operation. A plain deletion
// answers no statement, so it never disposes of a held obligation.
func (d departureData) validDeparture(k keyData) bool {
	if !d.Found || !validOperation(contract.OperationID(d.Operation)) {
		return false
	}
	return d.Authority == k.Core && d.Source == d.Authority && d.Target.Valid() && d.Target != k.Core &&
		d.Element == k.Element && d.FinalCut > 0
}

// decideRetire is the deterministic owner transition of the explicit content
// retirement. The controlling Core's recorded disclosure is input evidence,
// never a callback during replay. One transaction records the exact final
// disposition — the retired acceptance key, the disposition cut and the
// ordered manifest digest of the closed namespace — then removes the live
// content bytes and the retired relationship. Tombstones, the disposition
// record and the journal stay for anti-duplication and continuity; identities
// and old authorizations are never reused.
func decideRetire(s *facetState, c command, p proof, d departureData) (outcome, error) {
	out := outcome{Status: contract.Rejected, Cut: s.Cut}
	reject := func(code string) (outcome, error) { out.Error = code; return out, nil }
	if _, retired := s.Retired[string(c.Key.Element)]; retired {
		return reject("retired")
	}
	if _, fenced := s.Fenced[string(c.Key.Element)]; fenced {
		return reject("fenced")
	}
	for _, pending := range s.Pending {
		if pending.Element == c.Key.Element {
			return reject("pending")
		}
	}
	id := c.Key.id()
	if _, exists := s.Acceptances[id]; !exists {
		return reject("acceptance")
	}
	if !p.validRetirement(c.Key) && !p.validRetireDeparture(c.Key, d) {
		return outcome{}, contract.ErrUnavailable
	}
	// The disposal records the exact closed namespace whatever its size: the
	// disposition manifest is local evidence that never travels the wire —
	// only the disposition key and cut are ever disclosed — and the store
	// keeps it beside the tombstones it summarizes. The legal write rules let
	// tombstones accumulate without bound, so a manifest bound here would
	// strand content its own rules produced; the wire bound stays on the
	// transfer path, where the manifest actually crosses processes.
	manifest := buildManifest(s, c.Key.Element)
	if err := nextCut(s); err != nil {
		return outcome{}, err
	}
	s.Retired[string(c.Key.Element)] = retiredNamespace{Key: c.Key, Cut: s.Cut, Digest: manifestDigest(manifest), Manifest: manifest}
	delete(s.Acceptances, id)
	delete(s.Contacts, id)
	for key, r := range s.Records {
		if r.Element == c.Key.Element {
			delete(s.Records, key)
		}
	}
	out.Status = contract.Committed
	out.Cut = s.Cut
	return out, nil
}

// retiredDeparture reads the departure evidence of the transferred disposal
// shape: the contacted Core proved the element gone, so its own durable
// successor statement must name the element's definite successor. The read
// stays outside the commit lock, mirroring the re-acceptance departure; a
// lost Core, an unreachable statement and a plain deletion all fail before
// the journal and keep the request retryable.
func (f *Owner) retiredDeparture(ctx context.Context, p proof, k keyData, address values.NextHop) (departureData, error) {
	if p.State != contract.CoreMissing || !p.valid(k, false) {
		return departureData{}, contract.ErrUnavailable
	}
	statement, err := f.cores.ReadSuccessor(ctx, address, k.Core, k.Element)
	if err != nil || ctx.Err() != nil {
		return departureData{}, contract.ErrUnavailable
	}
	departure := packDeparture(statement)
	if !departure.validDeparture(k) {
		return departureData{}, contract.ErrUnavailable
	}
	return departure, nil
}

// RetireContent disposes of one held acceptance's content on an explicit
// request. The controlling Core's DETACHED disclosure naming this key is the
// local disposal evidence; after a completed Core transfer, the departed
// Core's proven missing disclosure plus its durable successor statement
// dispose of the still-open obligation instead. A lost Core contact keeps
// the request retryable, never deletes, and a second retire of the disposed
// namespace is refused durably.
func (f *Owner) RetireContent(ctx context.Context, actor values.PrincipalRef, key values.FacetAcceptanceKey, op contract.OperationID) (contract.MutationResult, error) {
	if ctx == nil || !f.validContext(key) || !validOperation(op) {
		return contract.MutationResult{}, contract.ErrInvalid
	}
	approval, err := f.authorize(actor, contract.PermissionRetireContent, key, values.FacetTypeRef{})
	if err != nil {
		return contract.MutationResult{}, err
	}
	c := f.base(actor, key, op, "retire")
	k := packKey(key)
	f.mu.Lock()
	if !f.settledContact(approval) {
		f.mu.Unlock()
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	if r, err, found := f.lookupLocked(c); found {
		f.mu.Unlock()
		return r, err
	}
	address, err := f.contactLocked(k)
	f.mu.Unlock()
	if err != nil {
		// Without a live contact the decision itself refuses persistently
		// (no acceptance or already retired) or fails temporarily (a broken
		// contact stays repairable); no evidence is read.
		f.mu.Lock()
		if !f.settledContact(approval) {
			f.mu.Unlock()
			return contract.MutationResult{}, contract.ErrUnavailable
		}
		result, err := f.commitLocked(c, proof{}, departureData{}, transferPlan{})
		f.mu.Unlock()
		return result, err
	}
	evidence, err := f.cores.ReadProposal(ctx, address, key)
	if err != nil || ctx.Err() != nil {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	p := packProof(evidence)
	var departure departureData
	if !p.validRetirement(k) {
		// Evidence outside the local retirement shapes fails before the
		// journal so a mid-transition Core can be retried against a later
		// disclosure; only a proven departure to a definite successor
		// disposes of the transferred obligation.
		departure, err = f.retiredDeparture(ctx, p, k, address)
		if err != nil {
			return contract.MutationResult{}, err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.settledContact(approval) {
		return contract.MutationResult{}, contract.ErrUnavailable
	}
	return f.commitLocked(c, p, departure, transferPlan{})
}

func publicRetireRequest(ref contract.PublicAcceptanceRef, id contract.OperationID) string {
	r := map[string]any{"op": "retire", "core": string(ref.Core), "element": string(ref.Element),
		"proposal": requestvalue.TokenJSON(ref.Proposal), "operationId": values.EncodePublicBytes([]byte(id))}
	raw, _ := values.CanonicalJSONValue(r)
	return string(raw)
}

// RetireContentControl retains the original authenticated authorization view
// across the Core evidence read. It shares the sole retire transition and
// operation namespace with local calls; no adapter can submit a subject name.
func (f *Owner) RetireContentControl(ctx context.Context, caller values.CallerContext, ref contract.PublicAcceptanceRef, id contract.OperationID) (contract.PublicAcceptanceOutcome, error) {
	if !validAcceptanceRef(ref) || !validOperation(id) {
		return contract.PublicAcceptanceOutcome{}, values.PublicInvalid
	}
	guard, release, err := f.acceptanceGuard(ctx, caller, ref.Element, contract.PermissionRetireContent)
	if err != nil {
		return contract.PublicAcceptanceOutcome{}, err
	}
	defer release()
	request := publicRetireRequest(ref, id)
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
	if !found {
		f.mu.Unlock()
		return contract.PublicAcceptanceOutcome{}, values.PublicUnavailable
	}
	k := packKey(key)
	address, err := f.contactLocked(k)
	f.mu.Unlock()
	if err != nil {
		f.mu.Lock()
		if err := f.acceptanceAvailableLocked(ctx, guard); err != nil {
			f.mu.Unlock()
			return contract.PublicAcceptanceOutcome{}, err
		}
		c := f.base(caller.Subject, key, id, "retire")
		c.PublicRequest = request
		result, err := f.commitLocked(c, proof{}, departureData{}, transferPlan{})
		if err == nil {
			err = f.acceptanceAvailableLocked(ctx, guard)
		}
		out, err := f.finishPublicAcceptance(ref, result, err)
		f.mu.Unlock()
		return out, err
	}
	evidence, err := f.cores.ReadProposal(ctx, address, key)
	if err != nil || ctx.Err() != nil {
		return contract.PublicAcceptanceOutcome{}, values.PublicUnavailable
	}
	p := packProof(evidence)
	var departure departureData
	if !p.validRetirement(k) {
		// Evidence outside the local retirement shapes fails before the
		// journal so a mid-transition Core can be retried against a later
		// disclosure; only a proven departure to a definite successor
		// disposes of the transferred obligation.
		departure, err = f.retiredDeparture(ctx, p, k, address)
		if err != nil {
			return contract.PublicAcceptanceOutcome{}, values.PublicUnavailable
		}
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
	c := f.base(caller.Subject, key, id, "retire")
	c.PublicRequest = request
	result, err := f.commitLocked(c, p, departure, transferPlan{})
	if err == nil {
		err = f.acceptanceAvailableLocked(ctx, guard)
	}
	return f.finishPublicAcceptance(ref, result, err)
}

// ReadDispositionControl is a fixed-Core-service port. It discloses the
// durable final disposition of one element's retired namespace — the retired
// acceptance key and the cut that recorded it — and nothing else.
func (f *Owner) ReadDispositionControl(ctx context.Context, caller values.CallerContext, element values.ElementID) (contract.DispositionRead, error) {
	if f == nil || !element.Valid() {
		return contract.DispositionRead{}, values.PublicInvalid
	}
	guard, release, err := f.acceptanceGuard(ctx, caller, element, contract.PermissionReadAcceptance)
	if err != nil {
		return contract.DispositionRead{}, err
	}
	defer release()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err = f.acceptanceAvailableLocked(ctx, guard); err != nil {
		return contract.DispositionRead{}, err
	}
	if err := f.available(); err != nil {
		return contract.DispositionRead{}, publicFacetError(err)
	}
	if retired, ok := f.state.Retired[string(element)]; ok {
		return contract.DispositionRead{Found: true, Key: retired.Key.value(), Cut: cutValue(f.scope, retired.Cut)}, nil
	}
	return contract.DispositionRead{}, nil
}
