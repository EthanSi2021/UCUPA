package composition

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	facetcontract "ucupa/ucupa-facet/contract"
)

type facetReadStub struct {
	authority   values.FactAuthorityRef
	proposal    corecontract.FacetProposalRead
	attach      corecontract.FacetAttachmentRead
	replacement corecontract.FacetReplacementRecord
	replaceErr  error
	successor   corecontract.FacetSuccessorRead
	actor       values.PrincipalRef
	proposals   int
	attaches    int
	replaces    int
	successors  int
	onRead      func()
}

func (s *facetReadStub) AuthorityRef() values.FactAuthorityRef {
	if s == nil {
		return ""
	}
	return s.authority
}

func (s *facetReadStub) ReadFacetProposal(actor values.PrincipalRef, _ values.ElementID) (corecontract.FacetProposalRead, error) {
	s.actor, s.proposals = actor, s.proposals+1
	if s.onRead != nil {
		s.onRead()
	}
	return s.proposal, nil
}

func (s *facetReadStub) ReadFacetAttachment(actor values.PrincipalRef, _ values.ElementID) (corecontract.FacetAttachmentRead, error) {
	s.actor, s.attaches = actor, s.attaches+1
	if s.onRead != nil {
		s.onRead()
	}
	return s.attach, nil
}

func (s *facetReadStub) ReadFacetReplacementRecord(id corecontract.OperationID, element values.ElementID) (corecontract.FacetReplacementRecord, error) {
	s.replaces++
	if s.onRead != nil {
		s.onRead()
	}
	return s.replacement, s.replaceErr
}

func (s *facetReadStub) ReadFacetSuccessor(actor values.PrincipalRef, _ values.ElementID) (corecontract.FacetSuccessorRead, error) {
	s.actor, s.successors = actor, s.successors+1
	if s.onRead != nil {
		s.onRead()
	}
	return s.successor, nil
}

type facetControlStub struct {
	authority values.FactAuthorityRef
	read      corecontract.AttachmentControlRead
	checkErr  error
	caller    values.CallerContext
	key       values.FacetAcceptanceKey
	prepare   bool
	token     values.PublicToken
	reads     int
	checks    int
	onRead    func()
	onCheck   func()
}

func (s *facetControlStub) AuthorityRef() values.FactAuthorityRef {
	if s == nil {
		return ""
	}
	return s.authority
}

func (s *facetControlStub) ReadAttachmentControl(_ context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, prepare bool) (corecontract.AttachmentControlRead, error) {
	s.caller, s.key, s.prepare, s.reads = caller, key, prepare, s.reads+1
	if s.onRead != nil {
		s.onRead()
	}
	return s.read, nil
}

func (s *facetControlStub) CheckAttachmentControl(_ context.Context, caller values.CallerContext, key values.FacetAcceptanceKey, token values.PublicToken) error {
	s.caller, s.key, s.token, s.checks = caller, key, token, s.checks+1
	if s.onCheck != nil {
		s.onCheck()
	}
	return s.checkErr
}

type facetAccessStub struct {
	authority values.FactAuthorityRef
	callRaw   []byte
	caller    values.CallerContext
	request   []byte
	calls     int
	onCall    func()
}

func (s *facetAccessStub) AuthorityRef() values.FactAuthorityRef {
	if s == nil {
		return ""
	}
	return s.authority
}

func (s *facetAccessStub) PublicCall(_ context.Context, caller values.CallerContext, request []byte) ([]byte, error) {
	s.caller, s.request, s.calls = caller, append([]byte(nil), request...), s.calls+1
	if s.onCall != nil {
		s.onCall()
	}
	return append([]byte(nil), s.callRaw...), nil
}

