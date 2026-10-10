package commit_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestPublicLostResponseKeepsSingleFactAndObservation(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "lost-response"}[afterCommit], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "public.db")
			cfg := contract.Config{PublicAccessPolicy: &values.PublicAccessPolicy{ContextTTL: time.Hour, ObservationTTL: time.Hour}}
			c, err := createPublicCommitFixture(path, cfg, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = c.close() })
			root := publicCommitRoot(t, c, publicCommitID(91600), "root")
			ref := map[string]any{"boundary": map[string]any{"core": string(c.owner.AuthorityRef()), "anchor": string(root)}, "nextHop": "local:core"}
			enter, err := publicCommitCall(t, c, map[string]any{"op": "enter", "intent": "list", "reference": ref})
			if err != nil {
				t.Fatal(err)
			}
			read, err := publicCommitCall(t, c, map[string]any{"op": "list", "handle": enter["handle"], "address": map[string]any{}, "prepareObserve": true})
			if err != nil {
				t.Fatal(err)
			}
			opID := values.EncodePublicBytes([]byte("create"))
			original := map[string]any{"op": "create-entity", "operationId": opID, "element": string(publicCommitID(91601)), "final": map[string]any{
				"parent": string(root), "pose": map[string]any{"translation": []any{0., 0., 0.}, "rotation": map[string]any{"pivot": 4., "ratios": []any{0., 0., 0.}}}, "range": map[string]any{"kind": "bounded-sphere", "radius": 1.}}}
			request := map[string]any{"handle": enter["handle"]}
			for k, v := range original {
				request[k] = v
			}
			commit.InjectWriteFailureForTest(c.owner, afterCommit, errors.New("injected commit uncertainty"))
			if _, err = publicCommitCall(t, c, request); !errors.Is(err, values.PublicIndeterminate) {
				t.Fatal(err)
			}
			recovery := map[string]any{"op": "recover-outcome", "reference": ref, "operationId": opID, "request": original}
			recovered, recoverErr := publicCommitCall(t, c, recovery)
			if afterCommit && (recoverErr != nil || recovered["status"] != "COMMITTED") {
				t.Fatal(recovered, recoverErr)
			}
			if !afterCommit && !errors.Is(recoverErr, values.PublicOutcomeUnknown) {
				t.Fatal(recovered, recoverErr)
			}
			begun, err := publicCommitCall(t, c, map[string]any{"op": "begin-observe", "preparation": read["preparation"]})
			if err != nil {
				t.Fatal(err)
			}
			poll := map[string]any{"op": "poll", "cursor": begun["cursor"]}
			batch, err := publicCommitCall(t, c, poll)
			want := 0
			if afterCommit {
				want = 1
			}
			if err != nil || len(batch["groups"].([]any)) != want {
				t.Fatal(batch, err)
			}
			result, err := publicCommitCall(t, c, request)
			if err != nil || result["status"] != "COMMITTED" {
				t.Fatal(result, err)
			}
			if afterCommit && !reflect.DeepEqual(result, recovered) {
				t.Fatal("recovery changed result")
			}
			batch, err = publicCommitCall(t, c, poll)
			if err != nil || len(batch["groups"].([]any)) != 1 {
				t.Fatal("replay or rollback produced false groups", batch, err)
			}
			if err = c.close(); err != nil {
				t.Fatal(err)
			}
			c, err = createPublicCommitFixture(path, cfg, true)
			if err != nil {
				t.Fatal(err)
			}
			again, err := publicCommitCall(t, c, recovery)
			if err != nil || !reflect.DeepEqual(again, result) {
				t.Fatal(again, err)
			}
			if _, err = publicCommitCall(t, c, poll); !errors.Is(err, values.PublicGap) {
				t.Fatal("restart restored ephemeral cursor", err)
			}
		})
	}
}
