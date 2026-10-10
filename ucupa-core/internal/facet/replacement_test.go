package facet_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/facet"
)

var errReplacementPeer = errors.New("replacement peer unreachable")

// replacementSourcePeer holds the durable replacement facts of the designated
// source Facet service: its fence evidence, its live payloads and its
// settlement. It never parses or re-encodes manifest bytes.
type replacementSourcePeer struct {
	key      values.FacetAcceptanceKey
	refuse   bool
	fenced   bool
	settled  bool
	target   values.FactAuthorityRef
	finalCut uint64
	manifest []byte
	payloads map[values.FacetTypeRef][]byte
	fail     error
}

func (s *replacementSourcePeer) CheckSourceExport(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID) (replacement.FacetReplacementSourceState, error) {
	if s.refuse {
		return replacement.FacetReplacementSourceState{Found: true, Refused: true}, nil
	}
	if s.fenced {
		return replacement.FacetReplacementSourceState{Found: true, Fenced: true, FinalCut: s.finalCut, ManifestDigest: sha256.Sum256(s.manifest), Manifest: s.manifest}, nil
	}
	return replacement.FacetReplacementSourceState{Found: true}, nil
}

func (s *replacementSourcePeer) FenceSource(_ context.Context, _ values.NextHop, _ values.FacetAcceptanceKey, _ contract.OperationID, target values.FactAuthorityRef) (replacement.FacetReplacementFenced, error) {
	if s.fail != nil {
		return replacement.FacetReplacementFenced{}, s.fail
	}
	s.fenced, s.target = true, target
	return replacement.FacetReplacementFenced{Key: s.key, FinalCut: s.finalCut, ManifestDigest: sha256.Sum256(s.manifest), Manifest: s.manifest}, nil
}

func (s *replacementSourcePeer) ReadSourceEntry(_ context.Context, _ values.NextHop, _ values.FacetAcceptanceKey, _ contract.OperationID, typ values.FacetTypeRef) (replacement.FacetReplacementEntry, error) {
	payload, ok := s.payloads[typ]
	if !ok {
		return replacement.FacetReplacementEntry{}, errReplacementPeer
	}
	return replacement.FacetReplacementEntry{Type: typ, Payload: payload}, nil
}

func (s *replacementSourcePeer) SettleSource(_ context.Context, _ values.NextHop, _ values.FacetAcceptanceKey, _ contract.OperationID, target values.FactAuthorityRef) error {
	s.settled, s.target = true, target
	return nil
}

// replacementTargetPeer holds the designated target Facet's staged import. It
// recomputes the manifest digest from the exact bytes the Core relays and
// discloses only the staged live key set, never payload bytes.
type replacementTargetPeer struct {
	key        values.FacetAcceptanceKey
	accepted   bool
	installed  bool
	source     values.FactAuthorityRef
	finalCut   uint64
	installCut uint64
	received   []byte
	core       values.NextHop
	live       []replacement.FacetReplacementLiveEntry
	staged     map[values.FacetTypeRef][]byte
	fail       error
}

func (t *replacementTargetPeer) CheckTargetImport(_ context.Context, _ values.NextHop, _ values.FacetAcceptanceKey, _ contract.OperationID, manifest []byte, core values.NextHop) (replacement.FacetReplacementImportState, error) {
	if t.fail != nil {
		return replacement.FacetReplacementImportState{}, t.fail
	}
	t.received = append([]byte(nil), manifest...)
	t.core = core
	digest := sha256.Sum256(manifest)
	if t.accepted {
		return replacement.FacetReplacementImportState{Accepted: true, Digest: digest, FinalCut: t.finalCut}, nil
	}
	var live []replacement.FacetReplacementLiveEntry
	for _, entry := range t.live {
		if _, staged := t.staged[entry.Type]; !staged {
			live = append(live, entry)
		}
	}
	return replacement.FacetReplacementImportState{Digest: digest, FinalCut: t.finalCut, Live: live}, nil
}

