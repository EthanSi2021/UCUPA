package realmgrpc

import (
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realminstance "ucupa/ucupa-realm/instance"
)

func authorizeRequest(caller []byte, requirements ...*realmpb.Requirement) *realmpb.AuthorizeViewRequest {
	return &realmpb.AuthorizeViewRequest{Caller: caller, Requirements: requirements}
}

func TestAuthorizeAndValidateViewDirect(t *testing.T) {
	f := newDirectFixture(t, nil)
	ctx := ownerCtx(handleTestOwner)
	caller := f.establishUser(t, ctx)
	out, err := f.s.service.AuthorizeView(ctx, authorizeRequest(caller, f.userRequirement()))
	if err != nil {
		t.Fatal(err)
	}
	if out.GetDecision() != realmpb.Decision_VALID || len(out.GetView()) != 32 {
		t.Fatalf("authorize = %v", out)
	}
	view := out.GetView()

	// The exact requirement set validates, in any wire order.
	other := coreElementRequirement("ucupa.test.other")
	both, err := f.s.service.AuthorizeView(ctx, authorizeRequest(caller, f.userRequirement(), other))
	if err != nil {
		t.Fatal(err)
	}
	validate := func(callerToken, viewToken []byte, requirements ...*realmpb.Requirement) *realmpb.ValidateViewResponse {
		t.Helper()
		response, err := f.s.service.ValidateView(ctx, &realmpb.ValidateViewRequest{Caller: callerToken, View: viewToken, Requirements: requirements})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	if got := validate(caller, view, f.userRequirement()); got.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("single validation = %v", got)
	}
	if got := validate(caller, both.GetView(), other, f.userRequirement()); got.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("order-insensitive validation = %v", got)
	}
	// A different requirement set is a handle mismatch, not a denial.
	if _, err := f.s.service.ValidateView(ctx, &realmpb.ValidateViewRequest{
		Caller: caller, View: view, Requirements: []*realmpb.Requirement{other},
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("mismatched set = %v", err)
	}
	// A forked caller handle cannot validate its parent's view.
	forked, err := f.s.service.ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: caller})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.service.ValidateView(ctx, &realmpb.ValidateViewRequest{
		Caller: forked.GetCaller(), View: view, Requirements: []*realmpb.Requirement{f.userRequirement()},
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("cross-caller view = %v", err)
	}
}

func TestDenialViewDirectIsProofOfClosureNotAllow(t *testing.T) {
	f := newDirectFixture(t, nil)
	ctx := ownerCtx(handleTestOwner)
	caller := f.establishUser(t, ctx)
	// The user holds the read grant; write is a closed branch.
	write := coreElementRequirement("ucupa.test.write")
	captured, err := f.s.service.CaptureDenialView(ctx, &realmpb.CaptureDenialViewRequest{Caller: caller, Requirement: write})
	if err != nil {
		t.Fatal(err)
	}
	if captured.GetDecision() != realmpb.Decision_DENIED || len(captured.GetDenial()) != 32 {
		t.Fatalf("denial capture = %v", captured)
	}
	// A granted requirement is reported VALID and never yields a denial view.
	granted, err := f.s.service.CaptureDenialView(ctx, &realmpb.CaptureDenialViewRequest{Caller: caller, Requirement: f.userRequirement()})
	if err != nil {
		t.Fatal(err)
	}
	if granted.GetDecision() != realmpb.Decision_VALID || granted.GetDenial() != nil {
		t.Fatalf("granted capture = %v", granted)
	}
	// A valid denial proves the branch stayed closed; it never proves an Allow.
	validated, err := f.s.service.ValidateDenialView(ctx, &realmpb.ValidateDenialViewRequest{Caller: caller, Denial: captured.GetDenial(), Requirement: write})
	if err != nil {
		t.Fatal(err)
	}
	if validated.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("denial validation = %v", validated)
	}
	authorized, err := f.s.service.AuthorizeView(ctx, authorizeRequest(caller, write))
	if err != nil {
		t.Fatal(err)
	}
	if authorized.GetDecision() != realmpb.Decision_DENIED || authorized.GetView() != nil {
		t.Fatalf("denial viewed as allow = %v", authorized)
	}
	// The denial handle is bound to its exact requirement.
	if _, err := f.s.service.ValidateDenialView(ctx, &realmpb.ValidateDenialViewRequest{
		Caller: caller, Denial: captured.GetDenial(), Requirement: f.userRequirement(),
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("cross-requirement denial = %v", err)
	}
}

