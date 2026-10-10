package commit

import (
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	elementfact "ucupa/ucupa-core/internal/element/fact"
	facetfact "ucupa/ucupa-core/internal/facet/fact"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// These tests drive the sole commit owner through its real owner methods and
// journals. The manifest bytes stay opaque Facet evidence: the controller never
// parses them and cannot rederive their digest.
func facetReplacementFixture(t *testing.T) (*Owner, foundationStoreFixture, values.FactAuthorityRef, values.FactAuthorityRef) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.db")
	seeded := seedFoundationStore(t, path)
	o := openFoundationStore(t, path)
	original := transferFacetFixture(t, o, seeded.entityID, contract.FacetActive)
	target := foundationStoreOtherAuthority(t, original.Attachment.Key.Facet)
	return o, seeded, original.Attachment.Key.Facet, target
}

func beginFacetReplacement(t *testing.T, o *Owner, input transferfact.FacetReplacementIntent) transferfact.FacetReplacementControl {
	t.Helper()
	o.Lock()
	c, err := o.BeginFacetReplacementLocked(input)
	o.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFacetReplacementSwitchesNamespaceControl(t *testing.T) {
	o, seeded, source, target := facetReplacementFixture(t)
	defer closeFoundationStore(t, o)
	record, _ := o.state.attachments.facts.Record(seeded.entityID)
	manifest := "opaque-facet-namespace-bytes"
	input := transferfact.FacetReplacementIntent{Operation: "facet-replace", Actor: foundationStoreActor, Element: seeded.entityID,
		Source: source, Target: target, SourceKey: record.Attachment.Key, SourceNextHop: record.NextHop,
		SourceAddress: "local:core", TargetAddress: "local:target-facet", Expected: record.Attachment.Revision}
	c := beginFacetReplacement(t, o, input)
	if c.At == (contract.CoreReadCut{}) || !c.Proposal.Valid() || c.Proposal == record.Attachment.Revision || !c.Valid() {
		t.Fatal("reservation did not issue a fresh proposal", c)
	}
	if again := beginFacetReplacement(t, o, input); again != c {
		t.Fatal("repeated reservation is not idempotent")
	}
	other := input
	other.TargetAddress = "local:elsewhere"
	o.Lock()
	_, err := o.BeginFacetReplacementLocked(other)
	o.Unlock()
	if !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("changed input reused the reserved operation", err)
	}
	if err := o.state.guardTransferElement(seeded.entityID); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("open replacement does not hold its element", err)
	}

	actor, operation := foundationStoreActor, input.Operation
	o.Lock()
	permitted, err := o.PermitFacetReplacementLocked(actor, operation, c.At)
	o.Unlock()
	if err != nil || permitted.PermitAt == (contract.CoreReadCut{}) || token.Serial(permitted.PermitAt) <= token.Serial(c.At) {
		t.Fatal("permit milestone", permitted, err)
	}
	o.Lock()
	repeated, err := o.PermitFacetReplacementLocked(actor, operation, c.At)
	o.Unlock()
	if err != nil || repeated != permitted {
		t.Fatal("repeated permit is not idempotent", err)
	}
	fence := transferfact.FacetReplacementFence{FinalCut: 4, ManifestDigest: sha256.Sum256([]byte(manifest))}
	o.Lock()
	retained, bytes, err := o.RecordFacetReplacementFenceLocked(actor, operation, c.At, fence, manifest)
	o.Unlock()
	if err != nil || retained != fence || bytes != manifest {
		t.Fatal("fence milestone", retained, err)
	}
	o.Lock()
	retained, bytes, err = o.FacetReplacementFenceLocked(actor, operation)
	o.Unlock()
	if err != nil || retained != fence || bytes != manifest {
		t.Fatal("fence read-back lost the exact manifest bytes", retained, err)
	}
	control := func() transferfact.FacetReplacementControl {
		o.Lock()
		defer o.Unlock()
		got, err := o.FacetReplacementControlLocked(operation)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	accepted := func() transferfact.FacetReplacementControl {
		o.Lock()
		defer o.Unlock()
		got, err := o.RecordFacetReplacementAcceptanceLocked(actor, operation, c.At)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}()
	if accepted.AcceptedAt == (contract.CoreReadCut{}) || token.Serial(accepted.AcceptedAt) <= token.Serial(accepted.FencedAt) {
		t.Fatal("acceptance milestone", accepted)
	}
	committed := func() transferfact.FacetReplacementControl {
		o.Lock()
		defer o.Unlock()
		got, err := o.CommitFacetReplacementLocked(actor, operation, c.At)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}()
	switched, _ := o.state.attachments.facts.Record(seeded.entityID)
	key := values.FacetAcceptanceKey{Core: o.state.authority, Facet: target, Element: seeded.entityID, Proposal: c.Proposal}
	if switched.Attachment.State != contract.FacetActive || switched.Attachment.Key != key || switched.Pending.Valid() ||
		switched.Attachment.Revision != facetfact.AttachmentRevisionForCut(seeded.entityID, committed.CommittedAt) ||
		switched.Attachment.Revision == record.Attachment.Revision ||
		switched.NextHop != input.TargetAddress || switched.Attachment.NextHop != input.TargetAddress ||
		switched.Contact != token.FacetContactRevisionFromParts(token.Scope(committed.CommittedAt), key, token.Serial(committed.CommittedAt)) ||
		switched.Attachment.NextHopRevision != switched.Contact || switched.Attachment.OriginalProposal != (values.PublicToken{}) {
		t.Fatal("switch did not install the exact accepted key", switched)
	}
	if err := o.state.guardTransferElement(seeded.entityID); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("unswitched replacement released its element early", err)
	}
	installed := func() transferfact.FacetReplacementControl {
		o.Lock()
		defer o.Unlock()
		got, err := o.RecordFacetReplacementInstallationLocked(actor, operation, c.At)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}()
	settled := func() transferfact.FacetReplacementControl {
		o.Lock()
		defer o.Unlock()
		got, err := o.SettleFacetReplacementLocked(actor, operation, c.At)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}()
	if !settled.Terminal() || token.Serial(settled.SettledAt) <= token.Serial(installed.InstalledAt) {
		t.Fatal("settlement milestone", settled)
	}
	if err := o.state.guardTransferElement(seeded.entityID); err != nil {
		t.Fatal("settled replacement still holds its element", err)
	}
	if current := control(); !current.Terminal() || current != settled {
		t.Fatal("control read diverged", current)
	}
	o.Lock()
	result, code, err := o.ApplyFacetLocked("facet-address-after", facetfact.Binding{Actor: foundationStoreActor, Kind: facetfact.Address,
		Element: seeded.entityID, Key: key, Contact: switched.Contact, Address: "local:target-facet-2"}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	o.Unlock()
	if err != nil || code != facetfact.OK || result.Status != contract.MutationCommitted {
		t.Fatal("settled element cannot update its contact", result, code, err)
	}
	if err := validateCoreState(o.state); err != nil {
		t.Fatal("full history replay", err)
	}
	path, before := o.store.db.Path(), o.state.clone()
	closeFoundationStore(t, o)
	reopened := openFoundationStore(t, path)
	defer closeFoundationStore(t, reopened)
	if !sameCompleteCoreState(before, reopened.state) {
		t.Fatal("restart changed the replacement facts")
	}
	if err := validateCoreState(reopened.state); err != nil {
		t.Fatal("restart replay", err)
	}
}

