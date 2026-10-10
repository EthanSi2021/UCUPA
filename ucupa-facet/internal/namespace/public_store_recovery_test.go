package namespace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ucupa/shared/values"
	facetcontract "ucupa/ucupa-facet/contract"
	requestvalue "ucupa/ucupa-facet/internal/namespace/request"
)

// publicStoreControlSource is a test-only fixed-address Core control port. It
// returns the already accepted ACTIVE evidence and an empty Core observation
// stream; the change under test is therefore produced only by this Facet's
// namespace commit.
type publicStoreControlSource struct {
	proposal   *acceptanceProposalSource
	attachment facetcontract.CoreEvidence
}

func (s *publicStoreControlSource) ResolveProposal(ctx context.Context, address values.NextHop, ref facetcontract.PublicAcceptanceRef) (facetcontract.CoreProposalRead, error) {
	return s.proposal.ResolveProposal(ctx, address, ref)
}

func (s *publicStoreControlSource) ReadProposal(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	if ctx == nil || address != "local:core" || key != s.proposal.resolved.Key || ctx.Err() != nil {
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	e := s.proposal.resolved.Evidence
	return e, nil
}

func (s *publicStoreControlSource) ReadAttachment(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (facetcontract.CoreEvidence, error) {
	if ctx == nil || address != "local:core" || key != s.proposal.resolved.Key || ctx.Err() != nil {
		return facetcontract.CoreEvidence{}, facetcontract.ErrUnavailable
	}
	e := s.attachment
	e.Pending = values.FacetAcceptanceKey{}
	return e, nil
}

func (s *publicStoreControlSource) ReadSuccessor(ctx context.Context, address values.NextHop, core values.FactAuthorityRef, element values.ElementID) (facetcontract.CoreSuccessorRead, error) {
	panic("unexpected successor read")
}

func (s *publicStoreControlSource) ReadControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, prepare bool) (facetcontract.CoreControlRead, error) {
	e, err := s.ReadAttachment(ctx, address, key)
	if err != nil {
		return facetcontract.CoreControlRead{}, err
	}
	r := facetcontract.CoreControlRead{Evidence: e, Proposal: s.proposal.ref.Proposal}
	if prepare {
		r.Preparation, err = values.NewPublicToken(key.Core, "observation-preparation")
		if err != nil {
			return facetcontract.CoreControlRead{}, facetcontract.ErrUnavailable
		}
	}
	return r, nil
}

func (s *publicStoreControlSource) CheckControl(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, preparation values.PublicToken) error {
	if ctx == nil || ctx.Err() != nil || address != "local:core" || key != s.proposal.resolved.Key ||
		!preparation.Valid() || preparation.Owner != key.Core || preparation.Kind != "observation-preparation" {
		return facetcontract.ErrUnavailable
	}
	return nil
}

func (s *publicStoreControlSource) ControlCall(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, raw []byte) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || address != "local:core" || key != s.proposal.resolved.Key {
		return nil, facetcontract.ErrUnavailable
	}
	value, err := values.ParseCanonicalJSON(raw, values.PublicJSONLimits())
	if err != nil {
		return nil, values.PublicInvalid
	}
	request, ok := value.(map[string]any)
	if !ok {
		return nil, values.PublicInvalid
	}
	switch request["op"] {
	case "begin-observe":
		preparation, err := requestvalue.Token(request["preparation"], "observation-preparation")
		if err != nil {
			return nil, values.PublicInvalid
		}
		if err := s.CheckControl(ctx, address, key, preparation); err != nil {
			return nil, err
		}
		cursor, err := values.NewPublicToken(key.Core, "observation-cursor")
		if err != nil {
			return nil, facetcontract.ErrUnavailable
		}
		return values.CanonicalJSONValue(map[string]any{"cursor": requestvalue.TokenJSON(cursor)})
	case "poll":
		cursor, err := requestvalue.Token(request["cursor"], "observation-cursor")
		if err != nil || cursor.Owner != key.Core {
			return nil, values.PublicInvalid
		}
		return values.CanonicalJSONValue(map[string]any{"groups": []any{}, "nextCursor": requestvalue.TokenJSON(cursor)})
	case "release-observe":
		if raw, ok := request["preparation"]; ok {
			preparation, err := requestvalue.Token(raw, "observation-preparation")
			if err != nil {
				return nil, values.PublicInvalid
			}
			if err := s.CheckControl(ctx, address, key, preparation); err != nil {
				return nil, err
			}
		}
		return values.CanonicalJSONValue(map[string]any{"released": true})
	default:
		return nil, values.PublicInvalid
	}
}

