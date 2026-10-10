package mount

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access"
	"ucupa/ucupa-core/internal/commit"
	"ucupa/ucupa-core/internal/element"
)

var mountTestActor = values.PrincipalRef{Realm: "mount-test", Principal: "operator"}

func TestMountOperationSurfaceKeepsStateAndGuardsPrivate(t *testing.T) {
	typ := reflect.TypeOf((*Operations)(nil))
	want := []string{
		"ApplyMount", "ApplyMountControl", "AuthorityRef", "AuthorizeMountAcceptance", "AuthorizeMountControl",
		"CheckMountAcceptance", "CommandCommitment", "ConfigureMountTransport", "DescribeMountApproval",
		"InspectMountContact", "InspectMountControl", "MountPeer", "PrepareApprovedMountControl",
		"PrepareMountControl", "ReadMount", "ReadMountOperation", "RecoverMounts", "RepairMountContact",
		"ResolveMountControl", "UpdateMountNextHop",
	}
	got := make([]string, typ.NumMethod())
	for i := range got {
		got[i] = typ.Method(i).Name
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected Mount operation surface: %v", got)
	}
	for i := 0; i < typ.Elem().NumField(); i++ {
		if typ.Elem().Field(i).IsExported() {
			t.Fatal("Mount exposes its private owner, authorization or connection state")
		}
	}
}

type mountTestAuthorizer struct {
	allow          bool
	decision       values.AuthorizationDecision
	validate       values.AuthorizationDecision
	seen           values.CallerContext
	required       []values.Requirement
	beforeValidate func()
}

func newMountTestAuthorizer() *mountTestAuthorizer {
	return &mountTestAuthorizer{allow: true, decision: values.AuthorizationValid, validate: values.AuthorizationValid}
}
func (a *mountTestAuthorizer) AuthorizePrincipal(values.PrincipalRef, ...values.Requirement) values.AuthorizationDecision {
	if a.allow {
		return values.AuthorizationValid
	}
	return values.AuthorizationDenied
}
func (a *mountTestAuthorizer) AuthorizeView(ctx context.Context, c values.CallerContext, r ...values.Requirement) (values.AuthorizationDecision, values.AuthorizationView) {
	a.seen, a.required = c, append([]values.Requirement(nil), r...)
	return a.decision, values.AuthorizationView{1}
}
func (a *mountTestAuthorizer) ValidateView(ctx context.Context, c values.CallerContext, _ values.AuthorizationView, r ...values.Requirement) values.AuthorizationDecision {
	if a.beforeValidate != nil {
		a.beforeValidate()
	}
	if c != a.seen || !reflect.DeepEqual(r, a.required) {
		return values.AuthorizationDenied
	}
	return a.validate
}
func (*mountTestAuthorizer) CaptureDenialView(context.Context, values.CallerContext, values.Requirement) (values.AuthorizationDecision, values.AuthorizationDenialView) {
	return values.AuthorizationValid, values.AuthorizationDenialView{}
}
func (*mountTestAuthorizer) ValidateDenialView(_ context.Context, _ values.CallerContext, view values.AuthorizationDenialView, _ values.Requirement) values.AuthorizationDecision {
	if view == (values.AuthorizationDenialView{}) {
		return values.AuthorizationDenied
	}
	return values.AuthorizationChanged
}

type mountTestCore struct {
	owner     *commit.Owner
	elements  *element.Operations
	mounts    *Operations
	auth      *mountTestAuthorizer
	admission *access.Admission
	durable   string
}

func mountTestID(n int) values.ElementID {
	return values.ElementID(fmt.Sprintf("50000000-0000-4000-8000-%012d", n))
}