func TestFacetReplacementCancellation(t *testing.T) {
	o, seeded, source, target := facetReplacementFixture(t)
	defer closeFoundationStore(t, o)
	record, _ := o.state.attachments.facts.Record(seeded.entityID)
	intent := func(operation string) transferfact.FacetReplacementIntent {
		return transferfact.FacetReplacementIntent{Operation: contract.OperationID(operation), Actor: foundationStoreActor, Element: seeded.entityID,
			Source: source, Target: target, SourceKey: record.Attachment.Key, SourceNextHop: record.NextHop,
			SourceAddress: "local:core", TargetAddress: "local:target-facet", Expected: record.Attachment.Revision}
	}
	actor := foundationStoreActor
	cancel := func(operation string, expected contract.CoreReadCut) (transferfact.FacetReplacementControl, error) {
		o.Lock()
		defer o.Unlock()
		return o.CancelFacetReplacementLocked(actor, contract.OperationID(operation), expected)
	}
	settle := func(operation string, expected contract.CoreReadCut) error {
		o.Lock()
		defer o.Unlock()
		_, err := o.SettleFacetReplacementLocked(actor, contract.OperationID(operation), expected)
		return err
	}

	// The original caller's free window closes when the permit is dispatched.
	free := beginFacetReplacement(t, o, intent("facet-free-cancel"))
	cancelled, err := cancel("facet-free-cancel", free.At)
	if err != nil || cancelled.CancelledAt == (contract.CoreReadCut{}) || !cancelled.Terminal() {
		t.Fatal("free cancellation", cancelled, err)
	}
	repeat, err := cancel("facet-free-cancel", free.At)
	if err != nil || repeat != cancelled {
		t.Fatal("repeated cancellation is not idempotent", err)
	}
	if err := o.state.guardTransferElement(seeded.entityID); err != nil {
		t.Fatal("cancelled replacement still holds its element", err)
	}
	if err := settle("facet-free-cancel", free.At); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("cancelled replacement still settles", err)
	}

	// After the permit only the source's durable refusal reopens cancellation.
	permitted := beginFacetReplacement(t, o, intent("facet-refused-cancel"))
	o.Lock()
	_, err = o.PermitFacetReplacementLocked(actor, "facet-refused-cancel", permitted.At)
	o.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cancel("facet-refused-cancel", permitted.At); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("network silence reopened cancellation", err)
	}
	refusal := transferfact.FacetReplacementRefusal{Source: source}
	o.Lock()
	refused, err := o.RefuseFacetReplacementLocked(actor, "facet-refused-cancel", permitted.At, refusal)
	o.Unlock()
	if err != nil || refused.Refusal != refusal {
		t.Fatal("refusal milestone", refused, err)
	}
	o.Lock()
	again, err := o.RefuseFacetReplacementLocked(actor, "facet-refused-cancel", permitted.At, refusal)
	o.Unlock()
	if err != nil || again != refused {
		t.Fatal("repeated refusal is not idempotent", err)
	}
	o.Lock()
	_, _, err = o.RecordFacetReplacementFenceLocked(actor, "facet-refused-cancel", permitted.At,
		transferfact.FacetReplacementFence{FinalCut: 9, ManifestDigest: sha256.Sum256([]byte("manifest"))}, "manifest")
	o.Unlock()
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatal("refused operation can still fence", err)
	}
	aborted, err := cancel("facet-refused-cancel", permitted.At)
	if err != nil || aborted.CancelledAt == (contract.CoreReadCut{}) || !aborted.Terminal() {
		t.Fatal("durable refusal did not reopen cancellation", aborted, err)
	}

	// After the source fence the replacement can only run to its outcome.
	final := beginFacetReplacement(t, o, intent("facet-fenced-no-abort"))
	o.Lock()
	if _, err = o.PermitFacetReplacementLocked(actor, "facet-fenced-no-abort", final.At); err != nil {
		t.Fatal(err)
	}
	if _, _, err = o.RecordFacetReplacementFenceLocked(actor, "facet-fenced-no-abort", final.At,
		transferfact.FacetReplacementFence{FinalCut: 12, ManifestDigest: sha256.Sum256([]byte("final"))}, "final"); err != nil {
		t.Fatal(err)
	}
	o.Unlock()
	if _, err = cancel("facet-fenced-no-abort", final.At); !errors.Is(err, values.PublicUnavailable) {
		t.Fatal("fenced operation still allows an abort", err)
	}
	if err := validateCoreState(o.state); err != nil {
		t.Fatal("full history replay", err)
	}
}

