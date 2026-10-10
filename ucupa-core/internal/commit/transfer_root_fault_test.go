package commit

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"ucupa/ucupa-core/contract"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Each existing journal transition is interrupted on both durable sides. The
// same input is retried after reopening BOTH owners; no role is reconstituted
// from runtime flags or an assumed peer outcome.
func TestRootTransferFaultsReopenAndConcurrentRetries(t *testing.T) {
	for stage := 0; stage < 5; stage++ {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("stage-%d/committed-%t", stage, after), func(t *testing.T) {
				source, _, _ := transferIntentFixture(t)
				target := emptyRootReceiver(t)
				i := rootTransferIntent(t, source, target, "root-fault")
				pending, err := beginTransfer(t, source, i)
				if err != nil {
					t.Fatal(err)
				}
				var f transferfact.SourceFence
				var raw string
				var fenceRevision uint64
				var a transferfact.TargetAcceptance
				var d transferfact.SourceDecision
				var installed transferfact.TargetInstallation
				run := func(n int) (any, error) {
					switch n {
					case 0:
						return fenceTransfer(source, i, pending.Revision)
					case 1:
						return acceptTransfer(t, target, f, raw)
					case 2:
						return commitSuccessor(source, a, fenceRevision)
					case 3:
						return installTransfer(target, d)
					default:
						return settleTransfer(source, installed)
					}
				}
				retain := func(n int, value any) {
					t.Helper()
					switch n {
					case 0:
						fenceRevision = value.(transferfact.IntentRecord).FenceRevision
						var err error
						source.RLock()
						f, raw, err = source.TransferSourceFenceLocked(i.Actor, i.Operation)
						source.RUnlock()
						if err != nil {
							t.Fatal(err)
						}
					case 1:
						a = value.(transferfact.TargetAcceptance)
					case 2:
						d = value.(transferfact.SourceDecision)
					case 3:
						installed = value.(transferfact.TargetInstallation)
					}
				}
				for n := 0; n < stage; n++ {
					v, err := run(n)
					if err != nil {
						t.Fatal("prepare", n, err)
					}
					retain(n, v)
				}
				affected := source
				if stage == 1 || stage == 3 {
					affected = target
				}
				before := affected.state.clone()
				fail := func() error { return errors.New("root durable boundary failure") }
				if after {
					affected.store.failAfterCommit = fail
				} else {
					affected.store.failBeforeCommit = fail
				}
				if _, err := run(stage); !errors.Is(err, contract.ErrStore) {
					t.Fatal("fault not reached", err)
				}
				if sameCompleteCoreState(before, affected.state) == after {
					t.Fatal("wrong side of durable boundary")
				}
				for _, ptr := range []**Owner{&source, &target} {
					o := *ptr
					path, durable := o.store.db.Path(), o.state.clone()
					closeFoundationStore(t, o)
					*ptr = openFoundationStore(t, path)
					t.Cleanup(func() { _ = (*ptr).CloseStore() })
					if !sameCompleteCoreState(durable, (*ptr).state) {
						t.Fatal("reopen invented root progress")
					}
				}
				// Calls only touch their owner's locked transaction. The operands
				// above are immutable during these identical retry attempts.
				var results [8]any
				var errs [8]error
				var wg sync.WaitGroup
				for n := range results {
					wg.Add(1)
					go func(n int) { defer wg.Done(); results[n], errs[n] = run(stage) }(n)
				}
				wg.Wait()
				for n := range results {
					if errs[n] != nil || !reflect.DeepEqual(results[n], results[0]) {
						t.Fatal("retry disagreed", n, errs[n])
					}
				}
				retain(stage, results[0])
				for n := stage + 1; n < 5; n++ {
					v, err := run(n)
					if err != nil {
						t.Fatal("complete", n, err)
					}
					retain(n, v)
				}
				if source.state.elements.RootExists() || !target.state.elements.RootExists() {
					t.Fatal("wrong final owner")
				}
				if len(source.state.transfers.events) != 4 || len(target.state.transfers.events) != 2 || target.state.revisionSerial != 3 {
					t.Fatal("duplicate root commit")
				}
				for _, o := range []*Owner{source, target} {
					if err := validateCoreState(o.state); err != nil {
						t.Fatal("final replay", err)
					}
				}
			})
		}
	}
}
