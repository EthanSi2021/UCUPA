package commit

import (
	"encoding/json"
	"errors"
	"go.etcd.io/bbolt"
	"sync"
	"testing"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	"ucupa/ucupa-core/internal/token"
	transferencoding "ucupa/ucupa-core/internal/transfer/encoding"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func installTransfer(o *Owner, d transferfact.SourceDecision) (transferfact.TargetInstallation, error) {
	o.Lock()
	defer o.Unlock()
	return o.InstallTransferTargetLocked(d)
}

func decidedTransfer(t *testing.T) (*Owner, *Owner, transferfact.SourceDecision) {
	t.Helper()
	source, target, _, a := successorFixture(t)
	d, err := commitSuccessor(source, a, 2)
	if err != nil {
		t.Fatal(err)
	}
	return source, target, d
}

func TestTransferInstallationAtomicOwnRevisionsAndReplay(t *testing.T) {
	source, target, d := decidedTransfer(t)
	i := d.Acceptance.Fence.Input
	before := target.state.clone()
	path := target.store.db.Path()
	r, err := installTransfer(target, d)
	if err != nil || r.Decision != d || token.Serial(r.At) != before.cutSerial+1 || r.Revision != 2 {
		t.Fatal("install", r, err)
	}
	if len(target.state.commits) != len(before.commits)+1 || len(target.state.operations) != len(before.operations) || target.state.revisionSerial != before.revisionSerial+2 {
		t.Fatal("closure did not install in one commit with own revisions")
	}
	anchor, ok := target.state.elements.Anchor(i.Boundary)
	if !ok || anchor.Parent != i.Final.Parent || anchor.LocalPose != i.Final.LocalPose {
		t.Fatal("final placement changed")
	}
	if sphere, _ := anchor.Range.BoundedSphere(); sphere != i.Final.Range {
		t.Fatal("range normalized away")
	}
	entity, ok := target.state.elements.Entity(foundationStoreID(102))
	if !ok || entity.Parent != i.Boundary || entity.LocalPose.Translation.X != 1 {
		t.Fatal("descendant lost or changed")
	}
	for _, revision := range []contract.Revision{anchor.Revision, entity.Revision} {
		if token.RevisionScope(revision) != target.state.scope || token.RevisionSerial(revision) <= before.revisionSerial {
			t.Fatal("source revision reused")
		}
	}
	if anchor.Revision == entity.Revision {
		t.Fatal("duplicate revision")
	}
	if source.state.elements.Kind(i.Boundary) != 0 || !sourceVisibility(source.state).Inactive(i.Boundary) {
		t.Fatal("target installation reactivated source")
	}
	if err := validateCoreState(target.state); err != nil {
		t.Fatal("batch history", err)
	}
	target.RLock()
	candidate, valid := target.CaptureReadLocked()
	target.RUnlock()
	if !valid || !candidate.Projection.RevisionsValid() || candidate.Validate() != nil {
		t.Fatal("valid atomic batch rejected by snapshot metadata")
	}
	// Successful installation releases only this placement's holds. The old
	// source revision is not a writable CAS in the new owner.
	if err := target.state.guardTransferElement(i.Boundary); err != nil {
		t.Fatal("hold not released", err)
	}
	changed, err := applyFoundationStore(target, elementfact.OperationUpdateEntity, i.Actor, entity.ID, foundationStoreTuple(t, anchor.ID, 2, 2), entity.Revision, "target-update")
	if err != nil || changed.Status != contract.MutationCommitted {
		t.Fatal("new owner write", err)
	}
	if _, err := applyFoundationStore(target, elementfact.OperationUpdateAnchor, i.Actor, anchor.ID, i.Final, i.Expected, "source-cas"); err == nil {
		t.Fatal("source CAS accepted")
	}
	again, err := installTransfer(target, d)
	if err != nil || again != r {
		t.Fatal("retry overwrote later data", err)
	}
	current, _ := target.state.elements.Entity(entity.ID)
	if current.Revision != changed.Record.Entity.Revision {
		t.Fatal("original install re-executed")
	}
	durable := target.state.clone()
	closeFoundationStore(t, target)
	target = openFoundationStore(t, path)
	defer target.CloseStore()
	if !sameCompleteCoreState(durable, target.state) {
		t.Fatal("restart lost installation or later writes")
	}
	again, err = installTransfer(target, d)
	if err != nil || again != r {
		t.Fatal("reopen exact result", err)
	}
	// A later outbound transfer must encode revisions that can exceed the cut;
	// the canonical manifest has its own exact selected-revision bound.
	other, _ := values.NewFactAuthorityRef()
	input := transferfact.Intent{Profile: transferfact.SliceTransfer, Operation: "later-transfer", Actor: i.Actor, Source: target.authority, Target: other, Boundary: anchor.ID, Expected: anchor.Revision,
		Final: foundationStoreTuple(t, foundationStoreID(3000), 0, 20), SourceAddress: "local:target", TargetAddress: "local:third"}
	f, raw := frozenSource(t, target, input)
	if _, err := transferencoding.DecodeTransferManifest(f, raw); err != nil {
		t.Fatal("re-export rejected batch revisions", err)
	}
}

func TestTransferInstallationRequiresExactDecisionAndAcceptance(t *testing.T) {
	_, target, d := decidedTransfer(t)
	for name, mutate := range map[string]func(*transferfact.SourceDecision){
		"missing-decision":    func(d *transferfact.SourceDecision) { d.Revision = 0 },
		"wrong-control-scope": func(d *transferfact.SourceDecision) { d.At = d.Acceptance.At },
		"pre-fence-decision":  func(d *transferfact.SourceDecision) { d.At = d.Acceptance.Fence.FinalCut },
		"another-acceptance":  func(d *transferfact.SourceDecision) { d.Acceptance.Revision++ },
		"another-cut": func(d *transferfact.SourceDecision) {
			d.Acceptance.At = token.FromParts(token.Scope(d.Acceptance.At), token.Serial(d.Acceptance.At)+1)
		},
		"target": func(d *transferfact.SourceDecision) {
			d.Acceptance.Fence.Input.Target, _ = values.NewFactAuthorityRef()
		},
		"tuple":    func(d *transferfact.SourceDecision) { d.Acceptance.Fence.Input.Final.LocalPose.Translation.X++ },
		"manifest": func(d *transferfact.SourceDecision) { d.Acceptance.Fence.ManifestDigest[0] ^= 1 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := d
			mutate(&bad)
			before := target.state.clone()
			if _, err := installTransfer(target, bad); err == nil || !sameCompleteCoreState(before, target.state) {
				t.Fatal("bad install changed target", err)
			}
		})
	}
	r, err := installTransfer(target, d)
	if err != nil {
		t.Fatal(err)
	}
	bad := d
	bad.Revision++
	if _, err := installTransfer(target, bad); !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("different install retried", err)
	}
	if again, err := installTransfer(target, d); err != nil || again != r {
		t.Fatal("exact receipt not recovered", err)
	}
}

