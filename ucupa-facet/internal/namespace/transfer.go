package namespace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"sort"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/contract"
)

// stagedImport is the in-memory target-side staging buffer of one namespace
// import. It is never durable: recovery re-stages from the controlling Core.
type stagedImport struct {
	key       keyData
	operation byteString
	source    values.FactAuthorityRef
	finalCut  uint64
	digest    [32]byte
	manifest  []manifestEntryData
	live      int
	staged    map[int][]byte
	bytes     uint64
}

func (s *stagedImport) complete() bool { return len(s.staged) == s.live }
func stagedID(k keyData) string        { return string(k.Core) + ":" + string(k.Element) }

func revisionBytes(r revisionData) []byte {
	out := make([]byte, 0, 32+2+len(r.Element)+2+len(r.Type)+8)
	out = append(out, r.Scope[:]...)
	var width [2]byte
	binary.BigEndian.PutUint16(width[:], uint16(len(r.Element)))
	out = append(out, width[:]...)
	out = append(out, r.Element...)
	binary.BigEndian.PutUint16(width[:], uint16(len(r.Type)))
	out = append(out, width[:]...)
	out = append(out, r.Type...)
	var serial [8]byte
	binary.BigEndian.PutUint64(serial[:], r.Serial)
	return append(out, serial[:]...)
}

// manifestDigest covers every entry kind, type, length, payload digest and
// both revisions, so it proves the exact ordered namespace without bytes.
func manifestDigest(entries []manifestEntryData) [32]byte {
	h := sha256.New()
	var width [8]byte
	for _, e := range entries {
		if e.Live {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
		h.Write(e.Type)
		binary.BigEndian.PutUint64(width[:], e.Length)
		h.Write(width[:])
		h.Write(e.Digest[:])
		h.Write(revisionBytes(e.Revision))
		h.Write(revisionBytes(e.Deleted))
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

func buildManifest(s *facetState, element values.ElementID) []manifestEntryData {
	entries := []manifestEntryData{}
	for _, r := range s.Records {
		if r.Element != element {
			continue
		}
		entries = append(entries, manifestEntryData{
			Type: r.Type, Live: true, Length: uint64(len(r.Payload)),
			Digest: sha256.Sum256(r.Payload), Revision: r.Revision,
		})
	}
	for _, t := range s.Tombstones {
		if t.Element != element {
			continue
		}
		entries = append(entries, manifestEntryData{Type: t.Type, Live: false, Deleted: t})
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].Type, entries[j].Type) < 0 })
	return entries
}

// validManifestData accepts only one strictly ordered, bounded manifest whose
// every entry revision belongs to the bound element and type.
func validManifestData(entries []manifestEntryData, k keyData) bool {
	if len(entries) > maxManifestEntries {
		return false
	}
	var prev []byte
	for _, e := range entries {
		if len(e.Type) != 32 {
			return false
		}
		if prev != nil && bytes.Compare(prev, e.Type) >= 0 {
			return false
		}
		prev = e.Type
		if e.Live {
			if e.Length > maxPayloadBytes || reflect.DeepEqual(e.Revision, revisionData{}) || !reflect.DeepEqual(e.Deleted, revisionData{}) {
				return false
			}
			if e.Revision.Element != k.Element || !bytes.Equal(e.Revision.Type, e.Type) {
				return false
			}
		} else {
			if e.Length != 0 || e.Digest != [32]byte{} || !reflect.DeepEqual(e.Revision, revisionData{}) || reflect.DeepEqual(e.Deleted, revisionData{}) {
				return false
			}
			if e.Deleted.Element != k.Element || !bytes.Equal(e.Deleted.Type, e.Type) {
				return false
			}
		}
	}
	return true
}

// validTransferData matches the exact live payload of every manifest entry in
// manifest order; nothing else travels.
func validTransferData(manifest []manifestEntryData, transfer []transferEntry) bool {
	live := 0
	for _, e := range manifest {
		if e.Live {
			live++
		}
	}
	if len(transfer) != live {
		return false
	}
	i := 0
	for _, e := range manifest {
		if !e.Live {
			continue
		}
		t := transfer[i]
		i++
		if !bytes.Equal(t.Type, e.Type) || uint64(len(t.Payload)) != e.Length || sha256.Sum256(t.Payload) != e.Digest {
			return false
		}
	}
	return true
}

// liveEntries projects one ordered manifest's live keys for the controlling
// Core's relay loop; payload bytes never travel in this projection.
func liveEntries(entries []manifestEntryData) []contract.NamespaceLiveEntry {
	live := make([]contract.NamespaceLiveEntry, 0, len(entries))
	for _, e := range entries {
		if !e.Live {
			continue
		}
		typ, _ := values.NewFacetTypeRef(e.Type)
		live = append(live, contract.NamespaceLiveEntry{Type: typ, Length: e.Length})
	}
	return live
}

