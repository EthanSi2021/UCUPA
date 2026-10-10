package commit

import (
	"encoding/json"
	"testing"

	contract "ucupa/ucupa-core/contract"
	storebytes "ucupa/ucupa-core/internal/store"
)

func TestMountCommitCodecSeparatesDecisionSources(t *testing.T) {
	for _, event := range []coreCommit{
		{kind: elementCommitted, operation: contract.OperationID(string([]byte{0, 0xff, 0x80}))},
		{kind: mountCommitted, controlIndex: 1},
		{kind: mountCommitted, controlIndex: ^uint64(0)},
	} {
		stored, err := storeCommit(event)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(stored)
		if err != nil {
			t.Fatal(err)
		}
		var decoded storedCommit
		if err := storebytes.DecodeCanonicalJSON(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		actual, err := decoded.commit()
		if err != nil || actual != event {
			t.Fatalf("round trip %q: got %#v, %v; want %#v", encoded, actual, err, event)
		}
	}
	for _, event := range []coreCommit{
		{}, {kind: commitKind(255)},
		{kind: elementCommitted},
		{kind: elementCommitted, operation: "element", controlIndex: 1},
		{kind: mountCommitted},
		{kind: mountCommitted, operation: "element", controlIndex: 1},
	} {
		if _, err := storeCommit(event); err == nil {
			t.Fatalf("invalid commit accepted: %#v", event)
		}
	}
	for _, stored := range []storedCommit{
		{}, {Kind: "unknown"},
		{Kind: "element"},
		{Kind: "element", Operation: []byte("element"), ControlIndex: 1},
		{Kind: "mount"},
		{Kind: "mount", Operation: []byte("element"), ControlIndex: 1},
	} {
		if _, err := stored.commit(); err == nil {
			t.Fatalf("invalid stored commit accepted: %#v", stored)
		}
	}
}

func TestMountCommitCodecRejectsNoncanonicalOrMixedShape(t *testing.T) {
	for _, encoded := range []string{
		`{"kind":"mount","controlIndex":1,"extra":true}`,
		`{"kind":"mount","controlIndex":1,"controlIndex":1}`,
		`{ "kind":"mount","controlIndex":1}`,
		`{"kind":"mount","operation":"","controlIndex":1}`,
		`{"kind":"mount","controlIndex":0}`,
		`{"kind":"mount","controlIndex":1} {}`,
	} {
		var stored storedCommit
		err := storebytes.DecodeCanonicalJSON([]byte(encoded), &stored)
		if err == nil {
			_, err = stored.commit()
		}
		if err == nil {
			t.Fatalf("malformed commit accepted: %s", encoded)
		}
	}
}
