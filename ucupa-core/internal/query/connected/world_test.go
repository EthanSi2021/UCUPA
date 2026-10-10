package connected

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	peercontract "ucupa/ucupa-core/contract/connected"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/query/exchange"
	coretoken "ucupa/ucupa-core/internal/token"
)

func worldTestElements(anchors []elementfact.Anchor, entities []elementfact.Entity) elementfact.View {
	builder := elementfact.NewProjection()
	for _, anchor := range anchors {
		builder.AddAnchor(anchor.ID, anchor)
	}
	for _, entity := range entities {
		builder.AddEntity(entity.ID, entity)
	}
	return builder.Build().View
}

func worldTestAttachments(ids ...values.ElementID) facetfact.View {
	builder := facetfact.NewProjectionBuilder()
	for _, id := range ids {
		builder.Add(id, facetfact.Record{})
	}
	return builder.Build().View
}

func TestConnectedWorldMergesLiveViewsWithoutMountOrDuplicates(t *testing.T) {
	const (
		mount   values.ElementID        = "00000000-0000-4000-8000-000000009120"
		branch  values.ElementID        = "00000000-0000-4000-8000-000000009121"
		entity  values.ElementID        = "00000000-0000-4000-8000-000000009122"
		parent  values.FactAuthorityRef = "00000000-0000-4000-8000-000000009201"
		carrier values.FactAuthorityRef = "00000000-0000-4000-8000-000000009202"
	)
	key := coretoken.MountKey{
		ParentAuthority: parent, MountAnchorUUID: mount, CarrierAuthority: carrier,
		ProposalRevision: coretoken.MountRevisionFromParts([32]byte{1}, mount, 1),
	}
	parentMount := mountfact.New()
	parentMount.ReplaceOutgoing(mount, mountfact.Record{
		State: contract.MountActive, Key: key,
		Admission: contract.MountAdmissionKey{MountKey: key, OperationID: "world-parent-admission"},
	})
	carrierMount := mountfact.New()
	carrierMount.ReplaceIncoming(mountfact.Incoming{Key: key, Gate: mountfact.GateOpen})

	parentFacts := exchange.Facts{
		Present: true, Authority: parent,
		Elements:    worldTestElements([]elementfact.Anchor{{ID: mount}}, nil),
		Attachments: worldTestAttachments(mount),
		Mount:       parentMount.View,
	}
	carrierFacts := exchange.Facts{
		Present: true, Authority: carrier,
		Elements:    worldTestElements([]elementfact.Anchor{{ID: branch}}, nil),
		Attachments: worldTestAttachments(branch),
		Mount:       carrierMount.View,
	}
	entityFacts := exchange.Facts{
		Present: true, Authority: carrier,
		Elements: worldTestElements(nil, []elementfact.Entity{{ID: entity, Parent: branch}}),
	}

	world, err := newConnectedWorld([]exchange.Facts{parentFacts, carrierFacts, entityFacts})
	if err != nil {
		t.Fatalf("disjoint connected facts rejected: %v", err)
	}
	if _, ok := world.elements.Anchor(mount); !ok {
		t.Fatal("parent Element view was not merged")
	}
	if _, ok := world.elements.Anchor(branch); !ok {
		t.Fatal("carrier Element view was not merged")
	}
	if _, ok := world.elements.Entity(entity); !ok {
		t.Fatal("carrier Entity view was not merged")
	}
	if _, ok := world.attachments.Record(mount); !ok {
		t.Fatal("parent current attachment was not merged")
	}
	if _, ok := world.attachments.Record(branch); !ok {
		t.Fatal("carrier current attachment was not merged")
	}
	if world.mount.OutgoingCount() != 0 || world.mount.Incoming().Key.Valid() {
		t.Fatal("world copied a source mount relation into the merged view")
	}
	if world.sources[mount].Authority != parent || world.sources[branch].Authority != carrier || world.sources[entity].Authority != carrier {
		t.Fatalf("world changed source ownership: %#v", world.sources)
	}

	if _, err := newConnectedWorld([]exchange.Facts{parentFacts, parentFacts}); !errors.Is(err, peercontract.ErrIdentityConflict) {
		t.Fatalf("duplicate Element identity was not rejected: %v", err)
	}
}
