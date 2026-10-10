package namespace

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
	"ucupa/ucupa-facet/internal/token"
)

type keyData struct {
	Core     values.FactAuthorityRef
	Facet    values.FactAuthorityRef
	Element  values.ElementID
	Proposal [76]byte
}

func packKey(k values.FacetAcceptanceKey) keyData {
	v := keyData{Core: k.Core, Facet: k.Facet, Element: k.Element}
	copy(v.Proposal[:], k.Proposal.Bytes())
	return v
}
func (k keyData) value() values.FacetAcceptanceKey {
	p, _ := values.NewAttachmentRevision(k.Proposal[:])
	return values.FacetAcceptanceKey{Core: k.Core, Facet: k.Facet, Element: k.Element, Proposal: p}
}
func (k keyData) id() string { raw, _ := json.Marshal(k); return string(raw) }

type revisionData struct {
	Scope   [32]byte
	Element values.ElementID
	Type    []byte
	Serial  uint64
}

func packRevision(r contract.Revision) revisionData {
	if token.IsZeroRevision(r) {
		return revisionData{}
	}
	scope, element, typ, serial := token.RevisionParts(r)
	return revisionData{scope, element, []byte(typ), serial}
}
func (r revisionData) value() contract.Revision {
	return token.NewRevision(r.Scope, r.Element, string(r.Type), r.Serial)
}

type proof struct {
	Authority values.FactAuthorityRef
	Element   values.ElementID
	State     contract.CoreState
	Revision  [76]byte
	Active    keyData
	Pending   keyData
	Cut       [40]byte
}

func packProof(e contract.CoreEvidence) proof {
	p := proof{Authority: e.Authority, Element: e.Element, State: e.State, Active: packKey(e.Active), Pending: packKey(e.Pending)}
	copy(p.Revision[:], e.AttachmentRevision.Bytes())
	copy(p.Cut[:], e.Cut.Bytes())
	return p
}
func (p proof) value() contract.CoreEvidence {
	r, _ := values.NewAttachmentRevision(p.Revision[:])
	c, _ := values.NewCoreCutEvidence(p.Cut[:])
	return contract.CoreEvidence{Authority: p.Authority, Element: p.Element, State: p.State, AttachmentRevision: r, Active: p.Active.value(), Pending: p.Pending.value(), Cut: c}
}
func (p proof) valid(k keyData, proposal bool) bool {
	e := p.value()
	if !e.Cut.Valid() || e.Authority != k.Core || e.Element != k.Element {
		return false
	}
	if p.State == contract.CoreMissing {
		return p.Revision == [76]byte{} && p.Active == keyData{} && p.Pending == keyData{}
	}
	if p.State < contract.CoreNone || p.State > contract.CoreDetached || !e.AttachmentRevision.Valid() {
		return false
	}
	if !bytes.Equal(p.Revision[:32], p.Cut[:32]) || string(p.Revision[32:68]) != string(k.Element) || binary.BigEndian.Uint64(p.Revision[68:]) > binary.BigEndian.Uint64(p.Cut[32:]) {
		return false
	}
	if p.State == contract.CoreNone && p.Active != (keyData{}) {
		return false
	}
	if p.State == contract.CoreActive || p.State == contract.CoreDetached {
		if !p.Active.value().Valid() || p.Active.Core != k.Core || p.Active.Element != k.Element || p.Pending != (keyData{}) {
			return false
		}
		if !bytes.Equal(p.Active.Proposal[:32], p.Revision[:32]) || binary.BigEndian.Uint64(p.Active.Proposal[68:]) > binary.BigEndian.Uint64(p.Revision[68:]) {
			return false
		}
	}
	if p.Pending != (keyData{}) {
		if !proposal || p.State != contract.CoreNone || !p.Pending.value().Valid() || p.Pending.Core != k.Core || p.Pending.Element != k.Element || p.Pending.Proposal != p.Revision {
			return false
		}
	}
	return true
}

