package commit

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func transitionCommand(o *Owner, i transferfact.Intent) contract.TransitionCommand {
	return contract.TransitionCommand{Kind: contract.TransitionSlice, Target: i.Target, Boundary: i.Boundary,
		Expected: o.SigningLocked().ElementRevision(i.Expected), Final: i.Final,
		SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress}
}

func TestPublicTransitionRecoversOriginalCASAfterFenceWithoutReopeningSource(t *testing.T) {
	o, path, i := transferIntentFixture(t)
	o.Lock()
	command := transitionCommand(o, i)
	original, err := o.BeginPublicTransitionLocked(i.Actor, i.Operation, command)
	o.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fenceTransfer(o, i, 1); err != nil {
		t.Fatal(err)
	}
	closeFoundationStore(t, o)
	o = openFoundationStore(t, path)
	defer o.CloseStore()
	o.Lock()
	defer o.Unlock()
	r, err := o.BeginPublicTransitionLocked(i.Actor, i.Operation, command)
	if err != nil || r.Progress != contract.TransitionFenced || r.Revision == original.Revision {
		t.Fatalf("original retry after fence: %+v %v", r, err)
	}
	if _, err := o.CancelPublicTransitionLocked(i.Actor, i.Operation, i.Boundary, original.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("original cancel bypassed fence: %v", err)
	}
	if _, err := o.CancelPublicTransitionLocked(i.Actor, i.Operation, i.Boundary, r.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("current CAS bypassed fence: %v", err)
	}
	if o.state.elements.Kind(i.Boundary) != 0 || len(o.state.transfers.events) != 2 {
		t.Fatal("read/cancel changed fence or reopened source")
	}
}

func TestPublicTransitionFaultKeepsOriginalComparisonAcrossRestart(t *testing.T) {
	for _, step := range []string{"begin", "cancel"} {
		for _, after := range []bool{false, true} {
			t.Run(step+map[bool]string{false: "/before", true: "/after"}[after], func(t *testing.T) {
				o, path, i := transferIntentFixture(t)
				o.Lock()
				command := transitionCommand(o, i)
				var initial contract.TransitionStatus
				var err error
				if step == "cancel" {
					initial, err = o.BeginPublicTransitionLocked(i.Actor, i.Operation, command)
					if err != nil {
						o.Unlock()
						t.Fatal(err)
					}
				}
				failure := func() error { return errors.New("store response lost") }
				if after {
					o.store.failAfterCommit = failure
				} else {
					o.store.failBeforeCommit = failure
				}
				var out contract.TransitionStatus
				if step == "begin" {
					out, err = o.BeginPublicTransitionLocked(i.Actor, i.Operation, command)
				} else {
					out, err = o.CancelPublicTransitionLocked(i.Actor, i.Operation, i.Boundary, initial.Revision)
				}
				o.Unlock()
				if !errors.Is(err, contract.ErrStore) || out != (contract.TransitionStatus{}) {
					t.Fatalf("uncertain result disclosed success: %+v %v", out, err)
				}
				closeFoundationStore(t, o)
				o = openFoundationStore(t, path)
				defer o.CloseStore()
				o.Lock()
				defer o.Unlock()
				if step == "begin" {
					out, err = o.BeginPublicTransitionLocked(i.Actor, i.Operation, command)
				} else {
					out, err = o.CancelPublicTransitionLocked(i.Actor, i.Operation, i.Boundary, initial.Revision)
				}
				want := contract.TransitionIntent
				count := 1
				if step == "cancel" {
					want, count = contract.TransitionCancelled, 2
				}
				if err != nil || out.Progress != want || !out.Revision.Valid() || len(o.state.transfers.events) != count {
					t.Fatalf("exact recovery: %+v %v", out, err)
				}
			})
		}
	}
}

func TestPublicTransitionCannotUseRandomOrOtherControlComparison(t *testing.T) {
	o, _, i := transferIntentFixture(t)
	o.Lock()
	defer o.Unlock()
	command := transitionCommand(o, i)
	fake, err := values.NewPublicToken(o.authority, "record-revision")
	if err != nil {
		t.Fatal(err)
	}
	bad := command
	bad.Expected = fake
	if _, err := o.BeginPublicTransitionLocked(i.Actor, i.Operation, bad); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("self-issued comparison accepted: %v", err)
	}
	if len(o.state.transfers.events) != 0 {
		t.Fatal("rejected comparison claimed operation")
	}
	if _, err := o.BeginPublicTransitionLocked(i.Actor, i.Operation, command); err != nil {
		t.Fatal(err)
	}
	other, err := o.BeginPublicTransitionLocked(i.Actor, "other", command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.CancelPublicTransitionLocked(i.Actor, i.Operation, i.Boundary, other.Revision); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("another operation comparison accepted: %v", err)
	}
	fake, _ = values.NewPublicToken(o.authority, "transition-revision")
	if _, err := o.CancelPublicTransitionLocked(i.Actor, i.Operation, i.Boundary, fake); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("random control comparison accepted: %v", err)
	}
}