func manifestEntryValue(e manifestEntryData) contract.NamespaceManifestEntry {
	typ, _ := values.NewFacetTypeRef(e.Type)
	return contract.NamespaceManifestEntry{
		Type: typ, Live: e.Live, Length: e.Length, Digest: e.Digest,
		Revision: e.Revision.value(), Deleted: e.Deleted.value(),
	}
}

func manifestValue(f fencedNamespace, key values.FacetAcceptanceKey) contract.NamespaceManifest {
	m := contract.NamespaceManifest{
		Element:  key.Element,
		Source:   key.Facet,
		FinalCut: f.Cut,
		Entries:  make([]contract.NamespaceManifestEntry, len(f.Manifest)),
		Digest:   f.Digest,
	}
	for i, e := range f.Manifest {
		m.Entries[i] = manifestEntryValue(e)
	}
	return m
}

func unpackManifest(manifest contract.NamespaceManifest) ([]manifestEntryData, bool) {
	if manifest.Element == "" || !manifest.Source.Valid() || manifest.FinalCut == 0 {
		return nil, false
	}
	entries := make([]manifestEntryData, len(manifest.Entries))
	for i, e := range manifest.Entries {
		entries[i] = manifestEntryData{
			Type: e.Type.Bytes(), Live: e.Live, Length: e.Length, Digest: e.Digest,
			Revision: packRevision(e.Revision), Deleted: packRevision(e.Deleted),
		}
	}
	return entries, true
}

// stepID derives the journal command ID of one transfer step from the transfer
// operation, making retries of the same step deduplicate across processes.
func stepID(op contract.OperationID, kind string) byteString {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(op))
	return byteString(hex.EncodeToString(h.Sum(nil)))
}

func (f *Owner) transferBase(actor values.PrincipalRef, key values.FacetAcceptanceKey, op contract.OperationID, kind string) command {
	c := f.base(actor, key, contract.OperationID(stepID(op, kind)), kind)
	c.Operation = byteString(op)
	return c
}

func (f *Owner) readPlan(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op contract.OperationID) (transferPlan, error) {
	source, ok := f.cores.(contract.ReplacementSource)
	if !ok {
		return transferPlan{}, contract.ErrUnavailable
	}
	read, err := source.ReadReplacement(ctx, address, key.Core, key.Element, op)
	if err != nil || ctx.Err() != nil {
		return transferPlan{}, contract.ErrUnavailable
	}
	return packPlan(read), nil
}