func (t *replacementTargetPeer) StageTargetEntry(_ context.Context, _ values.NextHop, _ values.FacetAcceptanceKey, _ contract.OperationID, typ values.FacetTypeRef, payload []byte) error {
	t.staged[typ] = append([]byte(nil), payload...)
	return nil
}

func (t *replacementTargetPeer) AcceptTarget(_ context.Context, _ values.NextHop, _ values.FacetAcceptanceKey, _ contract.OperationID, _ values.NextHop) (replacement.FacetReplacementAccepted, error) {
	if t.fail != nil {
		return replacement.FacetReplacementAccepted{}, t.fail
	}
	t.accepted = true
	return replacement.FacetReplacementAccepted{Key: t.key}, nil
}

func (t *replacementTargetPeer) InstallTarget(_ context.Context, _ values.NextHop, _ values.FacetAcceptanceKey, _ contract.OperationID, source values.FactAuthorityRef, finalCut uint64) error {
	t.installed, t.source, t.installCut = true, source, finalCut
	return nil
}

func (t *replacementTargetPeer) ReadTargetImport(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID) (replacement.FacetReplacementImportState, error) {
	if t.accepted {
		return replacement.FacetReplacementImportState{Accepted: true, Installed: t.installed, Digest: sha256.Sum256(t.received), FinalCut: t.finalCut}, nil
	}
	return replacement.FacetReplacementImportState{}, errReplacementPeer
}

// replacementFixture activates one real attachment on the Core so the public
// replacement entries always start from the exact current relationship.
func replacementFixture(t *testing.T, durable bool) (fixture, values.FactAuthorityRef, contract.PublicFacetInspection) {
	t.Helper()
	f := publicFixture(t, durable, nil)
	pending := publicPropose(t, f, "replacement-propose")
	if err := f.facets.SetFacetAcceptanceSource(acceptanceSource(func(_ context.Context, _ values.NextHop, key values.FacetAcceptanceKey) (contract.FacetAcceptanceEvidence, error) {
		return accepted(key), nil
	})); err != nil {
		t.Fatal(err)
	}
	_, active, err := f.facets.ApplyFacetControl(context.Background(), publicCaller, "replacement-activate", contract.PublicFacetCommand{
		Kind: "activate", Element: facetEntity, Expected: pending.Revision,
	})
	if err != nil || active.State != contract.FacetActive || !active.Revision.Valid() {
		t.Fatalf("activate = %#v, %v", active, err)
	}
	target, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	return f, target, active
}

// replacementCommand fixes the caller-visible comparison of the current
// relationship; the source Facet identity stays Core-disclosed.
func replacementCommand(active contract.PublicFacetInspection, target values.FactAuthorityRef) contract.PublicFacetReplacementCommand {
	return contract.PublicFacetReplacementCommand{Element: facetEntity, Expected: active.Revision,
		Target: target, TargetAddress: "local:target-facet", SourceAddress: "local:core"}
}