// validReaccept checks the proposal shape exposed by an element's new Core
// after a completed Core transfer: DETACHED with this Facet's previous
// acceptance as the predecessor key and exactly one pending proposal for this
// Facet. It is mutually exclusive with the plain proposal shape, whose
// Core state is NONE with the same pending key, and with the local recovery
// shape, whose predecessor key stayed with the same Core.
func (p proof) validReaccept(k keyData) bool {
	e := p.value()
	if !e.Cut.Valid() || e.Authority != k.Core || e.Element != k.Element || p.State != contract.CoreDetached {
		return false
	}
	if !e.AttachmentRevision.Valid() || !bytes.Equal(p.Revision[:32], p.Cut[:32]) ||
		string(p.Revision[32:68]) != string(k.Element) || binary.BigEndian.Uint64(p.Revision[68:]) > binary.BigEndian.Uint64(p.Cut[32:]) {
		return false
	}
	if p.Pending != k || !p.Pending.value().Valid() || p.Pending.Proposal != p.Revision {
		return false
	}
	return p.Active.value().Valid() && p.Active.Facet == k.Facet && p.Active.Element == k.Element && p.Active.Core != k.Core
}

// validRecovery checks the proposal shape of the same Core after its local
// detach: DETACHED with this Facet's revoked acceptance key of the same Core
// as the predecessor and exactly one pending proposal for this Facet. The
// DETACHED disclosure itself is the revocation evidence, so no departure
// statement is read; the Facet's own predecessor acceptance remains the
// proof that the stored content was neither replaced nor retired.
func (p proof) validRecovery(k keyData) bool {
	e := p.value()
	if !e.Cut.Valid() || e.Authority != k.Core || e.Element != k.Element || p.State != contract.CoreDetached {
		return false
	}
	if !e.AttachmentRevision.Valid() || !bytes.Equal(p.Revision[:32], p.Cut[:32]) ||
		string(p.Revision[32:68]) != string(k.Element) || binary.BigEndian.Uint64(p.Revision[68:]) > binary.BigEndian.Uint64(p.Cut[32:]) {
		return false
	}
	if p.Pending != k || !p.Pending.value().Valid() || p.Pending.Proposal != p.Revision {
		return false
	}
	return p.Active.value().Valid() && p.Active.Facet == k.Facet && p.Active.Element == k.Element && p.Active.Core == k.Core
}

// departureData is the journal form of the previous Core's durable successor
// statement for the departed element. It is input evidence recorded next to
// exactly one re-acceptance decision, never a callback during replay.
type departureData struct {
	Found     bool
	Authority values.FactAuthorityRef
	Element   values.ElementID
	Operation byteString
	Source    values.FactAuthorityRef
	Target    values.FactAuthorityRef
	FinalCut  uint64
	Settled   bool
}

func packDeparture(r contract.CoreSuccessorRead) departureData {
	return departureData{Found: r.Found, Authority: r.Authority, Element: r.Element, Operation: byteString(r.Operation),
		Source: r.Source, Target: r.Target, FinalCut: r.FinalCut, Settled: r.Settled}
}
func (d departureData) value() contract.CoreSuccessorRead {
	return contract.CoreSuccessorRead{Found: d.Found, Authority: d.Authority, Element: d.Element, Operation: contract.OperationID(d.Operation),
		Source: d.Source, Target: d.Target, FinalCut: d.FinalCut, Settled: d.Settled}
}

// validSuccessor checks the departure proof of one re-acceptance: the previous
// Core itself must state the element's durable departure to exactly the new
// Core under the original transfer operation. A plain deletion, a temporarily
// undecided transfer and a carrier-controlled departure all fail here.
func (d departureData) validSuccessor(previous keyData, next keyData) bool {
	if !d.Found || !validOperation(contract.OperationID(d.Operation)) {
		return false
	}
	return d.Authority == previous.Core && d.Source == d.Authority && d.Target == next.Core &&
		d.Element == previous.Element && d.Element == next.Element && d.FinalCut > 0
}