// decideTransfer is the deterministic owner transition of the five namespace
// replacement steps. Recorded plan and Core evidence are input, never
// callbacks. External evidence that does not satisfy a step premise fails
// temporarily so orchestration can retry; local races settle durably.
func decideTransfer(s *facetState, scope [32]byte, c command, p proof, d departureData, plan transferPlan) (outcome, error) {
	out := outcome{Status: contract.Rejected, Cut: s.Cut}
	reject := func(code string) (outcome, error) { out.Error = code; return out, nil }
	_, hasKey := s.Acceptances[c.Key.id()]
	id := c.Key.id()
	switch c.Kind {
	case "fence":
		if !hasKey {
			return reject("acceptance")
		}
		if _, refused := s.Refused[string(c.Operation)]; refused {
			return reject("refused")
		}
		if _, fenced := s.Fenced[string(c.Key.Element)]; fenced {
			return reject("fenced")
		}
		if !plan.present() {
			return outcome{}, contract.ErrUnavailable
		}
		if plan.Authority != c.Key.Core || plan.Element != c.Key.Element || plan.Operation != c.Operation ||
			plan.Source != c.Key.Facet || plan.Target != c.Target || plan.FinalCut != 0 || plan.ManifestDigest != [32]byte{} {
			return outcome{}, contract.ErrUnavailable
		}
		if !plan.Permitted || plan.Fenced || plan.Accepted || plan.Committed || plan.Settled || plan.Cancelled {
			return outcome{}, contract.ErrUnavailable
		}
		manifest := buildManifest(s, c.Key.Element)
		if len(manifest) > maxManifestEntries {
			return reject("capacity")
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		s.Fenced[string(c.Key.Element)] = fencedNamespace{
			Core: c.Key.Core, Operation: c.Operation, Target: c.Target,
			Cut: s.Cut, Digest: manifestDigest(manifest), Manifest: manifest, SourceKey: c.Key,
		}
		out.Status = contract.Committed
		out.Cut = s.Cut
		return out, nil
	case "refuse":
		if !hasKey {
			return reject("acceptance")
		}
		if _, fenced := s.Fenced[string(c.Key.Element)]; fenced {
			return reject("fenced")
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		s.Refused[string(c.Operation)] = refusalData{Core: c.Key.Core, Element: c.Key.Element, Target: c.Target}
		out.Status = contract.Committed
		out.Cut = s.Cut
		return out, nil
	case "settle":
		if !hasKey {
			return reject("acceptance")
		}
		fenced, fencedFound := s.Fenced[string(c.Key.Element)]
		if !fencedFound || fenced.Operation != c.Operation {
			return reject("fenced")
		}
		if !plan.present() {
			return outcome{}, contract.ErrUnavailable
		}
		if plan.Authority != c.Key.Core || plan.Element != c.Key.Element || plan.Operation != c.Operation ||
			plan.Source != c.Key.Facet || plan.Target != c.Target ||
			plan.FinalCut != fenced.Cut || plan.ManifestDigest != fenced.Digest {
			return outcome{}, contract.ErrUnavailable
		}
		if !plan.Permitted || !plan.Fenced || !plan.Accepted || !plan.Committed || plan.Settled || plan.Cancelled {
			return outcome{}, contract.ErrUnavailable
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		fenced.Settled = true
		s.Fenced[string(c.Key.Element)] = fenced
		out.Status = contract.Committed
		out.Cut = s.Cut
		return out, nil
	case "accept-namespace":
		if _, retired := s.Retired[string(c.Key.Element)]; retired {
			return reject("retired")
		}
		for _, a := range s.Acceptances {
			if a.Key.Element == c.Key.Element && a.Key.Core != c.Key.Core {
				return reject("lineage")
			}
		}
		for _, pending := range s.Pending {
			if pending.Element == c.Key.Element {
				return reject("pending")
			}
		}
		for _, r := range s.Records {
			if r.Element == c.Key.Element {
				return reject("manifest")
			}
		}
		if !plan.present() {
			return outcome{}, contract.ErrUnavailable
		}
		digest := manifestDigest(c.Manifest)
		if plan.Authority != c.Key.Core || plan.Element != c.Key.Element || plan.Operation != c.Operation ||
			plan.Source != c.Source || plan.Target != c.Key.Facet || plan.Proposal != c.Key.Proposal ||
			plan.FinalCut != c.FinalCut || plan.ManifestDigest != digest {
			return outcome{}, contract.ErrUnavailable
		}
		if !plan.Permitted || !plan.Fenced || plan.Accepted || plan.Committed || plan.Settled || plan.Cancelled {
			return outcome{}, contract.ErrUnavailable
		}
		if len(s.Pending) >= maxPendingInstalls {
			return reject("capacity")
		}
		var pendingBytes uint64
		for _, pending := range s.Pending {
			for _, t := range pending.Transfer {
				pendingBytes += uint64(len(t.Payload))
			}
		}
		for _, t := range c.Transfer {
			pendingBytes += uint64(len(t.Payload))
		}
		if pendingBytes > maxPendingBytes {
			return reject("capacity")
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		// The namespace acceptance establishes the target relation itself in
		// the same transaction: a replacement proposal never passes the normal
		// CoreNone acceptance path while the element still holds its old owner.
		if _, exists := s.Acceptances[id]; !exists {
			s.Acceptances[id] = accepted{c.Key, s.Cut}
			s.Contacts[id] = contact{c.Address, s.Cut}
		}
		transfer := make([]transferEntry, len(c.Transfer))
		for i, t := range c.Transfer {
			transfer[i] = transferEntry{Type: append([]byte(nil), t.Type...), Payload: append([]byte(nil), t.Payload...)}
		}
		s.Pending[id] = pendingInstall{
			Core: c.Key.Core, Element: c.Key.Element, Operation: c.Operation, Source: c.Source,
			FinalCut: c.FinalCut, Digest: digest, Manifest: c.Manifest, Transfer: transfer,
		}
		out.Status = contract.Committed
		out.Cut = s.Cut
		out.AcceptedAt = s.Cut
		out.Address = c.Address
		out.ContactSerial = s.Cut
		return out, nil
	case "install":
		if !hasKey {
			return reject("acceptance")
		}
		pending, found := s.Pending[id]
		if !found || pending.Operation != c.Operation || pending.Source != c.Source || pending.FinalCut != c.FinalCut {
			return outcome{}, contract.ErrUnavailable
		}
		if !p.valid(c.Key, false) {
			return outcome{}, contract.ErrUnavailable
		}
		if code := coreRejection(p, c.Key); code != "" {
			return outcome{}, contract.ErrUnavailable
		}
		for _, r := range s.Records {
			if r.Element == c.Key.Element {
				return reject("manifest")
			}
		}
		if err := nextCut(s); err != nil {
			return outcome{}, err
		}
		// Installed content and types are exact, but revisions are reissued by
		// this owner: source tokens never become writable CAS credentials.
		for _, t := range pending.Transfer {
			rev := revisionData{scope, c.Key.Element, append([]byte(nil), t.Type...), s.Cut}
			s.Records[recordID(c.Key.Element, t.Type)] = record{c.Key.Element, append([]byte(nil), t.Type...), append([]byte(nil), t.Payload...), rev}
		}
		for _, m := range pending.Manifest {
			if !m.Live {
				s.Tombstones[recordID(c.Key.Element, m.Type)] = revisionData{scope, c.Key.Element, append([]byte(nil), m.Type...), s.Cut}
			}
		}
		delete(s.Pending, id)
		out.Status = contract.Committed
		out.Cut = s.Cut
		return out, nil
	}
	return outcome{}, contract.ErrInvalid
}