func TestBindingChangesInvalidationDirect(t *testing.T) {
	f := newDirectFixture(t, nil)
	ctx := ownerCtx(handleTestOwner)
	caller := f.establishUser(t, ctx)
	binding := f.kernelBinding(f.user)
	out, err := f.s.service.AuthorizeView(ctx, authorizeRequest(caller, f.userRequirement()))
	if err != nil {
		t.Fatal(err)
	}
	view := out.GetView()

	// An unrelated binding change leaves the original view valid.
	unrelated := f.kernelBinding(values.PrincipalRef{Realm: "realm-a", Principal: "user"})
	unrelated.Permission = "ucupa.test.unrelated"
	if _, err := f.kernel.PutBinding(f.admin, unrelated, f.kernel.BindingSetRevision(), "op-unrelated"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.kernel.DeleteBinding(f.admin, unrelated, f.kernel.BindingSetRevision(), "op-unrelated-del"); err != nil {
		t.Fatal(err)
	}
	kept, err := f.s.service.ValidateView(ctx, &realmpb.ValidateViewRequest{
		Caller: caller, View: view, Requirements: []*realmpb.Requirement{f.userRequirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if kept.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("unrelated change affected the view: %v", kept)
	}

	// Revocation denies the next online validation: the current decision is
	// DENIED, distinct from a still-valid view whose content has changed.
	revision, err := f.kernel.DeleteBinding(f.admin, binding, f.kernel.BindingSetRevision(), "op-revoke")
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := f.s.service.ValidateView(ctx, &realmpb.ValidateViewRequest{
		Caller: caller, View: view, Requirements: []*realmpb.Requirement{f.userRequirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if revoked.GetDecision() != realmpb.Decision_DENIED {
		t.Fatalf("revoked view = %v", revoked)
	}
	if denied := f.sAuthorizeDecision(t, caller); denied != realmpb.Decision_DENIED {
		t.Fatalf("revoked authorize = %v", denied)
	}

	// Re-granting the same permission does not revive the old view (no ABA).
	if _, err := f.kernel.PutBinding(f.admin, binding, revision, "op-regrant"); err != nil {
		t.Fatal(err)
	}
	after, err := f.s.service.ValidateView(ctx, &realmpb.ValidateViewRequest{
		Caller: caller, View: view, Requirements: []*realmpb.Requirement{f.userRequirement()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.GetDecision() != realmpb.Decision_CHANGED {
		t.Fatalf("regranted old view = %v", after)
	}
	// A fresh authorization succeeds with a new view.
	refreshed, err := f.s.service.AuthorizeView(ctx, authorizeRequest(caller, f.userRequirement()))
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.GetDecision() != realmpb.Decision_VALID || string(refreshed.GetView()) == string(view) {
		t.Fatalf("regranted authorize = %v", refreshed)
	}
}

func (f *directFixture) sAuthorizeDecision(t *testing.T, caller []byte) realmpb.Decision {
	t.Helper()
	out, err := f.s.service.AuthorizeView(ownerCtx(handleTestOwner), authorizeRequest(caller, f.userRequirement()))
	if err != nil {
		t.Fatal(err)
	}
	return out.GetDecision()
}

func TestUnavailableDecisionIsSeparatedFromDenialDirect(t *testing.T) {
	userCred := randomCredential(t)
	adminCred := randomCredential(t)
	durable, err := realminstance.CreateDurable(newDirectSeed(userCred, adminCred), filepath.Join(t.TempDir(), "realm.db"))
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(durable, baseTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	ctx := ownerCtx(handleTestOwner)
	caller, err := server.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: userCred.ID[:],
		Secret:       userCred.Secret[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := server.service.AuthorizeView(ctx, authorizeRequest(caller.GetCaller(), coreElementRequirement(string(directTestPermission))))
	if err != nil {
		t.Fatal(err)
	}
	if authorized.GetDecision() != realmpb.Decision_VALID {
		t.Fatalf("pre-retirement authorize = %v", authorized)
	}
	// Retire the durable owner: every decision must become UNAVAILABLE, which
	// stays distinct from a proven DENIED on the wire.
	if err := durable.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := server.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: userCred.ID[:],
		Secret:       userCred.Secret[:],
	}); status.Code(err) != codes.Unavailable {
		t.Fatalf("retired establish = %v", err)
	}
	out, err := server.service.AuthorizeView(ctx, authorizeRequest(caller.GetCaller(), coreElementRequirement(string(directTestPermission))))
	if err != nil {
		t.Fatal(err)
	}
	if out.GetDecision() != realmpb.Decision_UNAVAILABLE {
		t.Fatalf("retired authorize = %v", out)
	}
	validate, err := server.service.ValidateView(ctx, &realmpb.ValidateViewRequest{
		Caller: caller.GetCaller(), View: authorized.GetView(), Requirements: []*realmpb.Requirement{coreElementRequirement(string(directTestPermission))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if validate.GetDecision() != realmpb.Decision_UNAVAILABLE {
		t.Fatalf("retired validate = %v", validate)
	}
	captured, err := server.service.CaptureDenialView(ctx, &realmpb.CaptureDenialViewRequest{
		Caller: caller.GetCaller(), Requirement: coreElementRequirement("ucupa.test.write"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.GetDecision() != realmpb.Decision_UNAVAILABLE || captured.GetDenial() != nil {
		t.Fatalf("retired denial capture = %v", captured)
	}
}