func TestTransferInstallationFaultsAndConcurrentRetries(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "response-lost"}[after], func(t *testing.T) {
			_, target, d := decidedTransfer(t)
			path := target.store.db.Path()
			before := target.state.clone()
			fail := func() error { return errors.New("installation response lost") }
			if after {
				target.store.failAfterCommit = fail
			} else {
				target.store.failBeforeCommit = fail
			}
			if _, err := installTransfer(target, d); !errors.Is(err, contract.ErrStore) {
				t.Fatal("fault", err)
			}
			if !target.AvailableLocked() || sameCompleteCoreState(before, target.state) == after {
				t.Fatal("wrong durable side")
			}
			durable := target.state.clone()
			closeFoundationStore(t, target)
			target = openFoundationStore(t, path)
			defer target.CloseStore()
			if !sameCompleteCoreState(durable, target.state) {
				t.Fatal("restart changed durable side")
			}
			var results [8]transferfact.TargetInstallation
			var errs [8]error
			var wg sync.WaitGroup
			for n := range results {
				wg.Add(1)
				go func(n int) { defer wg.Done(); results[n], errs[n] = installTransfer(target, d) }(n)
			}
			wg.Wait()
			for n := range results {
				if errs[n] != nil || results[n] != results[0] || results[n].Decision != d {
					t.Fatal("concurrent install", errs[n])
				}
			}
			if len(target.state.transfers.events) != 2 || target.state.revisionSerial != 3 || target.state.cutSerial != 3 {
				t.Fatal("duplicate installation")
			}
		})
	}
}

func TestTransferInstallationCorruptHistoryRejected(t *testing.T) {
	for _, kind := range []string{"decision-missing", "decision-scope", "decision-before-fence", "acceptance", "digest", "duplicate", "extra-manifest", "decision-on-acceptance", "installed-record", "batch-bound"} {
		t.Run(kind, func(t *testing.T) {
			_, target, d := decidedTransfer(t)
			if _, err := installTransfer(target, d); err != nil {
				t.Fatal(err)
			}
			path := target.store.db.Path()
			closeFoundationStore(t, target)
			mutateFoundationStore(t, path, func(tx *bbolt.Tx) {
				b := tx.Bucket(elementBucketName)
				if kind == "installed-record" {
					key := currentStoreElementKey(t, d.Acceptance.Fence.Input.Boundary)
					if b.Get(key) == nil {
						t.Fatal("missing corruption fixture")
					}
					if err := b.Delete(key); err != nil {
						t.Fatal(err)
					}
					return
				}
				var events []transferencoding.StoredTransferEvent
				if err := json.Unmarshal(b.Get(transferJournalKey), &events); err != nil {
					t.Fatal(err)
				}
				e := &events[1]
				switch kind {
				case "decision-missing":
					e.Decision = nil
				case "decision-scope":
					e.Decision.Scope = e.Decision.Acceptance.Scope
				case "decision-before-fence":
					e.Decision.Cut = e.Decision.Acceptance.Fence.Cut
				case "acceptance":
					e.Decision.Acceptance.Revision++
				case "digest":
					e.Decision.Acceptance.Fence.Digest[0] ^= 1
				case "duplicate":
					events = append(events, *e)
				case "extra-manifest":
					e.Manifest = []byte("unexpected")
				case "decision-on-acceptance":
					events[0].Decision = e.Decision
				case "batch-bound":
					var m transferencoding.StoredTransferManifest
					if err := json.Unmarshal(events[0].Manifest, &m); err != nil {
						t.Fatal(err)
					}
					m.RevisionLimit++
					events[0].Manifest, _ = json.Marshal(m)
				}
				data, err := json.Marshal(events)
				if err != nil {
					t.Fatal(err)
				}
				if err := b.Put(transferJournalKey, data); err != nil {
					t.Fatal(err)
				}
			})
			opened, err := OpenDurable(path, nil)
			if opened != nil {
				_ = opened.CloseStore()
				t.Fatal("opened corrupt installation")
			}
			if !errors.Is(err, contract.ErrStore) {
				t.Fatal("corruption not classified", err)
			}
		})
	}
}