// validAbsence checks the previous Core's proven-absence answer for the
// predecessor key: the Core itself answered and stated no durable departure
// for the element. That recorded answer is the evidence of a persistent
// refusal, indistinguishable from an ordinary deletion by design.
func (d departureData) validAbsence(previous keyData) bool {
	return !d.Found && d.Authority == previous.Core && d.Element == previous.Element &&
		d.Operation == "" && d.Source == "" && d.Target == "" && d.FinalCut == 0 && !d.Settled
}

// transferPlan is the recorded projection of the controlling Core's durable
// facet-replacement control record at command time. It is input evidence,
// never a callback during replay; an absent plan has an invalid Authority.
type transferPlan struct {
	Authority      values.FactAuthorityRef
	Element        values.ElementID
	Operation      byteString
	Source         values.FactAuthorityRef
	Target         values.FactAuthorityRef
	Proposal       [76]byte
	FinalCut       uint64
	ManifestDigest [32]byte
	Permitted      bool
	Fenced         bool
	Accepted       bool
	Committed      bool
	Settled        bool
	Cancelled      bool
}

func packPlan(r contract.CoreReplacementRead) transferPlan {
	p := transferPlan{Authority: r.Authority, Element: r.Element, Operation: byteString(r.Operation), Source: r.Source, Target: r.Target,
		FinalCut: r.FinalCut, ManifestDigest: r.ManifestDigest, Permitted: r.Permitted, Fenced: r.Fenced, Accepted: r.Accepted,
		Committed: r.Committed, Settled: r.Settled, Cancelled: r.Cancelled}
	copy(p.Proposal[:], r.Proposal.Bytes())
	return p
}
func (p transferPlan) value() contract.CoreReplacementRead {
	r, _ := values.NewAttachmentRevision(p.Proposal[:])
	return contract.CoreReplacementRead{Authority: p.Authority, Element: p.Element, Operation: contract.OperationID(p.Operation), Source: p.Source,
		Target: p.Target, Proposal: r, FinalCut: p.FinalCut, ManifestDigest: p.ManifestDigest, Permitted: p.Permitted, Fenced: p.Fenced,
		Accepted: p.Accepted, Committed: p.Committed, Settled: p.Settled, Cancelled: p.Cancelled}
}
func (p transferPlan) present() bool {
	return p.Authority.Valid() && p.Element != "" && p.Operation != "" && p.Source.Valid() && p.Target.Valid()
}

type accepted struct {
	Key keyData
	At  uint64
}
type contact struct {
	Address values.NextHop
	Serial  uint64
}
type record struct {
	Element  values.ElementID
	Type     []byte
	Payload  []byte
	Revision revisionData
}

// manifestEntryData is the journal form of one complete ordered namespace
// manifest entry at the source final cut.
type manifestEntryData struct {
	Type     []byte
	Live     bool
	Length   uint64
	Digest   [32]byte
	Revision revisionData
	Deleted  revisionData
}

// transferEntry carries the exact payload bytes of one live manifest entry.
type transferEntry struct {
	Type    []byte
	Payload []byte
}

// fencedNamespace is the permanent source-side fence evidence of one element.
// The manifest, final cut and source key prove the exact namespace content
// that left this owner; Settled only marks the recorded final effect.
type fencedNamespace struct {
	Core      values.FactAuthorityRef
	Operation byteString
	Target    values.FactAuthorityRef
	Cut       uint64
	Digest    [32]byte
	Manifest  []manifestEntryData
	SourceKey keyData
	Settled   bool
}

// refusalData is the durable source-side answer that this transfer will never
// be fenced here; it serializes against the fence inside one commit domain.
type refusalData struct {
	Core    values.FactAuthorityRef
	Element values.ElementID
	Target  values.FactAuthorityRef
}

// pendingInstall is the accepted but not yet installed target-side namespace.
// It keeps the durable acceptance invisible until the unique Core switch is
// verified, then installs its entries; installed revisions are reissued by
// the target owner, never the source tokens.
type pendingInstall struct {
	Core      values.FactAuthorityRef
	Element   values.ElementID
	Operation byteString
	Source    values.FactAuthorityRef
	FinalCut  uint64
	Digest    [32]byte
	Manifest  []manifestEntryData
	Transfer  []transferEntry
}

