package encoding

import (
	"crypto/sha256"
	"encoding/json"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

const MaxTransferManifestRecords = 4096

const MaxTransferManifestBytes = 1 << 20

const MaxTransferRetainedBytes = 8 << 20

// A manifest is a bounded, immutable source observation using the existing
// persistence codecs. It carries no writer key, permission grant or receipts.
// Its digest binds bytes, not authority; remote authentication remains required.
type StoredTransferManifest struct {
	SourceParent  *StoredTransferParentHold `json:"sourceParent,omitempty"`
	IntentDigest  [32]byte                  `json:"intentDigest"`
	Scope         [32]byte                  `json:"scope"`
	FinalCut      uint64                    `json:"finalCut"`
	RevisionLimit uint64                    `json:"revisionLimit"` // highest revision in this exact finite manifest
	Members       []StoredTransferMember    `json:"members"`
	Ancestors     []StoredTransferAncestor  `json:"ancestors"`
}

type StoredTransferMember struct {
	Mount   *storedTransferMount              `json:"mount,omitempty"`
	ID      values.ElementID                  `json:"id"`
	Element storerecord.StoredElement         `json:"element"`
	Facet   storerecord.StoredFacetAttachment `json:"facet"`
	NextHop values.NextHop                    `json:"nextHop"`
	Contact storerecord.StoredFacetContact    `json:"contact"`
	Lineage []StoredLineageStep               `json:"lineage,omitempty"`
}

type StoredTransferAncestor struct {
	ID      values.ElementID          `json:"id"`
	Element storerecord.StoredElement `json:"element"`
}

func EncodeSourceManifest(input transferfact.Intent, slice transferfact.Slice) (string, error) {
	encodedInput, err := EncodeTransferIntent(input)
	if err != nil {
		return "", err
	}
	if slice.Source() != input.Source || slice.Boundary() != input.Boundary || slice.Profile() != input.Profile {
		return "", contract.ErrConflict
	}
	if slice.ExternalParent().Present && (slice.SourceParent() == (transferfact.ParentHold{}) || slice.SourceParent().Input.Initial.Input != input) {
		return "", contract.ErrMountCoordinationRequired
	}
	return encodeManifest(encodedInput, slice)
}

func EncodeCarrierManifest(permit transferfact.CarrierControl, slice transferfact.Slice) (string, error) {
	input, err := EncodeCarrierPermit(permit)
	if err != nil {
		return "", err
	}
	i := permit.Reservation.Input
	if slice.Source() != i.Source || slice.Boundary() != i.Mount || slice.Profile() != transferfact.CarrierReplacement {
		return "", contract.ErrConflict
	}
	return encodeManifest(input, slice)
}

// One canonical member/ancestor/lineage encoding for all content profiles.
func encodeManifest(encodedInput []byte, slice transferfact.Slice) (string, error) {
	members, ancestors := slice.Members(), slice.Ancestors()
	if len(members) == 0 && slice.Profile() != transferfact.CarrierReplacement || len(members)+len(ancestors) > MaxTransferManifestRecords {
		return "", values.PublicResourceExhausted
	}
	m := StoredTransferManifest{IntentDigest: sha256.Sum256(encodedInput), Scope: token.Scope(slice.Cut()), FinalCut: token.Serial(slice.Cut()),
		Members: make([]StoredTransferMember, 0, len(members)), Ancestors: make([]StoredTransferAncestor, 0, len(ancestors))}
	if slice.SourceParent() != (transferfact.ParentHold{}) {
		v := storeTransferParentHold(slice.SourceParent())
		m.SourceParent = &v
	}
	for _, member := range members {
		if member.Facet.Pending != (values.FacetAcceptanceKey{}) {
			return "", contract.ErrAttachmentStateConflict
		}
		record := StoredTransferMember{ID: member.ID(), Facet: storerecord.StoreFacetAttachment(member.Facet.Attachment), NextHop: member.Facet.NextHop, Contact: storerecord.StoreFacetContact(member.Facet.Contact)}
		record.Mount = storeTransferMount(member.Mount)
		switch member.Kind {
		case contract.ElementAnchor:
			record.Element = storerecord.StoreAnchor(member.Anchor)
		case contract.ElementEntity:
			record.Element = storerecord.StoreEntity(member.Entity)
		default:
			return "", contract.ErrStore
		}
		m.Members = append(m.Members, record)
		for _, step := range slice.Lineage(member.ID()) {
			m.Members[len(m.Members)-1].Lineage = append(m.Members[len(m.Members)-1].Lineage, storeLineageStep(step))
		}
		m.RevisionLimit = max(m.RevisionLimit, record.Element.RevisionSerial)
	}
	for _, ancestor := range ancestors {
		m.Ancestors = append(m.Ancestors, StoredTransferAncestor{ID: ancestor.ID, Element: storerecord.StoreAnchor(ancestor)})
		m.RevisionLimit = max(m.RevisionLimit, token.RevisionSerial(ancestor.Revision))
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	if len(data) > MaxTransferManifestBytes {
		return "", values.PublicResourceExhausted
	}
	return string(data), nil
}
