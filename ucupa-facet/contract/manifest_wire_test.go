package contract

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"ucupa/shared/values"
)

func TestNamespaceManifestWireAcceptsCompleteEmptyNamespace(t *testing.T) {
	source, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	manifest := NamespaceManifest{
		Element:  values.ElementID("00000000-0000-4000-8000-00000000fa73"),
		Source:   source,
		FinalCut: 7,
		Entries:  []NamespaceManifestEntry{},
	}
	raw, err := EncodeNamespaceManifest(manifest)
	if err != nil {
		t.Fatalf("encode empty namespace: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty namespace encoded to an absent manifest")
	}
	decoded, err := DecodeNamespaceManifest(raw)
	if err != nil {
		t.Fatalf("decode empty namespace: %v", err)
	}
	if decoded.Element != manifest.Element || decoded.Source != manifest.Source || decoded.FinalCut != manifest.FinalCut || len(decoded.Entries) != 0 {
		t.Fatalf("decoded empty namespace = %#v", decoded)
	}
	if reencoded, err := EncodeNamespaceManifest(decoded); err != nil || !bytes.Equal(reencoded, raw) {
		t.Fatalf("empty namespace round trip = %x, err=%v; want %x", reencoded, err, raw)
	}

	manifest.Entries = nil
	nilEntries, err := EncodeNamespaceManifest(manifest)
	if err != nil || !bytes.Equal(nilEntries, raw) {
		t.Fatalf("nil-entry empty namespace = %x, err=%v; want %x", nilEntries, err, raw)
	}
}

func TestNamespaceManifestWireRejectsInvalidEmptyNamespaceEnvelope(t *testing.T) {
	source, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	manifest := NamespaceManifest{
		Element:  values.ElementID("00000000-0000-4000-8000-00000000fa74"),
		Source:   source,
		FinalCut: 1,
	}
	raw, err := EncodeNamespaceManifest(manifest)
	if err != nil {
		t.Fatalf("encode valid empty namespace: %v", err)
	}

	countOffset := 2 + len(manifest.Element) + 2 + len(manifest.Source) + 8
	tooMany := append([]byte(nil), raw...)
	binary.BigEndian.PutUint16(tooMany[countOffset:countOffset+2], MaxNamespaceManifestEntries+1)
	trailing := append(append([]byte(nil), raw...), 0)
	badFinalCut := append([]byte(nil), raw...)
	finalCutOffset := countOffset - 8
	binary.BigEndian.PutUint64(badFinalCut[finalCutOffset:countOffset], 0)

	for name, invalid := range map[string][]byte{
		"truncated header":       raw[:len(raw)-1],
		"entry count over limit": tooMany,
		"trailing bytes":         trailing,
		"zero final cut":         badFinalCut,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeNamespaceManifest(invalid); !errors.Is(err, ErrInvalid) {
				t.Fatalf("DecodeNamespaceManifest(%x) = %v, want ErrInvalid", invalid, err)
			}
		})
	}

	for name, invalid := range map[string]NamespaceManifest{
		"missing element": {Source: source, FinalCut: 1},
		"missing source":  {Element: manifest.Element, FinalCut: 1},
		"zero final cut":  {Element: manifest.Element, Source: source},
		"too many entries": {
			Element: manifest.Element, Source: source, FinalCut: 1,
			Entries: make([]NamespaceManifestEntry, MaxNamespaceManifestEntries+1),
		},
	} {
		t.Run("encode "+name, func(t *testing.T) {
			if _, err := EncodeNamespaceManifest(invalid); !errors.Is(err, ErrInvalid) {
				t.Fatalf("EncodeNamespaceManifest(%#v) = %v, want ErrInvalid", invalid, err)
			}
		})
	}
}

func TestNamespaceManifestWireRejectsEveryTruncatedPrefix(t *testing.T) {
	source, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeNamespaceManifest(NamespaceManifest{
		Element:  values.ElementID("00000000-0000-4000-8000-00000000fa75"),
		Source:   source,
		FinalCut: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for length := 0; length < len(raw); length++ {
		// Copy so the decoder cannot read a truncated byte through spare capacity.
		prefix := append([]byte(nil), raw[:length]...)
		if _, err := DecodeNamespaceManifest(prefix); !errors.Is(err, ErrInvalid) {
			t.Fatalf("truncated manifest of %d/%d bytes: %v, want ErrInvalid", length, len(raw), err)
		}
	}
}