func endpointIdentity(t *testing.T) values.FactAuthorityRef {
	t.Helper()
	ref, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

var (
	endpointActor                   = values.PrincipalRef{Realm: "facet-endpoint-test", Principal: "service"}
	endpointCaller                  = values.CallerContext{Subject: endpointActor, Proof: [32]byte{1}}
	endpointID     values.ElementID = "33000000-0000-4000-8000-000000000001"
	endpointAddr   values.NextHop   = "local:facet-core-endpoint"
)

func endpointSource(t *testing.T, connections *LocalFacetCoreConnections, caller bool) facetCoreSource {
	t.Helper()
	var source facetcontract.CoreSource
	var err error
	if caller {
		source, err = connections.ForFacetCaller(endpointCaller)
	} else {
		source, err = connections.ForFacet(endpointActor)
	}
	if err != nil {
		t.Fatal(err)
	}
	return source.(facetCoreSource)
}

func endpointKey(owner values.FactAuthorityRef) values.FacetAcceptanceKey {
	var raw [76]byte
	raw[0] = 1
	copy(raw[32:68], endpointID)
	binary.BigEndian.PutUint64(raw[68:], 1)
	revision, _ := values.NewAttachmentRevision(raw[:])
	return values.FacetAcceptanceKey{Core: owner, Element: endpointID, Facet: owner, Proposal: revision}
}

func TestLocalFacetEndpointAllowsReadWithoutControl(t *testing.T) {
	owner := endpointIdentity(t)
	key := endpointKey(owner)
	read := &facetReadStub{authority: owner,
		proposal: corecontract.FacetProposalRead{Authority: owner, Element: endpointID, State: corecontract.ExistencePresent, Attachment: corecontract.FacetAttachment{State: corecontract.FacetNone}, Pending: key},
		attach:   corecontract.FacetAttachmentRead{Authority: owner, Element: endpointID, State: corecontract.ExistencePresent, Attachment: corecontract.FacetAttachment{State: corecontract.FacetActive, Key: key}},
	}
	connections := &LocalFacetCoreConnections{}
	if err := connections.BindEndpoint(endpointAddr, LocalFacetCoreEndpoint{Read: read}); err != nil {
		t.Fatal(err)
	}
	plain := endpointSource(t, connections, false)
	proposal, err := plain.ReadProposal(context.Background(), endpointAddr, key)
	if err != nil || proposal.Authority != owner || proposal.Element != endpointID || proposal.State != facetcontract.CoreNone || proposal.Pending != key || read.actor != endpointActor {
		t.Fatalf("proposal = %#v, actor=%#v, err=%v", proposal, read.actor, err)
	}
	attachment, err := plain.ReadAttachment(context.Background(), endpointAddr, key)
	if err != nil || attachment.State != facetcontract.CoreActive || attachment.Active != key || read.actor != endpointActor {
		t.Fatalf("attachment = %#v, actor=%#v, err=%v", attachment, read.actor, err)
	}
	controlled := endpointSource(t, connections, true)
	if _, err = controlled.ReadControl(context.Background(), endpointAddr, key, true); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("missing control read = %v", err)
	}
	if err = controlled.CheckControl(context.Background(), endpointAddr, key, values.PublicToken{}); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("missing control check = %v", err)
	}
	if _, err = controlled.ControlCall(context.Background(), endpointAddr, key, []byte("request")); !errors.Is(err, values.PublicUnavailable) {
		t.Fatalf("missing control call = %v", err)
	}
}

func TestLocalFacetEndpointRejectsNilAndMismatchedPairs(t *testing.T) {
	owner, other := endpointIdentity(t), endpointIdentity(t)
	read := &facetReadStub{authority: owner}
	var nilRead *facetReadStub
	var nilControl *facetControlStub
	var nilAccess *facetAccessStub
	for _, endpoint := range []LocalFacetCoreEndpoint{
		{},
		{Read: nilRead},
		{Read: read, Control: nilControl},
		{Read: read, Access: nilAccess},
		{Read: read, Control: &facetControlStub{authority: owner}},
		{Read: read, Access: &facetAccessStub{authority: owner}},
		{Read: read, Control: &facetControlStub{authority: other}, Access: &facetAccessStub{authority: owner}},
		{Read: read, Control: &facetControlStub{authority: owner}, Access: &facetAccessStub{authority: other}},
	} {
		connections := &LocalFacetCoreConnections{}
		if err := connections.BindEndpoint(endpointAddr, endpoint); !errors.Is(err, facetcontract.ErrInvalid) {
			t.Fatalf("BindEndpoint(%#v) = %v", endpoint, err)
		}
	}
}