func newPublicStoreFixture(t *testing.T) (*publicAcceptanceFixture, values.FacetTypeRef, *publicStoreControlSource) {
	t.Helper()
	x := newPublicAcceptanceFixture(t, true)
	core := &publicStoreControlSource{proposal: x.source, attachment: testProof(x.key, facetcontract.CoreActive, 3)}
	x.f.cores = core
	x.accept(t, "public-store-accept")
	return x, fixtureType("public-store-recovery"), core
}

func publicStoreRequest(t *testing.T, x *publicAcceptanceFixture, request map[string]any) ([]byte, error) {
	t.Helper()
	raw, err := values.CanonicalJSONValue(request)
	if err != nil {
		t.Fatal(err)
	}
	return x.f.PublicCall(context.Background(), x.caller, raw)
}

func publicStoreCall(t *testing.T, x *publicAcceptanceFixture, request map[string]any) map[string]any {
	t.Helper()
	raw, err := publicStoreRequest(t, x, request)
	if err != nil {
		t.Fatalf("public %s: %v", request["op"], err)
	}
	value, err := values.ParseCanonicalJSON(raw, values.PublicJSONLimits())
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("public result is not an object: %T", value)
	}
	return result
}

func publicStoreAccess(x *publicAcceptanceFixture) map[string]any {
	return map[string]any{
		"key": map[string]any{
			"core":     string(x.ref.Core),
			"facet":    string(x.f.authority),
			"element":  string(x.ref.Element),
			"proposal": requestvalue.TokenJSON(x.ref.Proposal),
		},
		"nextHop": "local:core",
	}
}

func publicStoreGet(t *testing.T, x *publicAcceptanceFixture, typ values.FacetTypeRef, prepare bool) map[string]any {
	return publicStoreCall(t, x, map[string]any{
		"op":             "get",
		"access":         publicStoreAccess(x),
		"typeRef":        values.EncodePublicBytes(typ.Bytes()),
		"prepareObserve": prepare,
	})
}

func publicStoreState(t *testing.T, x *publicAcceptanceFixture, typ values.FacetTypeRef) string {
	t.Helper()
	result := publicStoreGet(t, x, typ, false)
	state, ok := result["state"].(string)
	if !ok {
		t.Fatalf("public get state is not a string: %+v", result)
	}
	return state
}

func publicStorePresent(t *testing.T, x *publicAcceptanceFixture, typ values.FacetTypeRef, payload string, revision any) map[string]any {
	t.Helper()
	result := publicStoreGet(t, x, typ, false)
	if result["state"] != "PRESENT" || result["payload"] != values.EncodePublicBytes(fixturePayload(payload)) {
		t.Fatalf("public present state: %+v", result)
	}
	if revision != nil && !reflect.DeepEqual(result["revision"], revision) {
		t.Fatalf("public revision changed: got=%v want=%v", result["revision"], revision)
	}
	return result
}
func publicStorePut(x *publicAcceptanceFixture, typ values.FacetTypeRef, id, payload string) map[string]any {
	return map[string]any{
		"op":          "put",
		"access":      publicStoreAccess(x),
		"typeRef":     values.EncodePublicBytes(typ.Bytes()),
		"expected":    map[string]any{"absent": true},
		"operationId": values.EncodePublicBytes([]byte(id)),
		"payload":     values.EncodePublicBytes(fixturePayload(payload)),
	}
}
func publicStoreStreamGroups(t *testing.T, x *publicAcceptanceFixture, raw any) int {
	t.Helper()
	token, err := requestvalue.Token(raw, "observation-preparation")
	if err != nil {
		t.Fatal(err)
	}
	x.f.mu.Lock()
	defer x.f.mu.Unlock()
	entry := x.f.public.LookupLocked(token.Value)
	if entry == nil || entry.Stream == nil {
		t.Fatal("missing observation preparation")
	}
	return len(entry.Stream.Groups)
}