func TestFacetReplacementSerializesCompetingControl(t *testing.T) {
	o, seeded, source, target := facetReplacementFixture(t)
	defer closeFoundationStore(t, o)
	record, _ := o.state.attachments.facts.Record(seeded.entityID)
	input := transferfact.FacetReplacementIntent{Operation: "facet-replace", Actor: foundationStoreActor, Element: seeded.entityID,
		Source: source, Target: target, SourceKey: record.Attachment.Key, SourceNextHop: record.NextHop,
		SourceAddress: "local:core", TargetAddress: "local:target-facet", Expected: record.Attachment.Revision}
	actor := foundationStoreActor

	// The current attachment revision and source are exact admission facts,
	// checked before any replacement holds the element.
	stale := input
	stale.Operation = "facet-replace-stale"
	stale.Expected = facetfact.AttachmentRevisionForCut(seeded.entityID, token.FromParts(o.state.scope, o.state.cutSerial+1))
	o.Lock()
	_, err := o.BeginFacetReplacementLocked(stale)
	o.Unlock()
	if !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal("stale attachment revision admitted a replacement", err)
	}
	foreign := input
	foreign.Operation = "facet-replace-foreign"
	foreign.Source = foundationStoreOtherAuthority(t, source)
	foreign.SourceKey = record.Attachment.Key
	foreign.SourceKey.Facet = foreign.Source
	o.Lock()
	_, err = o.BeginFacetReplacementLocked(foreign)
	o.Unlock()
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatal("foreign source admitted a replacement", err)
	}

	// An open replacement serializes every competing control of the element.
	c := beginFacetReplacement(t, o, input)
	if !c.Valid() {
		t.Fatal("invalid control record", c)
	}
	// A wrong reservation cut is refused without consuming any new state.
	o.Lock()
	_, err = o.PermitFacetReplacementLocked(actor, input.Operation, contract.CoreReadCut{})
	o.Unlock()
	if !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatal("stale control cut still advanced", err)
	}
	// The claimed operation ID is refused across every control domain.
	o.Lock()
	_, err, found := o.LookupFacetLocked(input.Operation, facetfact.Binding{Actor: actor, Kind: facetfact.Detach,
		Element: seeded.entityID, Expected: record.Attachment.Revision})
	o.Unlock()
	if !found || !errors.Is(err, contract.ErrOperationConflict) {
		t.Fatal("replacement operation ID is not claimed across domains", err)
	}
	// Attachment control cannot interleave with the open replacement.
	o.Lock()
	_, _, err = o.ApplyFacetLocked("facet-detach-compete", facetfact.Binding{Actor: actor, Kind: facetfact.Detach,
		Element: seeded.entityID, Expected: record.Attachment.Revision}, contract.FacetAcceptanceEvidence{}, contract.FacetDispositionRead{})
	o.Unlock()
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatal("detach interleaved with the open replacement", err)
	}
	// Element lifecycle control is serialized the same way.
	entity, _ := o.state.elements.Entity(seeded.entityID)
	if _, err = applyFoundationStore(o, elementfact.OperationUpdateEntity, actor, seeded.entityID,
		foundationStoreTuple(t, seeded.anchorID, 1, 2), entity.Revision, "entity-update-compete"); !errors.Is(err, contract.ErrConflict) {
		t.Fatal("element update interleaved with the open replacement", err)
	}
	// A competing replacement for the same element is refused.
	compete := input
	compete.Operation = "facet-replace-compete"
	o.Lock()
	_, err = o.BeginFacetReplacementLocked(compete)
	o.Unlock()
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatal("two replacements hold one element", err)
	}
}
