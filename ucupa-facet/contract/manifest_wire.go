package contract

import (
	"encoding/binary"

	"ucupa/shared/values"
	"ucupa/ucupa-facet/internal/token"
)

// MaxNamespaceManifestEntries bounds one complete ordered namespace manifest.
// It is shared by the owner decision path and the wire encoder below.
const MaxNamespaceManifestEntries = 1024

// The manifest wire format is the exact byte form one fenced namespace
// retains in the controlling Core's journal: fixed-order, length-prefixed and
// free of any content payload. It is deterministic so a journal read-back can
// be reused verbatim for every downstream retry; it carries the opaque Facet
// revisions as plain owner-scoped parts, never as a writable credential.
//
//	layout:
//	  u16 element length, element
//	  u16 source length, source
//	  u64 final cut
//	  u16 entry count (0..MaxNamespaceManifestEntries)
//	  entries, strictly type-ascending, each:
//	    u8 live flag, 32-byte type, u64 length, 32-byte digest,
//	    revision (scope 32B, u16+element, u16+type, u64 serial),
//	    deleted revision (same layout)

func putRevision(out []byte, v Revision) []byte {
	scope, element, typ, serial := token.RevisionParts(v)
	out = append(out, scope[:]...)
	var width [2]byte
	binary.BigEndian.PutUint16(width[:], uint16(len(element)))
	out = append(out, width[:]...)
	out = append(out, element...)
	binary.BigEndian.PutUint16(width[:], uint16(len(typ)))
	out = append(out, width[:]...)
	out = append(out, typ...)
	var serialBytes [8]byte
	binary.BigEndian.PutUint64(serialBytes[:], serial)
	return append(out, serialBytes[:]...)
}

func revisionFrom(in []byte, at int) (Revision, int, bool) {
	if at+32+2+8 > len(in) {
		return Revision{}, at, false
	}
	var scope [32]byte
	copy(scope[:], in[at:at+32])
	at += 32
	elementLen := int(binary.BigEndian.Uint16(in[at : at+2]))
	at += 2
	if at+elementLen+2+8 > len(in) {
		return Revision{}, at, false
	}
	element := values.ElementID(in[at : at+elementLen])
	at += elementLen
	typeLen := int(binary.BigEndian.Uint16(in[at : at+2]))
	at += 2
	if at+typeLen+8 > len(in) {
		return Revision{}, at, false
	}
	typ := string(in[at : at+typeLen])
	at += typeLen
	serial := binary.BigEndian.Uint64(in[at : at+8])
	at += 8
	return token.NewRevision(scope, element, typ, serial), at, true
}

func revisionZero(v Revision) bool {
	scope, element, typ, serial := token.RevisionParts(v)
	return scope == [32]byte{} && element == "" && typ == "" && serial == 0
}

// EncodeNamespaceManifest returns the exact journal bytes of one complete
// manifest. Entries must be strictly type-ascending and bounded; the caller
// cannot smuggle payloads because only digests travel here.
func EncodeNamespaceManifest(m NamespaceManifest) ([]byte, error) {
	if m.Element == "" || !m.Source.Valid() || m.FinalCut == 0 || len(m.Entries) > MaxNamespaceManifestEntries {
		return nil, ErrInvalid
	}
	out := make([]byte, 0, 64+len(m.Entries)*140)
	var width [2]byte
	binary.BigEndian.PutUint16(width[:], uint16(len(m.Element)))
	out = append(out, width[:]...)
	out = append(out, m.Element...)
	binary.BigEndian.PutUint16(width[:], uint16(len(m.Source)))
	out = append(out, width[:]...)
	out = append(out, m.Source...)
	var finalCut [8]byte
	binary.BigEndian.PutUint64(finalCut[:], m.FinalCut)
	out = append(out, finalCut[:]...)
	binary.BigEndian.PutUint16(width[:], uint16(len(m.Entries)))
	out = append(out, width[:]...)
	var prev []byte
	for _, e := range m.Entries {
		if !e.Type.Valid() {
			return nil, ErrInvalid
		}
		typ := e.Type.Bytes()
		if prev != nil && string(prev) >= string(typ) {
			return nil, ErrInvalid
		}
		prev = typ
		if e.Live && !revisionZero(e.Deleted) {
			return nil, ErrInvalid
		}
		if !e.Live && (e.Length != 0 || e.Digest != [32]byte{} || !revisionZero(e.Revision)) {
			return nil, ErrInvalid
		}
		if e.Live {
			out = append(out, 1)
		} else {
			out = append(out, 0)
		}
		out = append(out, typ...)
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], e.Length)
		out = append(out, length[:]...)
		out = append(out, e.Digest[:]...)
		out = putRevision(out, e.Revision)
		out = putRevision(out, e.Deleted)
	}
	return out, nil
}