func TestPublicStorePrecommitFailureDoesNotPublishChange(t *testing.T) {
	x, typ, _ := newPublicStoreFixture(t)
	if got := publicStoreState(t, x, typ); got != "NOT_FOUND" {
		t.Fatalf("precommit initial public state: %s", got)
	}
	prepared := publicStoreGet(t, x, typ, true)
	beforeCut, beforeEvents := x.f.state.Cut, len(x.f.events)
	beforeRecords := len(x.f.state.Records)
	x.f.store.failBeforeCommit = func() error { return errors.New("abort before commit") }
	request := publicStorePut(x, typ, "public-precommit-failure", "before")
	if raw, err := publicStoreRequest(t, x, request); raw != nil || !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("precommit failure: raw=%q err=%v", raw, err)
	}
	if x.f.state.Cut != beforeCut || len(x.f.events) != beforeEvents || len(x.f.state.Records) != beforeRecords {
		t.Fatal("precommit failure published namespace state")
	}
	if got := publicStoreState(t, x, typ); got != "NOT_FOUND" {
		t.Fatalf("precommit failed public state: %s", got)
	}
	if got := publicStoreStreamGroups(t, x, prepared["preparation"]); got != 0 {
		t.Fatalf("precommit failure published %d observation groups", got)
	}

	begun := publicStoreCall(t, x, map[string]any{"op": "begin-observe", "preparation": prepared["preparation"]})
	poll := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": begun["cursor"]})
	if groups, ok := poll["groups"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("precommit notification: %+v", poll)
	}
}

func TestPublicStoreResponseLossPublishesOnceAndExactRetry(t *testing.T) {
	x, typ, _ := newPublicStoreFixture(t)
	if got := publicStoreState(t, x, typ); got != "NOT_FOUND" {
		t.Fatalf("response-loss initial public state: %s", got)
	}
	prepared := publicStoreGet(t, x, typ, true)
	request := publicStorePut(x, typ, "public-response-loss", "after-save")
	beforeEvents := len(x.f.events)
	x.f.store.loseResponse = func() error { return errors.New("response lost") }
	if raw, err := publicStoreRequest(t, x, request); raw != nil || !errors.Is(err, values.PublicIndeterminate) {
		t.Fatalf("response loss: raw=%q err=%v", raw, err)
	}
	if len(x.f.events) != beforeEvents+1 || len(x.f.state.Records) != 1 {
		t.Fatal("response loss did not retain committed namespace event")
	}
	if got := publicStoreState(t, x, typ); got != "PRESENT" {
		t.Fatalf("response-loss committed public state: %s", got)
	}
	if got := publicStoreStreamGroups(t, x, prepared["preparation"]); got != 1 {
		t.Fatalf("response loss published %d groups; want one", got)
	}

	replay := publicStoreCall(t, x, request)
	if replay["status"] != "COMMITTED" || len(x.f.events) != beforeEvents+1 || publicStoreStreamGroups(t, x, prepared["preparation"]) != 1 {
		t.Fatalf("exact retry changed event or notification: %+v", replay)
	}
	begun := publicStoreCall(t, x, map[string]any{"op": "begin-observe", "preparation": prepared["preparation"]})
	first := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": begun["cursor"]})
	groups, ok := first["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("response-loss observation: %+v", first)
	}
	if replay = publicStoreCall(t, x, request); replay["status"] != "COMMITTED" || len(x.f.events) != beforeEvents+1 {
		t.Fatalf("post-poll exact retry changed event: %+v", replay)
	}
	empty := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": first["nextCursor"]})
	if groups, ok := empty["groups"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("duplicate observation after retry: %+v", empty)
	}
}

