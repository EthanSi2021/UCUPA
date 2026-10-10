package commit

import (
	"errors"
	"sync"
	"testing"

	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

func competingRootMount(t *testing.T, target *Owner) (*Owner, contract.MountPrepareRequest) {
	t.Helper()
	parent, _, _ := transferIntentFixture(t)
	// Use a genuinely empty Anchor: an occupied parent cannot establish a
	// mount and would turn the supposed race into an invalid zero request.
	id := foundationStoreID(9501)
	if _, err := applyFoundationStore(parent, elementfact.OperationCreateAnchor, foundationStoreActor, id, foundationStoreTuple(t, foundationStoreID(100), 0, 10), contract.Revision{}, "empty-competing-anchor"); err != nil {
		t.Fatal(err)
	}
	anchor, _ := parent.state.elements.Anchor(id)
	b := MountBinding{Actor: foundationStoreActor, PeerActor: foundationStoreActor, Command: contract.MountCommand{
		Kind: contract.MountEstablish, MountAnchorUUID: anchor.ID, ExpectedElementRevision: anchor.Revision, CarrierAuthority: target.authority,
		CarrierNextHop: "local:root-target", ParentNextHop: "local:competing-parent",
	}}
	parent.Lock()
	err := parent.ApplyMountIntentLocked(contract.LocalCallers{target.authority: foundationStoreActor}, "competing-mount", b)
	parent.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	request := parent.state.mount.operations["competing-mount"].Request
	if !ValidMountRequest(request) {
		t.Fatal("competing mount did not reserve a valid request")
	}
	return parent, request
}

func TestRootAcceptanceSerializesCompetingActivation(t *testing.T) {
	for _, kind := range []string{"create-root", "incoming-mount", "other-transfer"} {
		t.Run(kind, func(t *testing.T) {
			for attempt := 0; attempt < 8; attempt++ {
				source, _, _ := transferIntentFixture(t)
				target := emptyRootReceiver(t)
				i := rootTransferIntent(t, source, target, "root-compete")
				f, raw := frozenSource(t, source, i)
				var compete func() (bool, error)
				switch kind {
				case "create-root":
					compete = func() (bool, error) {
						r, err := applyFoundationStore(target, elementfact.OperationCreateRoot, foundationStoreActor, foundationStoreID(9500), contract.SpatialTuple{}, contract.Revision{}, "independent-root")
						return r.Status == contract.MutationCommitted, err
					}
				case "incoming-mount":
					parent, request := competingRootMount(t, target)
					compete = func() (bool, error) {
						target.Lock()
						defer target.Unlock()
						r, err := target.ApplyMountPrepareLocked(parent.authority, request, true)
						return r.Prepared, err
					}
				case "other-transfer":
					other := transferReceiver(t, foundationStoreID(9500))
					root, _ := other.state.elements.Anchor(foundationStoreID(9500))
					otherInput := i
					otherInput.Operation = "other-root"
					otherInput.Source = other.authority
					otherInput.Boundary = root.ID
					otherInput.Expected = root.Revision
					otherFence, otherRaw := frozenSource(t, other, otherInput)
					compete = func() (bool, error) {
						_, err := acceptTransfer(t, target, otherFence, otherRaw)
						return err == nil, err
					}
				}
				var acceptance transferfact.TargetAcceptance
				var acceptedErr, competingErr error
				var otherWon bool
				start := make(chan struct{})
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); <-start; acceptance, acceptedErr = acceptTransfer(t, target, f, raw) }()
				go func() { defer wg.Done(); <-start; otherWon, competingErr = compete() }()
				close(start)
				wg.Wait()
				if (acceptedErr == nil) == otherWon {
					t.Fatal("both or neither activated", acceptedErr, competingErr)
				}
				if err := validateCoreState(target.state); err != nil {
					t.Fatal("raced state invalid", err)
				}
				path, before := target.store.db.Path(), target.state.clone()
				closeFoundationStore(t, target)
				target = openFoundationStore(t, path)
				if !sameCompleteCoreState(before, target.state) {
					t.Fatal("reservation lost on reopen")
				}
				if acceptedErr == nil {
					if won, _ := compete(); won {
						t.Fatal("reopen lost root-slot hold")
					}
					d, err := commitSuccessor(source, acceptance, source.state.transfers.records[i.Operation].FenceRevision)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := installTransfer(target, d); err != nil {
						t.Fatal("reserved install", err)
					}
				} else if !errors.Is(acceptedErr, contract.ErrConflict) && !errors.Is(acceptedErr, contract.ErrMountCoordinationRequired) {
					t.Fatal("unexpected refusal", acceptedErr)
				}
				closeFoundationStore(t, target)
			}
		})
	}
}
