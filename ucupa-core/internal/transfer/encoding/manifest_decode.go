package encoding

import (
	"crypto/sha256"
	"encoding/binary"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// decodeTransferManifest verifies the bounded exact bytes against independently
// acquired source evidence, then reuses the original Element codecs, graph
// validation and closure selection. It constructs no active target facts.
func DecodeTransferManifest(f transferfact.SourceFence, raw string) (transferfact.Slice, error) {
	if !f.Input.Valid() {
		return transferfact.Slice{}, contract.ErrInvalid
	}
	input, err := EncodeTransferIntent(f.Input)
	if err != nil {
		return transferfact.Slice{}, err
	}
	return decodeManifest(manifestBinding{Source: f.Input.Source, Boundary: f.Input.Boundary, Cut: f.FinalCut,
		Digest: f.ManifestDigest, Input: input, Profile: f.Input.Profile, Expected: f.Input.Expected}, raw)
}

func DecodeCarrierManifest(f transferfact.CarrierFence, raw string) (transferfact.Slice, error) {
	if !f.Valid() {
		return transferfact.Slice{}, contract.ErrInvalid
	}
	input, err := EncodeCarrierPermit(f.Permit)
	if err != nil {
		return transferfact.Slice{}, err
	}
	r := f.Permit.Reservation
	return decodeManifest(manifestBinding{Source: r.Input.Source, Boundary: r.Input.Mount, Cut: f.FinalCut, Digest: f.ManifestDigest,
		Input: input, Profile: transferfact.CarrierReplacement,
		External: elementfact.ExternalParent{ID: r.Input.Mount, Range: r.Tuple.Range, Present: true, Admitted: true}}, raw)
}

// This short-lived codec context holds no live facts, credentials or state.
type manifestBinding struct {
	Source   values.FactAuthorityRef
	Boundary values.ElementID
	Cut      contract.CoreReadCut
	Digest   [32]byte
	Input    []byte
	Profile  transferfact.Profile
	Expected contract.Revision
	External elementfact.ExternalParent
}

func decodeManifest(b manifestBinding, raw string) (transferfact.Slice, error) {
	if len(raw) == 0 || !token.Valid(b.Cut) || b.Digest == ([32]byte{}) {
		return transferfact.Slice{}, contract.ErrInvalid
	}
	if len(raw) > MaxTransferManifestBytes {
		return transferfact.Slice{}, values.PublicResourceExhausted
	}
	if sha256.Sum256([]byte(raw)) != b.Digest {
		return transferfact.Slice{}, contract.ErrConflict
	}
	var m StoredTransferManifest
	if err := storebytes.DecodeCanonicalJSON([]byte(raw), &m); err != nil {
		return transferfact.Slice{}, contract.ErrInvalid
	}
	carrier := b.Profile == transferfact.CarrierReplacement
	if m.IntentDigest != sha256.Sum256(b.Input) || m.Scope != token.Scope(b.Cut) || m.FinalCut != token.Serial(b.Cut) ||
		(!carrier && (m.Scope != token.RevisionScope(b.Expected) || len(m.Members) == 0)) ||
		(b.Profile != transferfact.SliceTransfer && len(m.Ancestors) != 0) ||
		(b.Profile == transferfact.SliceTransfer && len(m.Ancestors) == 0 && m.SourceParent == nil) {
		return transferfact.Slice{}, contract.ErrConflict
	}
	var upstream transferfact.ParentHold
	if m.SourceParent != nil {
		var err error
		upstream, err = m.SourceParent.hold()
		if err != nil || b.Profile != transferfact.SliceTransfer || upstream.Input.Role != transferfact.SourceParent {
			return transferfact.Slice{}, contract.ErrConflict
		}
		input, err := EncodeTransferIntent(upstream.Input.Initial.Input)
		if err != nil || string(input) != string(b.Input) {
			return transferfact.Slice{}, contract.ErrConflict
		}
		b.External = elementfact.ExternalParent{ID: upstream.Input.Mount, Range: upstream.Tuple.Range, Present: true, Admitted: true}
	}
	if len(m.Members)+len(m.Ancestors) > MaxTransferManifestRecords {
		return transferfact.Slice{}, values.PublicResourceExhausted
	}
	// Temporary decoding state uses only the existing element loader. No Store,
	// writer key, outcome journal or live target state is copied into this view.
	elements := elementfact.New()
	facets := facetfact.NewProjectionBuilder()
	mounts := mountfact.New()
	for _, item := range m.Members {
		if item.Element.Kind != "anchor" && item.Element.Kind != "entity" {
			return transferfact.Slice{}, contract.ErrInvalid
		}
		if err := storerecord.LoadElement(&elements, m.Scope, item.ID, item.Element); err != nil {
			return transferfact.Slice{}, contract.ErrInvalid
		}
		record, err := decodeTransferFacet(item, b.Source, b.Cut)
		if err != nil {
			return transferfact.Slice{}, err
		}
		facets.Add(item.ID, record)
		edge, err := item.Mount.record()
		if err != nil {
			return transferfact.Slice{}, err
		}
		if item.Mount != nil {
			mounts.ReplaceOutgoing(item.ID, edge)
		}
	}
	for _, item := range m.Ancestors {
		if item.Element.Kind != "anchor" {
			return transferfact.Slice{}, contract.ErrInvalid
		}
		if err := storerecord.LoadElement(&elements, m.Scope, item.ID, item.Element); err != nil {
			return transferfact.Slice{}, contract.ErrInvalid
		}
	}
	if err := elements.Validate(m.Scope, m.RevisionLimit, b.External); err != nil {
		return transferfact.Slice{}, contract.ErrConflict
	}
	var revision contract.Revision
	if a, ok := elements.Anchor(b.Boundary); ok {
		revision = a.Revision
	} else if e, ok := elements.Entity(b.Boundary); ok {
		revision = e.Revision
	}
	if !carrier && revision != b.Expected {
		return transferfact.Slice{}, contract.ErrRevisionConflict
	}
	var slice transferfact.Slice
	var err error
	if carrier {
		slice, err = transferfact.SelectCarrier(b.Source, b.Cut, m.RevisionLimit, elements.View, facets.Build().View,
			mounts.View, b.External, MaxTransferManifestRecords)
	} else if upstream != (transferfact.ParentHold{}) {
		slice, err = transferfact.SelectHeld(upstream, b.Cut, elements.View, facets.Build().View, mounts.View, MaxTransferManifestRecords)
	} else {
		slice, err = transferfact.Select(b.Source, b.Cut, elements.View, facets.Build().View,
			mounts.View, b.Boundary, MaxTransferManifestRecords)
	}
	if err != nil {
		return transferfact.Slice{}, err
	}
	chains := make(map[values.ElementID][]transferfact.LineageStep)
	count := 0
	for _, item := range m.Members {
		if len(item.Lineage) > transferfact.MaxLineageSteps-count {
			return transferfact.Slice{}, values.PublicResourceExhausted
		}
		count += len(item.Lineage)
		for _, stored := range item.Lineage {
			step, err := stored.step()
			if err != nil {
				return transferfact.Slice{}, err
			}
			chains[item.ID] = append(chains[item.ID], step)
		}
	}
	slice, err = slice.WithLineage(chains)
	if err != nil {
		return transferfact.Slice{}, err
	}
	if err := validateTransferFacetOrigin(slice); err != nil {
		return transferfact.Slice{}, err
	}
	canonical, err := encodeManifest(b.Input, slice)
	if err != nil || canonical != raw {
		return transferfact.Slice{}, contract.ErrConflict
	}
	return slice, nil
}

func decodeTransferFacet(item StoredTransferMember, source values.FactAuthorityRef, cut contract.CoreReadCut) (facetfact.Record, error) {
	a, err := item.Facet.Attachment()
	if err != nil || !transferAttachmentToken(a.Revision, item.ID, cut) {
		return facetfact.Record{}, contract.ErrInvalid
	}
	c, err := item.Contact.Contact()
	if err != nil {
		return facetfact.Record{}, contract.ErrInvalid
	}
	if a.State == contract.FacetNone {
		if a.Key != (values.FacetAcceptanceKey{}) || a.OriginalProposal != (values.PublicToken{}) {
			return facetfact.Record{}, contract.ErrConflict
		}
	} else if a.State == contract.FacetActive || a.State == contract.FacetDetached || a.State == contract.FacetRetired {
		if !a.Key.Valid() || a.Key.Element != item.ID || !a.OriginalProposal.Valid() || a.OriginalProposal.Owner != a.Key.Core || a.OriginalProposal.Kind != "attachment-revision" {
			return facetfact.Record{}, contract.ErrConflict
		}
		if a.Key.Core == source {
			if !transferAttachmentToken(a.Key.Proposal, item.ID, cut) {
				return facetfact.Record{}, contract.ErrConflict
			}
		} else if a.State != contract.FacetDetached && a.State != contract.FacetRetired {
			return facetfact.Record{}, contract.ErrConflict
		}
	} else {
		return facetfact.Record{}, contract.ErrInvalid
	}
	if a.State == contract.FacetActive {
		if !item.NextHop.Valid() || a.NextHop != item.NextHop || a.NextHopRevision != c || !token.ValidFacetContactRevision(c) ||
			token.FacetContactScope(c) != token.Scope(cut) || token.FacetContactSerial(c) > token.Serial(cut) || token.FacetContactKey(c) != a.Key {
			return facetfact.Record{}, contract.ErrConflict
		}
	} else if a.NextHop != "" || item.NextHop != "" || c != (contract.FacetContactRevision{}) || a.NextHopRevision != c {
		return facetfact.Record{}, contract.ErrConflict
	}
	return facetfact.Record{Attachment: a, NextHop: item.NextHop, Contact: c}, nil
}

func transferAttachmentToken(r values.AttachmentRevision, id values.ElementID, cut contract.CoreReadCut) bool {
	data := r.Bytes()
	if len(data) != 76 {
		return false
	}
	serial := binary.BigEndian.Uint64(data[68:])
	return serial > 0 && serial <= token.Serial(cut) && r == facetfact.AttachmentRevisionForCut(id, token.FromParts(token.Scope(cut), serial))
}