func TestLocalFacetControlUsesOriginalCallerAndFields(t *testing.T) {
	owner := endpointIdentity(t)
	key := endpointKey(owner)
	read := &facetReadStub{authority: owner}
	preparation, _ := values.NewPublicToken(owner, "observation-preparation")
	proposal, _ := values.NewPublicToken(owner, "attachment-revision")
	control := &facetControlStub{authority: owner, read: corecontract.AttachmentControlRead{
		Read:     corecontract.FacetAttachmentRead{Authority: owner, Element: endpointID, State: corecontract.ExistencePresent, Attachment: corecontract.FacetAttachment{State: corecontract.FacetNone}},
		Proposal: proposal, Preparation: preparation,
	}}
	access := &facetAccessStub{authority: owner, callRaw: []byte("response")}
	connections := &LocalFacetCoreConnections{}
	if err := connections.BindEndpoint(endpointAddr, LocalFacetCoreEndpoint{Read: read, Control: control, Access: access}); err != nil {
		t.Fatal(err)
	}
	source := endpointSource(t, connections, true)
	got, err := source.ReadControl(context.Background(), endpointAddr, key, true)
	if err != nil || got.Evidence.Authority != owner || got.Evidence.Element != endpointID || got.Evidence.State != facetcontract.CoreNone || got.Proposal != proposal || got.Preparation != preparation || control.caller != endpointCaller || control.key != key || !control.prepare {
		t.Fatalf("control read = %#v, stub=%#v, err=%v", got, control, err)
	}
	if err = source.CheckControl(context.Background(), endpointAddr, key, preparation); err != nil || control.caller != endpointCaller || control.key != key || control.token != preparation {
		t.Fatalf("control check stub=%#v, err=%v", control, err)
	}
	raw, err := source.ControlCall(context.Background(), endpointAddr, key, []byte("request"))
	if err != nil || !reflect.DeepEqual(raw, []byte("response")) || !reflect.DeepEqual(access.request, []byte("request")) || access.caller != endpointCaller {
		t.Fatalf("control call = %q, stub=%#v, err=%v", raw, access, err)
	}
}

