package commit

import (
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
)

// This fixture retains the original guard test's five deterministic control
// transitions. It tests the sole current-facts/history validator, not transport
// or authorization, and does not manufacture settled mount records.
func mountGuardValidationPair(t *testing.T) (parent, carrier *Owner, mount values.ElementID) {
	t.Helper()
	newOwner := func(scope byte) *Owner {
		authority, err := values.NewFactAuthorityRef()
		if err != nil {
			t.Fatal(err)
		}
		owner, err := New(authority, [32]byte{scope}, contract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
		if err != nil {
			t.Fatal(err)
		}
		return owner
	}
	parent, carrier = newOwner(1), newOwner(2)
	root, mount := foundationStoreID(9100), foundationStoreID(9101)
	apply := func(owner *Owner, operation contract.OperationID, kind elementfact.OperationKind, id values.ElementID, final contract.SpatialTuple) contract.MutationResult {
		t.Helper()
		owner.Lock()
		defer owner.Unlock()
		result, err := owner.ApplyElementLocked(operation, commitbinding.Binding{
			Actor: foundationStoreActor, Kind: kind, Element: id, Final: final,
		})
		if err != nil || result.Status != contract.MutationCommitted {
			t.Fatalf("guard fixture Element %s: %#v, %v", operation, result, err)
		}
		return result
	}
	apply(parent, "guard-root", elementfact.OperationCreateRoot, root, contract.SpatialTuple{})
	created := apply(parent, "guard-mount", elementfact.OperationCreateAnchor, mount, foundationStoreTuple(t, root, 0, 10))
	advance := func(owner *Owner, event MountEvent) {
		t.Helper()
		next := owner.state.clone()
		event.BeforeCut = next.currentCut()
		if err := applyMountEvent(next, event); err != nil {
			t.Fatalf("control fixture transition %d: %v", event.Kind, err)
		}
		if err := validateCurrentSnapshot(next); err != nil {
			t.Fatalf("control fixture snapshot after %d: %v", event.Kind, err)
		}
		owner.state = next
	}
	const operation contract.OperationID = "guard-establish"
	advance(parent, MountEvent{Kind: MountEventReserve, Operation: operation, Authorized: true,
		Binding: MountBinding{Actor: foundationStoreActor, PeerActor: foundationStoreActor, Command: contract.MountCommand{
			Kind: contract.MountEstablish, MountAnchorUUID: mount, ExpectedElementRevision: created.Record.Anchor.Revision,
			CarrierAuthority: carrier.authority, CarrierNextHop: "local:guard-b", ParentNextHop: "local:guard-a",
		}},
	})
	request := parent.state.mount.operations[operation].Request
	advance(carrier, MountEvent{Kind: MountEventPrepare, Request: request, Source: parent.authority, Authorized: true})
	attempt := contract.MountAttemptKey{ParentAuthority: parent.authority, OperationID: operation}
	if !carrier.state.mount.attempts[attempt].Prepared.Prepared {
		t.Fatal("fixture prepare was rejected")
	}
	advance(parent, MountEvent{Kind: MountEventDecide, Operation: operation, Authorized: true,
		Prepared: carrier.state.mount.attempts[attempt].Prepared})
	advance(carrier, MountEvent{Kind: MountEventFinish, Source: parent.authority,
		Finish: contract.MountFinishRequest{Evidence: parent.state.mount.operations[operation].Evidence()}})
	advance(parent, MountEvent{Kind: MountEventSettle, Operation: operation,
		Closed: carrier.state.mount.attempts[attempt].Closed})
	apply(carrier, "entity", elementfact.OperationCreateEntity, foundationStoreID(9130), foundationStoreTuple(t, mount, 0, 1))
	return parent, carrier, mount
}

func TestMountGuardExternalStructureRejectsCorruption(t *testing.T) {
	parent, carrier, mount := mountGuardValidationPair(t)
	id := foundationStoreID(9130)
	for name, corrupt := range map[string]func(*coreState){
		"unrelated parent": func(s *coreState) {
			e, _ := s.elements.Entity(id)
			e.Parent = foundationStoreID(9199)
			_ = s.elements.RestoreEntity(e)
		},
		"missing incoming": func(s *coreState) { s.mount.facts.ReplaceIncoming(mountfact.Incoming{}) },
		"content before admission": func(s *coreState) {
			incoming := s.mount.facts.Incoming()
			incoming.Admission = mountfact.Admission{}
			s.mount.facts.ReplaceIncoming(incoming)
		},
		"outside admission": func(s *coreState) {
			e, _ := s.elements.Entity(id)
			e.LocalPose.Translation.X = 10
			_ = s.elements.RestoreEntity(e)
		},
		"external tombstone": func(s *coreState) {
			revision, _, _ := s.issueFactChange(mount)
			_ = s.elements.RestoreTombstone(mount, revision)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := carrier.state.clone()
			corrupt(s)
			if err := validateCurrentFacts(s); err == nil {
				t.Fatalf("invalid external shape accepted: %s", name)
			}
		})
	}
	for _, owner := range []*Owner{parent, carrier} {
		if err := validateCoreState(owner.state); err != nil {
			t.Fatalf("valid mount/Element interleaved history on %s: %v", owner.authority, err)
		}
	}
}