func TestPublicStoreSaveBoundaryRecoveryPublishesOnce(t *testing.T) {
	x, typ, core := newPublicStoreFixture(t)
	prepared := publicStoreGet(t, x, typ, true)
	preparation := prepared["preparation"]
	started := publicStoreCall(t, x, map[string]any{"op": "begin-observe", "preparation": preparation})
	first := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": started["cursor"]})
	if groups, ok := first["groups"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("initial observation: %+v", first)
	}
	beforeCut, beforeEvents := x.f.state.Cut, len(x.f.events)
	request := publicStorePut(x, typ, "public-after-save", "after-save")
	x.f.store.failAfterCommit = func() error { return errors.New("response after durable save") }
	if raw, err := publicStoreRequest(t, x, request); raw != nil || !errors.Is(err, values.PublicIndeterminate) {
		t.Fatalf("after-save boundary: raw=%q err=%v", raw, err)
	}
	if x.f.state.Cut == beforeCut || len(x.f.events) != beforeEvents+1 || len(x.f.state.Records) != 1 {
		t.Fatalf("recovery did not install durable commit: cut=%d events=%d records=%d", x.f.state.Cut, len(x.f.events), len(x.f.state.Records))
	}
	present := publicStorePresent(t, x, typ, "after-save", nil)
	revision := present["revision"]
	if _, err := publicStoreRequest(t, x, map[string]any{"op": "begin-observe", "preparation": preparation}); !errors.Is(err, values.PublicGap) {
		t.Fatalf("old preparation after cut change: %v", err)
	}
	if _, err := publicStoreRequest(t, x, map[string]any{"op": "poll", "cursor": started["cursor"]}); !errors.Is(err, values.PublicGap) {
		t.Fatalf("old begun cursor after cut change: %v", err)
	}
	if replay := publicStoreCall(t, x, request); replay["status"] != "COMMITTED" || len(x.f.events) != beforeEvents+1 {
		t.Fatalf("exact retry changed recovered commit: %+v", replay)
	}
	fresh := publicStoreGet(t, x, typ, true)
	freshStarted := publicStoreCall(t, x, map[string]any{"op": "begin-observe", "preparation": fresh["preparation"]})
	freshPoll := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": freshStarted["cursor"]})
	if groups, ok := freshPoll["groups"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("fresh observation after recovery: %+v", freshPoll)
	}
	old := x.f
	if err := Close(old); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDurable(x.auth, core, x.path, x.config, nil)
	if err != nil {
		t.Fatal(err)
	}
	x.f = reopened
	publicStorePresent(t, x, typ, "after-save", revision)
	if replay := publicStoreCall(t, x, request); replay["status"] != "COMMITTED" || len(x.f.events) != beforeEvents+1 {
		t.Fatalf("restart exact retry changed recovered commit: %+v", replay)
	}
	if _, err := publicStoreRequest(t, x, map[string]any{"op": "begin-observe", "preparation": preparation}); !errors.Is(err, values.PublicGap) {
		t.Fatalf("old preparation revived after restart: %v", err)
	}
	if _, err := publicStoreRequest(t, x, map[string]any{"op": "poll", "cursor": started["cursor"]}); !errors.Is(err, values.PublicGap) {
		t.Fatalf("old cursor revived after restart: %v", err)
	}
}

