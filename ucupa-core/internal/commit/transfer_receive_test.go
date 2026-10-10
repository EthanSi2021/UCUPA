package commit

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storerecord "ucupa/ucupa-core/internal/store/record"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func frozenSource(t *testing.T, o *Owner, input transferfact.Intent) (transferfact.SourceFence, string) {
	t.Helper()
	r, err := beginTransfer(t, o, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(o, input, r.Revision); err != nil {
		t.Fatal(err)
	}
	o.RLock()
	f, raw, err := o.TransferSourceFenceLocked(input.Actor, input.Operation)
	o.RUnlock()
	if err != nil {
		t.Fatal(err)
	}
	return f, raw
}

func TestTransferManifestDecodeUsesExactCanonicalSourceEvidence(t *testing.T) {
	o, _, input := transferIntentFixture(t)
	want, err := transferSource(t, o, input.Boundary, input.Expected, 100)
	if err != nil {
		t.Fatal(err)
	}
	f, raw := frozenSource(t, o, input)
	got, err := transferencoding.DecodeTransferManifest(f, raw)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("decode: %+v %v", got, err)
	}
	for _, change := range []func(*transferfact.SourceFence){
		func(f *transferfact.SourceFence) { f.Input.Operation = "other" },
		func(f *transferfact.SourceFence) { f.Input.Actor.Principal = "other" },
		func(f *transferfact.SourceFence) { f.FinalCut = contract.CoreReadCut{} },
		func(f *transferfact.SourceFence) { f.ManifestDigest = [32]byte{1} },
	} {
		bad := f
		change(&bad)
		if _, err := transferencoding.DecodeTransferManifest(bad, raw); err == nil {
			t.Fatal("unbound evidence accepted")
		}
	}
	for _, bad := range []string{raw[:len(raw)-1], raw + " ", "{}", strings.Repeat("x", transferencoding.MaxTransferManifestBytes+1)} {
		if _, err := transferencoding.DecodeTransferManifest(f, bad); err == nil {
			t.Fatal("changed bytes accepted against original fence")
		}
	}
	// These cases update a synthetic digest to independently exercise the
	// structural validator. Real evidence still requires authenticated source
	// acquisition; a client may not authorize its own replacement digest.
	for name, change := range map[string]func(*transferencoding.StoredTransferManifest){
		"duplicate":    func(m *transferencoding.StoredTransferManifest) { m.Members = append(m.Members, m.Members[0]) },
		"missing-root": func(m *transferencoding.StoredTransferManifest) { m.Ancestors = nil },
		"wrong-order": func(m *transferencoding.StoredTransferManifest) {
			m.Members[0], m.Members[1] = m.Members[1], m.Members[0]
		},
		"dangling": func(m *transferencoding.StoredTransferManifest) {
			m.Members[1].Element.Parent = string(foundationStoreID(4321))
		},
		"deleted": func(m *transferencoding.StoredTransferManifest) {
			m.Members[1].Element = storerecord.StoredElement{Kind: "tombstone", RevisionSerial: 3}
		},
		"duplicate-revision": func(m *transferencoding.StoredTransferManifest) {
			m.Members[1].Element.RevisionSerial = m.Members[0].Element.RevisionSerial
		},
		"future-revision":        func(m *transferencoding.StoredTransferManifest) { m.Members[1].Element.RevisionSerial = m.FinalCut },
		"facet-state":            func(m *transferencoding.StoredTransferManifest) { m.Members[0].Facet.State = 99 },
		"facet-contact":          func(m *transferencoding.StoredTransferManifest) { m.Members[0].NextHop = "local:other" },
		"facet-missing-revision": func(m *transferencoding.StoredTransferManifest) { m.Members[0].Facet.Revision = nil },
	} {
		t.Run(name, func(t *testing.T) {
			var m transferencoding.StoredTransferManifest
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				t.Fatal(err)
			}
			change(&m)
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			bad := f
			bad.ManifestDigest = sha256.Sum256(data)
			if _, err := transferencoding.DecodeTransferManifest(bad, string(data)); err == nil {
				t.Fatal("malformed closure accepted")
			}
		})
	}
}

func TestTransferSourceEvidenceDoesNotTreatIntentAsFence(t *testing.T) {
	o, path, input := transferIntentFixture(t)
	if _, err := beginTransfer(t, o, input); err != nil {
		t.Fatal(err)
	}
	o.RLock()
	_, _, err := o.TransferSourceFenceLocked(input.Actor, input.Operation)
	o.RUnlock()
	if err == nil {
		t.Fatal("intent disclosed as fence")
	}
	f, raw := frozenSource(t, o, input)
	o.RLock()
	_, _, err = o.TransferSourceFenceLocked(values.PrincipalRef{}, input.Operation)
	o.RUnlock()
	if err == nil {
		t.Fatal("foreign actor disclosed transfer")
	}
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer reopened.CloseStore()
	reopened.RLock()
	after, bytes, err := reopened.TransferSourceFenceLocked(input.Actor, input.Operation)
	reopened.RUnlock()
	if err != nil || after != f || bytes != raw {
		t.Fatal("reopen changed immutable fence", err)
	}
}