// retiredNamespace is the permanent final disposition of one element's
// content in this Facet: the retired acceptance key, the disposition cut and
// the exact ordered manifest at that cut. It proves the closed namespace,
// durably blocks any future acceptance of the element here, and is never
// garbage-collected.
type retiredNamespace struct {
	Key      keyData
	Cut      uint64
	Digest   [32]byte
	Manifest []manifestEntryData
}

type facetState struct {
	PublicKey    [32]byte
	Cut          uint64
	RealmContact *realmContactState `json:",omitempty"`
	Acceptances  map[string]accepted
	Contacts     map[string]contact
	Records      map[string]record
	Tombstones   map[string]revisionData
	Fenced       map[string]fencedNamespace
	Refused      map[string]refusalData
	Pending      map[string]pendingInstall
	Retired      map[string]retiredNamespace
}

func emptyState() facetState {
	return facetState{Cut: 1, Acceptances: map[string]accepted{}, Contacts: map[string]contact{}, Records: map[string]record{}, Tombstones: map[string]revisionData{},
		Fenced: map[string]fencedNamespace{}, Refused: map[string]refusalData{}, Pending: map[string]pendingInstall{}, Retired: map[string]retiredNamespace{}}
}
func (s facetState) clone() facetState {
	n := emptyState()
	n.Cut = s.Cut
	n.PublicKey = s.PublicKey
	if s.RealmContact != nil {
		contact := s.RealmContact.clone()
		n.RealmContact = &contact
	}
	for k, v := range s.Acceptances {
		n.Acceptances[k] = v
	}
	for k, v := range s.Contacts {
		n.Contacts[k] = v
	}
	for k, v := range s.Records {
		n.Records[k] = v
	}
	for k, v := range s.Tombstones {
		n.Tombstones[k] = v
	}
	for k, v := range s.Fenced {
		n.Fenced[k] = v
	}
	for k, v := range s.Refused {
		n.Refused[k] = v
	}
	for k, v := range s.Pending {
		n.Pending[k] = v
	}
	for k, v := range s.Retired {
		n.Retired[k] = v
	}
	return n
}
func recordID(element values.ElementID, typ []byte) string {
	return string(element) + ":" + hex.EncodeToString(typ)
}
func cutValue(scope [32]byte, serial uint64) values.FacetReadCut {
	var raw [40]byte
	copy(raw[:32], scope[:])
	binary.BigEndian.PutUint64(raw[32:], serial)
	c, _ := values.NewFacetReadCut(raw[:])
	return c
}

type command struct {
	PublicRequest string
	ID            byteString
	Actor         actorData
	Owner         values.FactAuthorityRef
	Kind          string
	Key           keyData
	Type          []byte
	Payload       []byte
	Absent        bool
	Expected      revisionData
	Address       values.NextHop
	ContactScope  [32]byte
	ContactKey    keyData
	ContactSerial uint64
	Target        values.FactAuthorityRef
	Operation     byteString
	Source        values.FactAuthorityRef
	FinalCut      uint64
	Manifest      []manifestEntryData
	Transfer      []transferEntry
}