// replacementPeers assembles the designated contacts for one begun operation.
// The source key comes from the retained record; the target key is the exact
// Core-issued acceptance key of this reservation.
func replacementPeers(t *testing.T, f fixture, id contract.OperationID, target values.FactAuthorityRef, payloadA, payloadB []byte) (replacement.FacetPeers, *replacementSourcePeer, *replacementTargetPeer) {
	t.Helper()
	record, err := f.facets.ReadFacetReplacementRecord(id, facetEntity)
	if err != nil {
		t.Fatal(err)
	}
	typeA, err := values.NewFacetTypeRef(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	typeB, err := values.NewFacetTypeRef(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte("opaque-facet-namespace-manifest")
	src := &replacementSourcePeer{key: facts(t, f.owner, facetEntity).attachment.Key, finalCut: 7, manifest: manifest,
		payloads: map[values.FacetTypeRef][]byte{typeA: payloadA, typeB: payloadB}}
	dst := &replacementTargetPeer{key: values.FacetAcceptanceKey{Core: f.owner.AuthorityRef(), Facet: target, Element: facetEntity, Proposal: record.Proposal},
		finalCut: 7,
		live:     []replacement.FacetReplacementLiveEntry{{Type: typeA, Length: uint64(len(payloadA))}, {Type: typeB, Length: uint64(len(payloadB))}},
		staged:   map[values.FacetTypeRef][]byte{}}
	return replacement.FacetPeers{Source: src, Target: dst}, src, dst
}

func assertSwitched(t *testing.T, f fixture, target values.FactAuthorityRef, id contract.OperationID) contract.FacetReplacementRecord {
	t.Helper()
	record, err := f.facets.ReadFacetReplacementRecord(id, facetEntity)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Permitted || !record.Fenced || !record.Accepted || !record.Committed || !record.Settled || record.Cancelled {
		t.Fatalf("settled record = %#v", record)
	}
	after := facts(t, f.owner, facetEntity)
	if after.attachment.State != contract.FacetActive || after.attachment.Key.Facet != target ||
		after.attachment.Key.Proposal != record.Proposal || after.pending.Valid() || after.nextHop != "local:target-facet" {
		t.Fatalf("switched fact = %#v", after)
	}
	return record
}

func TestPublicFacetReplacementFullChain(t *testing.T) {
	f, target, active := replacementFixture(t, true)
	sourceRef := facts(t, f.owner, facetEntity).attachment.Key.Facet
	if sourceRef == target || !sourceRef.Valid() {
		t.Fatal("replacement target must differ from the source")
	}
	command := replacementCommand(active, target)
	status, err := f.facets.BeginFacetReplacement(context.Background(), publicCaller, "full-op", command)
	if err != nil || status.Authority != f.owner.AuthorityRef() || status.Operation != "full-op" || status.Element != facetEntity ||
		status.Source != sourceRef || status.Target != target || status.Progress != contract.FacetReplacementReserved || status.Refused ||
		!status.Revision.Valid() || status.Revision.Kind != "facet-replacement-revision" || status.Revision.Owner != f.owner.AuthorityRef() {
		t.Fatalf("begin = %#v, %v", status, err)
	}
	again, err := f.facets.BeginFacetReplacement(context.Background(), publicCaller, "full-op", command)
	if err != nil || again != status {
		t.Fatalf("repeated begin = %#v, %v", again, err)
	}
	changed := command
	changed.TargetAddress = "local:elsewhere"
	if _, err = f.facets.BeginFacetReplacement(context.Background(), publicCaller, "full-op", changed); !errors.Is(err, values.PublicConflict) {
		t.Fatalf("changed input reused the reservation: %v", err)
	}
	inspected, err := f.facets.InspectFacetReplacement(context.Background(), publicCaller, "full-op", facetEntity)
	if err != nil || inspected != status {
		t.Fatalf("inspect = %#v, %v", inspected, err)
	}
	if _, err = f.facets.InspectFacetReplacement(context.Background(), publicCaller, "missing-op", facetEntity); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("foreign operation inspect = %v", err)
	}
	if _, err = f.facets.InspectFacetReplacement(context.Background(), publicCaller, "full-op", facetRoot); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("foreign element inspect = %v", err)
	}
	foreign := publicCaller
	foreign.Subject = values.PrincipalRef{Realm: "other", Principal: "reader"}
	if _, err = f.facets.InspectFacetReplacement(context.Background(), foreign, "full-op", facetEntity); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("foreign caller inspect = %v", err)
	}

	payloadA, payloadB := []byte("payload-a"), []byte("payload-b")
	peers, src, dst := replacementPeers(t, f, "full-op", target, payloadA, payloadB)
	record, err := f.facets.ReadFacetReplacementRecord("full-op", facetEntity)
	if err != nil || record.Authority != f.owner.AuthorityRef() || record.Operation != "full-op" || record.Source != sourceRef ||
		record.Target != target || !record.Proposal.Valid() || record.Permitted || record.Fenced || record.FinalCut != 0 {
		t.Fatalf("reserved record = %#v, %v", record, err)
	}
	final, err := f.facets.RecoverFacetReplacement(context.Background(), publicCaller, "full-op", facetEntity, peers)
	if err != nil || final.Progress != contract.FacetReplacementSettled || final.Refused || final.Operation != "full-op" {
		t.Fatalf("recovered = %#v, %v", final, err)
	}
	if _, err = f.facets.RecoverFacetReplacement(context.Background(), publicCaller, "full-op", facetRoot, peers); !errors.Is(err, values.PublicDenied) {
		t.Fatalf("foreign element recovery = %v", err)
	}
	settled := assertSwitched(t, f, target, "full-op")
	if settled.FinalCut != 7 || settled.ManifestDigest != sha256.Sum256(src.manifest) {
		t.Fatalf("settled fence facts = %#v", settled)
	}
	if !src.fenced || !src.settled || src.target != target {
		t.Fatalf("source peer = %#v", src)
	}
	if !dst.accepted || !dst.installed || dst.source != sourceRef || dst.installCut != 7 || dst.core != "local:core" ||
		!bytes.Equal(dst.received, src.manifest) || len(dst.staged) != 2 {
		t.Fatalf("target peer = %#v", dst)
	}
	for _, staged := range dst.staged {
		if !bytes.Equal(staged, payloadA) && !bytes.Equal(staged, payloadB) {
			t.Fatalf("relayed payload = %q", staged)
		}
	}
	if _, err = f.facets.BeginFacetReplacement(context.Background(), publicCaller, "stale-op", command); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("stale comparison begin = %v", err)
	}
	if _, err = f.facets.ReadFacetReplacementRecord("full-op", facetRoot); !errors.Is(err, values.PublicNotFound) {
		t.Fatalf("foreign element record read = %v", err)
	}
	if _, err = f.facets.ReadFacetReplacementRecord("", facetEntity); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("invalid record read = %v", err)
	}
}