func TestPublicStoreSaveBoundaryRecoveryKeepsNoCutObservationStable(t *testing.T) {
	x, typ, _ := newPublicStoreFixture(t)
	base := publicStoreCall(t, x, publicStorePut(x, typ, "public-stable-base", "stable"))
	if base["status"] != "COMMITTED" {
		t.Fatalf("base put: %+v", base)
	}
	publicStorePresent(t, x, typ, "stable", base["revision"])
	prepared := publicStoreGet(t, x, typ, true)
	started := publicStoreCall(t, x, map[string]any{"op": "begin-observe", "preparation": prepared["preparation"]})
	baseline := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": started["cursor"]})
	if groups, ok := baseline["groups"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("stable baseline observation: %+v", baseline)
	}
	beforeCut, beforeEvents := x.f.state.Cut, len(x.f.events)
	unchanged := publicStorePut(x, typ, "public-stable-unchanged", "stable")
	unchanged["expected"] = map[string]any{"revision": base["revision"]}
	x.f.store.failAfterCommit = func() error { return errors.New("unchanged after durable save") }
	if raw, err := publicStoreRequest(t, x, unchanged); raw != nil || !errors.Is(err, values.PublicIndeterminate) {
		t.Fatalf("unchanged boundary: raw=%q err=%v", raw, err)
	}
	if x.f.state.Cut != beforeCut || len(x.f.events) != beforeEvents+1 {
		t.Fatalf("unchanged recovery changed cut/events: cut=%d events=%d", x.f.state.Cut, len(x.f.events))
	}
	publicStorePresent(t, x, typ, "stable", base["revision"])
	if replay := publicStoreCall(t, x, unchanged); replay["status"] != "UNCHANGED" || len(x.f.events) != beforeEvents+1 {
		t.Fatalf("unchanged exact retry: %+v", replay)
	}
	unchangedPoll := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": started["cursor"]})
	if groups, ok := unchangedPoll["groups"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("unchanged recovery gapped observation: %+v", unchangedPoll)
	}
	beforeEvents = len(x.f.events)
	rejected := publicStorePut(x, typ, "public-stable-rejected", "rejected")
	x.f.store.failAfterCommit = func() error { return errors.New("rejected after durable save") }
	if raw, err := publicStoreRequest(t, x, rejected); raw != nil || !errors.Is(err, values.PublicIndeterminate) {
		t.Fatalf("rejected boundary: raw=%q err=%v", raw, err)
	}
	if x.f.state.Cut != beforeCut || len(x.f.events) != beforeEvents+1 {
		t.Fatalf("rejected recovery changed cut/events: cut=%d events=%d", x.f.state.Cut, len(x.f.events))
	}
	publicStorePresent(t, x, typ, "stable", base["revision"])
	if raw, err := publicStoreRequest(t, x, rejected); raw != nil || !errors.Is(err, values.PublicConflict) {
		t.Fatalf("rejected exact retry: raw=%q err=%v", raw, err)
	}
	if len(x.f.events) != beforeEvents+1 {
		t.Fatalf("rejected exact retry appended event")
	}
	rejectedPoll := publicStoreCall(t, x, map[string]any{"op": "poll", "cursor": started["cursor"]})
	if groups, ok := rejectedPoll["groups"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("rejected recovery gapped observation: %+v", rejectedPoll)
	}
}

func TestPublicStoreBoundaryIndeterminateDoesNotReviveOldPreparation(t *testing.T) {
	x, typ, core := newPublicStoreFixture(t)
	prepared := publicStoreGet(t, x, typ, true)
	beforeCut, beforeEvents := x.f.state.Cut, len(x.f.events)
	if err := x.f.store.db.Close(); err != nil {
		t.Fatal(err)
	}
	request := publicStorePut(x, typ, "public-db-boundary", "closed-db")
	if raw, err := publicStoreRequest(t, x, request); raw != nil || !errors.Is(err, values.PublicIndeterminate) {
		t.Fatalf("closed-db boundary: raw=%q err=%v", raw, err)
	}
	if x.f.store.db != nil || x.f.state.Cut != beforeCut || len(x.f.events) != beforeEvents || publicStoreStreamGroups(t, x, prepared["preparation"]) != 0 {
		t.Fatal("indeterminate boundary published or retained a speculative change")
	}
	if _, err := publicStoreRequest(t, x, map[string]any{"op": "begin-observe", "preparation": prepared["preparation"]}); !errors.Is(err, values.PublicUnavailable) && !errors.Is(err, values.PublicGap) {
		t.Fatalf("closed owner stream: %v", err)
	}

	old := x.f
	reopened, err := OpenDurable(x.auth, core, x.path, x.config, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = Close(old)
	x.f = reopened
	if _, err := publicStoreRequest(t, x, map[string]any{"op": "begin-observe", "preparation": prepared["preparation"]}); !errors.Is(err, values.PublicGap) {
		t.Fatalf("old preparation revived after reopen: %v", err)
	}
}