func mountTestTuple(t *testing.T, parent values.ElementID, x, radius float64) corecontract.SpatialTuple {
	t.Helper()
	rotation, err := corecontract.NewRotation(1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pose, err := corecontract.NewPose(corecontract.Vec3{X: x}, rotation)
	if err != nil {
		t.Fatal(err)
	}
	rangeValue, err := corecontract.NewBoundedSphere(radius)
	if err != nil {
		t.Fatal(err)
	}
	return corecontract.SpatialTuple{Parent: parent, LocalPose: pose, Range: rangeValue}
}

func newMountTestCore(t *testing.T, durable bool) *mountTestCore {
	t.Helper()
	authority, err := values.NewFactAuthorityRef()
	if err != nil {
		t.Fatal(err)
	}
	auth := newMountTestAuthorizer()
	var owner *commit.Owner
	path := ""
	if durable {
		path = filepath.Join(t.TempDir(), "core.db")
		owner, err = commit.CreateDurable(path, authority, [32]byte{7}, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"}, nil)
	} else {
		owner, err = commit.New(authority, [32]byte{7}, corecontract.RealmInitialization{Realm: "fixture-realm", NextHop: "local:fixture"})
	}
	if err != nil {
		t.Fatal(err)
	}
	limits := values.PublicLimits{ContextTTL: time.Minute, ObservationTTL: time.Minute, ConcurrentReads: 4}
	admission := access.New(auth, owner, limits)
	c := &mountTestCore{owner: owner, auth: auth, admission: admission, durable: path}
	c.elements, c.mounts = element.New(owner, auth), New(owner, auth, admission)
	t.Cleanup(func() { _ = c.owner.CloseStore() })
	return c
}

func (c *mountTestCore) reopen(t *testing.T) *mountTestCore {
	t.Helper()
	if err := c.owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
	owner, err := commit.OpenDurable(c.durable, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.owner = owner
	c.elements, c.mounts = element.New(owner, c.auth), New(owner, c.auth, c.admission)
	return c
}

func (c *mountTestCore) closeRun(t *testing.T) {
	t.Helper()
	c.owner.Lock()
	c.owner.BeginCloseLocked()
	c.owner.Unlock()
	if err := c.owner.CloseStore(); err != nil {
		t.Fatal(err)
	}
}

type mountTestTransport struct {
	source       *mountTestCore
	endpoints    map[values.NextHop]*mountTestCore
	afterPrepare func()
}

type checkedMountTestTransport struct {
	*mountTestTransport
	check    func()
	checkErr error
	checks   int
}

func (p *mountTestTransport) target(address values.NextHop, expected values.FactAuthorityRef) (corecontract.MountPeerPort, error) {
	target := p.endpoints[address]
	if target == nil {
		return nil, corecontract.ErrUnavailable
	}
	if target.owner.AuthorityRef() != expected {
		return nil, corecontract.ErrOwnerMismatch
	}
	return target.mounts.MountPeer(), nil
}
func (p *mountTestTransport) Prepare(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, request corecontract.MountPrepareRequest) (corecontract.MountPrepared, error) {
	peer, err := p.target(address, target)
	if err != nil {
		return corecontract.MountPrepared{}, err
	}
	got, err := peer.Prepare(ctx, p.source.owner.AuthorityRef(), request)
	if p.afterPrepare != nil {
		p.afterPrepare()
	}
	return got, err
}
func (p *mountTestTransport) Finish(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, request corecontract.MountFinishRequest) (corecontract.MountClosed, error) {
	peer, err := p.target(address, target)
	if err != nil {
		return corecontract.MountClosed{}, err
	}
	return peer.Finish(ctx, p.source.owner.AuthorityRef(), request)
}
func (p *mountTestTransport) ReadDecision(ctx context.Context, address values.NextHop, target values.FactAuthorityRef, key corecontract.MountAttemptKey) (corecontract.MountDecisionEvidence, error) {
	peer, err := p.target(address, target)
	if err != nil {
		return corecontract.MountDecisionEvidence{}, err
	}
	return peer.ReadDecision(ctx, p.source.owner.AuthorityRef(), key)
}
func (p *checkedMountTestTransport) CheckApproval(_ context.Context, address values.NextHop, target values.FactAuthorityRef, _ corecontract.OperationID, _ corecontract.MountCommand) (values.PrincipalRef, error) {
	p.checks++
	if p.check != nil {
		p.check()
	}
	if p.checkErr != nil {
		return values.PrincipalRef{}, p.checkErr
	}
	if _, err := p.target(address, target); err != nil {
		return values.PrincipalRef{}, err
	}
	return mountTestActor, nil
}

type mountPublicFixture struct {
	a, b         *mountTestCore
	transport    *checkedMountTestTransport
	root, anchor values.ElementID
	command      corecontract.MountCommand
	callers      corecontract.LocalCallers
	caller       values.CallerContext
	view         values.AuthorizationView
}

func newMountPublicFixture(t *testing.T, durable bool) *mountPublicFixture {
	t.Helper()
	a, b := newMountTestCore(t, durable), newMountTestCore(t, false)
	f := &mountPublicFixture{a: a, b: b, root: mountTestID(1), anchor: mountTestID(2), caller: values.CallerContext{Subject: mountTestActor}}
	if _, err := a.elements.CreateRoot(mountTestActor, f.root, "root"); err != nil {
		t.Fatal(err)
	}
	created, err := a.elements.CreateAnchor(mountTestActor, f.anchor, mountTestTuple(t, f.root, 0, 10), "anchor")
	if err != nil {
		t.Fatal(err)
	}
	f.command = corecontract.MountCommand{Kind: corecontract.MountEstablish, MountAnchorUUID: f.anchor, ExpectedElementRevision: created.Record.Anchor.Revision, CarrierAuthority: b.owner.AuthorityRef(), CarrierNextHop: "local:b", ParentNextHop: "local:a"}
	f.callers = corecontract.LocalCallers{a.owner.AuthorityRef(): mountTestActor, b.owner.AuthorityRef(): mountTestActor}
	base := &mountTestTransport{source: a, endpoints: map[values.NextHop]*mountTestCore{"local:b": b}}
	f.transport = &checkedMountTestTransport{mountTestTransport: base}
	if err := a.mounts.ConfigureMountTransport(base); err != nil {
		t.Fatal(err)
	}
	if err := b.mounts.ConfigureMountTransport(&mountTestTransport{source: b, endpoints: map[values.NextHop]*mountTestCore{"local:a": a}}); err != nil {
		t.Fatal(err)
	}
	f.view, err = a.mounts.AuthorizeMountControl(context.Background(), f.caller, f.command)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *mountPublicFixture) publicCommand(t *testing.T) corecontract.PublicMountCommand {
	t.Helper()
	f.a.owner.RLock()
	facts := f.a.owner.FactsLocked()
	anchor, ok := facts.Elements.Anchor(f.anchor)
	if !ok {
		f.a.owner.RUnlock()
		t.Fatal("anchor missing")
	}
	record := facts.Mount.Record(f.anchor)
	signer := f.a.owner.SigningLocked()
	input := corecontract.PublicMountCommand{Kind: f.command.Kind, Element: f.anchor, ExpectedElement: signer.ElementRevision(anchor.Revision), Carrier: f.command.CarrierAuthority, CarrierNextHop: f.command.CarrierNextHop, ParentNextHop: f.command.ParentNextHop}
	if record.Revision != (corecontract.MountRevision{}) {
		input.ExpectedMount = signer.MountRevision(record.Revision)
	}
	f.a.owner.RUnlock()
	return input
}

func (f *mountPublicFixture) apply(t *testing.T, id corecontract.OperationID, command corecontract.MountCommand) corecontract.MountControlResult {
	t.Helper()
	result, err := f.a.mounts.ApplyMount(context.Background(), f.callers, id, command)
	if err != nil || result.Decision != corecontract.MountCommitted || !result.Settled {
		t.Fatalf("apply: %#v %v", result, err)
	}
	return result
}

func mountProbe(owner *commit.Owner, id corecontract.OperationID, elementID values.ElementID) any {
	owner.RLock()
	defer owner.RUnlock()
	facts := owner.FactsLocked()
	op, exists := owner.MountOperationLocked(id)
	return struct {
		Cut       corecontract.CoreReadCut
		Record    any
		Incoming  any
		Operation commit.MountOperation
		Exists    bool
	}{facts.CurrentCut(), facts.Mount.Record(elementID), facts.Mount.Incoming(), op, exists}
}