func TestPublicFacetReplacementRecoversAcrossRestart(t *testing.T) {
	f, target, active := replacementFixture(t, true)
	command := replacementCommand(active, target)
	status, err := f.facets.BeginFacetReplacement(context.Background(), publicCaller, "restart-op", command)
	if err != nil || status.Progress != contract.FacetReplacementReserved {
		t.Fatalf("begin = %#v, %v", status, err)
	}
	payloadA, payloadB := []byte("payload-a"), []byte("payload-b")
	peers, src, dst := replacementPeers(t, f, "restart-op", target, payloadA, payloadB)
	dst.fail = errReplacementPeer
	if _, err = f.facets.RecoverFacetReplacement(context.Background(), publicCaller, "restart-op", facetEntity, peers); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("interrupted recovery = %v", err)
	}
	inspected, err := f.facets.InspectFacetReplacement(context.Background(), publicCaller, "restart-op", facetEntity)
	if err != nil || inspected.Progress != contract.FacetReplacementFenced || inspected.Revision != status.Revision {
		t.Fatalf("interrupted status = %#v, %v", inspected, err)
	}
	f.owner.Lock()
	f.owner.BeginCloseLocked()
	f.owner.Unlock()
	if err = f.owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	reopened, openErr := commit.OpenDurable(f.path, nil)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer reopened.CloseStore()
	f.owner, f.facets = reopened, facet.New(reopened, f.auth, access.New(f.auth, reopened, publicLimits(8)), nil)

	dst.fail = nil
	final, err := f.facets.RecoverFacetReplacement(context.Background(), publicCaller, "restart-op", facetEntity, peers)
	if err != nil || final.Progress != contract.FacetReplacementSettled || final.Revision != status.Revision {
		t.Fatalf("restarted recovery = %#v, %v", final, err)
	}
	assertSwitched(t, f, target, "restart-op")
	if !bytes.Equal(dst.received, src.manifest) {
		t.Fatal("relayed manifest bytes diverged from the journal-retained form")
	}
	if len(dst.staged) != 2 || !src.settled || !dst.installed {
		t.Fatal("restart lost staged peer progress")
	}
}