// DecodeNamespaceManifest is the exact inverse of EncodeNamespaceManifest for
// bytes previously retained by or obtained from the controlling Core journal.
// It enforces only the wire bounds and ordering; owner-side semantic checks
// remain with the namespace decision path.
func DecodeNamespaceManifest(raw []byte) (NamespaceManifest, error) {
	if len(raw) < 2+2+8+2 {
		return NamespaceManifest{}, ErrInvalid
	}
	at := 0
	readLen := func() (int, int, bool) {
		if at+2 > len(raw) {
			return 0, at, false
		}
		l := int(binary.BigEndian.Uint16(raw[at : at+2]))
		at += 2
		if at+l > len(raw) {
			return 0, at, false
		}
		return l, at, true
	}
	elementLen, pos, ok := readLen()
	if !ok {
		return NamespaceManifest{}, ErrInvalid
	}
	m := NamespaceManifest{Element: values.ElementID(raw[pos : pos+elementLen])}
	at = pos + elementLen
	sourceLen, pos, ok := readLen()
	if !ok {
		return NamespaceManifest{}, ErrInvalid
	}
	m.Source = values.FactAuthorityRef(raw[pos : pos+sourceLen])
	at = pos + sourceLen
	if !m.Source.Valid() {
		return NamespaceManifest{}, ErrInvalid
	}
	if at+8 > len(raw) {
		return NamespaceManifest{}, ErrInvalid
	}
	m.FinalCut = binary.BigEndian.Uint64(raw[at : at+8])
	at += 8
	if m.FinalCut == 0 {
		return NamespaceManifest{}, ErrInvalid
	}
	if at+2 > len(raw) {
		return NamespaceManifest{}, ErrInvalid
	}
	count := int(binary.BigEndian.Uint16(raw[at : at+2]))
	at += 2
	if count > MaxNamespaceManifestEntries {
		return NamespaceManifest{}, ErrInvalid
	}
	m.Entries = make([]NamespaceManifestEntry, 0, count)
	var prev []byte
	for i := 0; i < count; i++ {
		if at+1+32+8+32 > len(raw) {
			return NamespaceManifest{}, ErrInvalid
		}
		var e NamespaceManifestEntry
		e.Live = raw[at] == 1
		at++
		if !e.Live && raw[at-1] != 0 {
			return NamespaceManifest{}, ErrInvalid
		}
		typ, err := values.NewFacetTypeRef(raw[at : at+32])
		if err != nil {
			return NamespaceManifest{}, ErrInvalid
		}
		at += 32
		if prev != nil && string(prev) >= string(typ.Bytes()) {
			return NamespaceManifest{}, ErrInvalid
		}
		prev = typ.Bytes()
		e.Type = typ
		e.Length = binary.BigEndian.Uint64(raw[at : at+8])
		at += 8
		copy(e.Digest[:], raw[at:at+32])
		at += 32
		e.Revision, at, ok = revisionFrom(raw, at)
		if !ok {
			return NamespaceManifest{}, ErrInvalid
		}
		e.Deleted, at, ok = revisionFrom(raw, at)
		if !ok {
			return NamespaceManifest{}, ErrInvalid
		}
		m.Entries = append(m.Entries, e)
	}
	if at != len(raw) {
		return NamespaceManifest{}, ErrInvalid
	}
	return m, nil
}