func TestLocalFacetControlRebindsOutsideLockAndUsesCapturedPair(t *testing.T) {
	owner := endpointIdentity(t)
	key := endpointKey(owner)
	oldRead, newRead := &facetReadStub{authority: owner}, &facetReadStub{authority: owner}
	oldControl, newControl := &facetControlStub{authority: owner}, &facetControlStub{authority: owner}
	oldAccess := &facetAccessStub{authority: owner, callRaw: []byte("old")}
	newAccess := &facetAccessStub{authority: owner, callRaw: []byte("new")}
	connections := &LocalFacetCoreConnections{}
	if err := connections.BindEndpoint(endpointAddr, LocalFacetCoreEndpoint{Read: oldRead, Control: oldControl, Access: oldAccess}); err != nil {
		t.Fatal(err)
	}
	oldAccess.onCall = func() {
		if err := connections.BindEndpoint(endpointAddr, LocalFacetCoreEndpoint{Read: newRead, Control: newControl, Access: newAccess}); err != nil {
			t.Error(err)
		}
	}
	source := endpointSource(t, connections, true)
	done := make(chan struct{})
	var first []byte
	var firstErr error
	go func() {
		first, firstErr = source.ControlCall(context.Background(), endpointAddr, key, []byte("first"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("control call held the connection lock while invoking the port")
	}
	if firstErr != nil || string(first) != "old" || oldAccess.calls != 1 || newAccess.calls != 0 {
		t.Fatalf("captured call = %q, old=%d new=%d err=%v", first, oldAccess.calls, newAccess.calls, firstErr)
	}
	second, err := source.ControlCall(context.Background(), endpointAddr, key, []byte("second"))
	if err != nil || string(second) != "new" || newAccess.calls != 1 {
		t.Fatalf("rebound call = %q, calls=%d, err=%v", second, newAccess.calls, err)
	}
}

func TestFacetCoreSourceReadReplacementProjectsExactRecord(t *testing.T) {
	owner := endpointIdentity(t)
	record := corecontract.FacetReplacementRecord{
		Authority: owner, Element: endpointID, Operation: "replacement-read-op",
		Source: owner, Target: owner, FinalCut: 7, ManifestDigest: [32]byte{9},
		Permitted: true, Fenced: true,
	}
	read := &facetReadStub{authority: owner, replacement: record}
	connections := &LocalFacetCoreConnections{}
	if err := connections.BindEndpoint(endpointAddr, LocalFacetCoreEndpoint{Read: read}); err != nil {
		t.Fatal(err)
	}
	source := endpointSource(t, connections, false)
	got, err := source.ReadReplacement(context.Background(), endpointAddr, owner, endpointID, "replacement-read-op")
	if err != nil || got.Authority != owner || got.Element != endpointID || got.Operation != "replacement-read-op" ||
		got.Source != owner || got.Target != owner || got.FinalCut != 7 || got.ManifestDigest != [32]byte{9} ||
		!got.Permitted || !got.Fenced || got.Accepted || got.Committed || got.Settled || got.Cancelled || read.replaces != 1 {
		t.Fatalf("replacement read = %#v, stub reads=%d, err=%v", got, read.replaces, err)
	}
	other := endpointIdentity(t)
	if _, err = source.ReadReplacement(context.Background(), endpointAddr, other, endpointID, "replacement-read-op"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("wrong Core identity: got %v", err)
	}
	if _, err = source.ReadReplacement(context.Background(), "local:missing", owner, endpointID, "replacement-read-op"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("unbound address: got %v", err)
	}
	if _, err = source.ReadReplacement(context.Background(), endpointAddr, owner, endpointID, "different-op"); !errors.Is(err, facetcontract.ErrUnavailable) {
		t.Fatalf("operation mismatch: got %v", err)
	}
	if _, err = source.ReadReplacement(context.Background(), endpointAddr, owner, endpointID, ""); !errors.Is(err, facetcontract.ErrInvalid) {
		t.Fatalf("empty operation: got %v", err)
	}
}

func TestLocalFacetEndpointHidesIdentityChangesDuringCalls(t *testing.T) {
	for _, changed := range []string{"read", "replacement-read", "control-read", "control-check", "control-call"} {
		t.Run(changed, func(t *testing.T) {
			owner, other := endpointIdentity(t), endpointIdentity(t)
			key := endpointKey(owner)
			read := &facetReadStub{authority: owner, proposal: corecontract.FacetProposalRead{Authority: owner, Element: endpointID},
				replacement: corecontract.FacetReplacementRecord{Authority: owner, Element: endpointID, Operation: "identity-leak-op"}}
			control := &facetControlStub{authority: owner, read: corecontract.AttachmentControlRead{Read: corecontract.FacetAttachmentRead{Authority: owner, Element: endpointID}}}
			access := &facetAccessStub{authority: owner, callRaw: []byte("secret")}
			switch changed {
			case "read":
				read.onRead = func() { read.authority = other }
			case "replacement-read":
				read.onRead = func() { read.authority = other }
			case "control-read":
				control.onRead = func() { read.authority = other }
			case "control-check":
				control.onCheck = func() { control.authority = other }
			case "control-call":
				access.onCall = func() { access.authority = other }
			}
			connections := &LocalFacetCoreConnections{}
			if err := connections.BindEndpoint(endpointAddr, LocalFacetCoreEndpoint{Read: read, Control: control, Access: access}); err != nil {
				t.Fatal(err)
			}
			source := endpointSource(t, connections, true)
			switch changed {
			case "read":
				if out, err := source.ReadProposal(context.Background(), endpointAddr, key); !errors.Is(err, facetcontract.ErrUnavailable) || out != (facetcontract.CoreEvidence{}) {
					t.Fatalf("read leaked %#v, %v", out, err)
				}
			case "replacement-read":
				out, err := source.ReadReplacement(context.Background(), endpointAddr, owner, endpointID, "identity-leak-op")
				if !errors.Is(err, facetcontract.ErrUnavailable) || out != (facetcontract.CoreReplacementRead{}) {
					t.Fatalf("replacement read leaked %#v, %v", out, err)
				}
			case "control-read":
				if out, err := source.ReadControl(context.Background(), endpointAddr, key, false); !errors.Is(err, values.PublicUnavailable) || out != (facetcontract.CoreControlRead{}) {
					t.Fatalf("control read leaked %#v, %v", out, err)
				}
			case "control-check":
				if err := source.CheckControl(context.Background(), endpointAddr, key, values.PublicToken{}); !errors.Is(err, values.PublicUnavailable) {
					t.Fatalf("control check leaked %v", err)
				}
			case "control-call":
				if out, err := source.ControlCall(context.Background(), endpointAddr, key, []byte("request")); !errors.Is(err, values.PublicUnavailable) || out != nil {
					t.Fatalf("control call leaked %q, %v", out, err)
				}
			}
		})
	}
}