func TestPublicFacetReplacementCancellationWindows(t *testing.T) {
	f, target, active := replacementFixture(t, true)
	command := replacementCommand(active, target)
	ctx := context.Background()
	free, err := f.facets.BeginFacetReplacement(ctx, publicCaller, "free-op", command)
	if err != nil || free.Progress != contract.FacetReplacementReserved {
		t.Fatalf("begin = %#v, %v", free, err)
	}
	if _, err = f.facets.CancelFacetReplacement(ctx, publicCaller, "free-op", facetEntity, values.PublicToken{}); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("empty comparison cancel = %v", err)
	}
	if _, err = f.facets.CancelFacetReplacement(ctx, publicCaller, "free-op", facetEntity, active.Revision); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("wrong-kind comparison cancel = %v", err)
	}
	foreignOwner, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.facets.CancelFacetReplacement(ctx, publicCaller, "free-op", facetEntity, serviceToken(t, foreignOwner, "facet-replacement-revision")); !errors.Is(err, values.PublicInvalid) {
		t.Fatalf("foreign-owner comparison cancel = %v", err)
	}
	cancelled, err := f.facets.CancelFacetReplacement(ctx, publicCaller, "free-op", facetEntity, free.Revision)
	if err != nil || cancelled.Progress != contract.FacetReplacementCancelled || cancelled.Revision != free.Revision {
		t.Fatalf("free cancel = %#v, %v", cancelled, err)
	}
	repeat, err := f.facets.CancelFacetReplacement(ctx, publicCaller, "free-op", facetEntity, free.Revision)
	if err != nil || repeat != cancelled {
		t.Fatalf("repeated cancel = %#v, %v", repeat, err)
	}
	peers, src, dst := replacementPeers(t, f, "free-op", target, []byte("payload-a"), []byte("payload-b"))
	if terminal, err := f.facets.RecoverFacetReplacement(ctx, publicCaller, "free-op", facetEntity, peers); err != nil || terminal != cancelled {
		t.Fatalf("recovery of a cancelled replacement = %#v, %v", terminal, err)
	}

	// The element released by the cancellation accepts the next replacement.
	guard, err := f.facets.BeginFacetReplacement(ctx, publicCaller, "guard-op", command)
	if err != nil || guard.Progress != contract.FacetReplacementReserved {
		t.Fatalf("second begin = %#v, %v", guard, err)
	}
	peers, src, dst = replacementPeers(t, f, "guard-op", target, []byte("payload-a"), []byte("payload-b"))
	src.fail = errReplacementPeer
	if _, err = f.facets.RecoverFacetReplacement(ctx, publicCaller, "guard-op", facetEntity, peers); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("fenced-off recovery = %v", err)
	}
	inspected, err := f.facets.InspectFacetReplacement(ctx, publicCaller, "guard-op", facetEntity)
	if err != nil || inspected.Progress != contract.FacetReplacementPermitted || inspected.Refused {
		t.Fatalf("permitted status = %#v, %v", inspected, err)
	}
	if _, err = f.facets.CancelFacetReplacement(ctx, publicCaller, "guard-op", facetEntity, guard.Revision); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("permitted cancel without refusal = %v", err)
	}
	src.fail, src.refuse = nil, true
	refused, err := f.facets.RecoverFacetReplacement(ctx, publicCaller, "guard-op", facetEntity, peers)
	if err != nil || !refused.Refused || refused.Progress != contract.FacetReplacementPermitted || refused.Revision != guard.Revision {
		t.Fatalf("refusal discovery = %#v, %v", refused, err)
	}
	if src.fenced || src.settled {
		t.Fatal("refused source was fenced or settled anyway")
	}
	settled, err := f.facets.CancelFacetReplacement(ctx, publicCaller, "guard-op", facetEntity, guard.Revision)
	if err != nil || settled.Progress != contract.FacetReplacementCancelled {
		t.Fatalf("refusal-backed cancel = %#v, %v", settled, err)
	}
	if dst.accepted || dst.installed || len(dst.staged) != 0 {
		t.Fatalf("cancelled replacement touched the target = %#v", dst)
	}
}

func TestPublicFacetReplacementRequiresDurableCore(t *testing.T) {
	f, target, active := replacementFixture(t, false)
	if _, err := f.facets.BeginFacetReplacement(context.Background(), publicCaller, "memory-op", replacementCommand(active, target)); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("memory begin = %v", err)
	}
}