func (c command) valid(owner values.FactAuthorityRef) bool {
	if !validPublicCommand(c) {
		return false
	}
	if !validOperation(contract.OperationID(c.ID)) || !c.Actor.Valid() || c.Owner != owner || !c.Key.value().Valid() || c.Key.Facet != owner {
		return false
	}
	zeroExpected := reflect.DeepEqual(c.Expected, revisionData{})
	noContact := c.ContactScope == [32]byte{} && c.ContactKey == keyData{} && c.ContactSerial == 0
	plain := len(c.Type) == 0 && c.Payload == nil && !c.Absent && zeroExpected && noContact
	switch c.Kind {
	case "accept":
		return c.Address.Valid() && len(c.Type) == 0 && c.Payload == nil && !c.Absent && zeroExpected && noContact
	case "contact":
		return c.Address.Valid() && len(c.Type) == 0 && c.Payload == nil && !c.Absent && zeroExpected && c.ContactScope != [32]byte{} && c.ContactKey.value().Valid() && c.ContactSerial > 0
	case "put", "delete":
		if len(c.Type) != 32 || c.Address != "" || !noContact {
			return false
		}
		if c.Kind == "put" {
			return len(c.Payload) <= maxPayloadBytes && ((c.Absent && zeroExpected) || (!c.Absent && token.ValidRevision(c.Expected.value())))
		}
		return !c.Absent && c.Payload == nil && token.ValidRevision(c.Expected.value())
	case "fence", "refuse", "settle":
		return plain && c.Address == "" && c.Target.Valid() && c.Target != c.Key.Facet && validOperation(contract.OperationID(c.Operation)) &&
			c.Source == "" && c.FinalCut == 0 && c.Manifest == nil && c.Transfer == nil
	case "accept-namespace":
		return plain && c.Address.Valid() && c.Source.Valid() && c.Source != c.Key.Facet && validOperation(contract.OperationID(c.Operation)) &&
			c.FinalCut > 0 && validManifestData(c.Manifest, c.Key) && validTransferData(c.Manifest, c.Transfer) && c.Target == ""
	case "install":
		return plain && c.Address == "" && c.Source.Valid() && c.Source != c.Key.Facet && validOperation(contract.OperationID(c.Operation)) &&
			c.FinalCut > 0 && c.Manifest == nil && c.Transfer == nil && c.Target == ""
	case "retire":
		return plain && c.Address == "" && c.Target == "" && c.Operation == "" &&
			c.Source == "" && c.FinalCut == 0 && c.Manifest == nil && c.Transfer == nil
	}
	return false
}

type outcome struct {
	Status        contract.MutationStatus
	Cut           uint64
	Error         string
	AcceptedAt    uint64
	Address       values.NextHop
	ContactSerial uint64
	Revision      revisionData
	Core          proof
}
type event struct {
	Input     command
	Core      proof
	Departure departureData
	Plan      transferPlan
	Outcome   outcome
}

func (o outcome) result(scope [32]byte, k keyData) contract.MutationResult {
	r := contract.MutationResult{Status: o.Status, Cut: cutValue(scope, o.Cut), Core: o.Core.value(), Revision: o.Revision.value()}
	if o.AcceptedAt > 0 {
		r.Acceptance = contract.Acceptance{Key: k.value(), AcceptedAt: cutValue(scope, o.AcceptedAt)}
	}
	if o.ContactSerial > 0 {
		r.CoreNextHop = o.Address
		r.NextHopRevision = token.NewContactRevision(scope, k.value(), o.ContactSerial)
	}
	return r
}

var rejectionErrors = map[string]error{
	"proposal": contract.ErrProposalNotCurrent, "lineage": contract.ErrSubjectAuthorityConflict, "acceptance": contract.ErrAcceptanceNotFound,
	"contact": contract.ErrContactRevisionConflict, "element": contract.ErrElementNotFound, "none": contract.ErrNamespaceNone, "detached": contract.ErrNamespaceDetached,
	"owner": contract.ErrNotCurrentOwner, "attachment": contract.ErrAttachmentChanged, "key": contract.ErrKeyNotFound, "revision": contract.ErrRevisionConflict,
	"capacity": contract.ErrCapacity, "fenced": contract.ErrUnavailable, "refused": contract.ErrOperationConflict, "manifest": contract.ErrInvalid,
	"pending": contract.ErrOperationConflict, "retired": contract.ErrNamespaceRetired,
}

func coreRejection(p proof, k keyData) string {
	switch p.State {
	case contract.CoreMissing:
		return "element"
	case contract.CoreNone:
		return "none"
	case contract.CoreDetached:
		return "detached"
	case contract.CoreActive:
		if p.Active.Facet != k.Facet {
			return "owner"
		}
		if p.Active != k {
			return "attachment"
		}
		return ""
	}
	return "unavailable"
}
func nextCut(s *facetState) error {
	if s.Cut == math.MaxUint64 {
		return contract.ErrUnavailable
	}
	s.Cut++
	return nil
}
