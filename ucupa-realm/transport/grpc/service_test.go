package realmgrpc

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
	admission "ucupa/ucupa-realm/transport/grpc/internal/admission"
)

const directTestPermission values.PermissionID = "ucupa.test.read"
const baseHandleTTLSeconds = 10

type directFixture struct {
	s         *Server
	clock     *fakeClock
	kernel    *realminstance.Realm
	admin     values.PrincipalRef
	user      values.PrincipalRef
	foreign   values.PrincipalRef
	userCred  realm.CredentialEvidence
	adminCred realm.CredentialEvidence
}

func (f *directFixture) userRequirement() *realmpb.Requirement {
	return coreElementRequirement(string(directTestPermission))
}

func (f *directFixture) kernelBinding(subject values.PrincipalRef) realm.PermissionBinding {
	return realm.PermissionBinding{Subject: subject, Permission: directTestPermission, Scope: values.CoreElementScope(requirementsTestOwner, requirementsTestElement)}
}

func (f *directFixture) establishUser(t *testing.T, ctx context.Context) []byte {
	t.Helper()
	out, err := f.s.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: f.userCred.ID[:],
		Secret:       f.userCred.Secret[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.GetCaller()
}

func ownerCtx(owner values.FactAuthorityRef) context.Context {
	return admission.WithOwner(context.Background(), owner)
}

func newDirectSeed(userCred, adminCred realm.CredentialEvidence) realm.Seed {
	admin := values.PrincipalRef{Realm: "realm-a", Principal: "admin"}
	user := values.PrincipalRef{Realm: "realm-a", Principal: "user"}
	foreign := values.PrincipalRef{Realm: "issuer-realm", Principal: "subject-x"}
	return realm.Seed{
		ID:              "realm-a",
		LocalPrincipals: []values.PrincipalID{admin.Principal, user.Principal},
		Bindings: []realm.PermissionBinding{
			{Subject: admin, Permission: realm.PermissionPutBinding, Scope: values.RealmScope("realm-a")},
			{Subject: admin, Permission: realm.PermissionDeleteBinding, Scope: values.RealmScope("realm-a")},
			{Subject: user, Permission: directTestPermission, Scope: values.CoreElementScope(requirementsTestOwner, requirementsTestElement)},
			{Subject: user, Permission: "ucupa.test.other", Scope: values.CoreElementScope(requirementsTestOwner, requirementsTestElement)},
			{Subject: foreign, Permission: directTestPermission, Scope: values.CoreElementScope(requirementsTestOwner, requirementsTestElement)},
		},
		Credentials: []realm.CredentialBootstrap{
			{Subject: user.Principal, Evidence: userCred},
			{Subject: admin.Principal, Evidence: adminCred},
		},
	}
}

func randomCredential(t *testing.T) realm.CredentialEvidence {
	t.Helper()
	var evidence realm.CredentialEvidence
	evidence.Audience = "realm-a"
	if _, err := rand.Read(evidence.ID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(evidence.Secret[:]); err != nil {
		t.Fatal(err)
	}
	return evidence
}

func newDirectFixture(t *testing.T, mutate func(*Config)) *directFixture {
	t.Helper()
	f := &directFixture{
		clock: newFakeClock(),
		admin: values.PrincipalRef{Realm: "realm-a", Principal: "admin"},
		user:  values.PrincipalRef{Realm: "realm-a", Principal: "user"},
	}
	f.userCred = randomCredential(t)
	f.adminCred = randomCredential(t)
	kernel, err := realminstance.New(newDirectSeed(f.userCred, f.adminCred))
	if err != nil {
		t.Fatal(err)
	}
	f.kernel = kernel
	f.foreign = values.PrincipalRef{Realm: "issuer-realm", Principal: "subject-x"}
	config := baseTestConfig()
	if mutate != nil {
		mutate(&config)
	}
	server, err := newServer(kernel, config, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Stop)
	f.s = server
	return f
}

func TestEstablishCallerDirectIssuesRunLocalHandle(t *testing.T) {
	f := newDirectFixture(t, nil)
	ctx := ownerCtx(handleTestOwner)
	out, err := f.s.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: f.userCred.ID[:],
		Secret:       f.userCred.Secret[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetSubjectRealm() != "realm-a" || out.GetSubjectPrincipal() != "user" {
		t.Fatalf("subject = %s/%s", out.GetSubjectRealm(), out.GetSubjectPrincipal())
	}
	if out.GetExpiresInSeconds() != baseHandleTTLSeconds {
		t.Fatalf("expiry = %d", out.GetExpiresInSeconds())
	}
	if len(out.GetCaller()) != 32 {
		t.Fatalf("handle length = %d", len(out.GetCaller()))
	}
	if f.s.admission.Size() != 1 {
		t.Fatalf("handles = %d", f.s.admission.Size())
	}
	entry, err := f.s.admission.Lookup([32]byte(out.GetCaller()))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Kind != admission.Caller || entry.Owner != handleTestOwner || entry.Caller.Subject != f.user {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestEstablishCallerDirectRejectsBadEvidence(t *testing.T) {
	f := newDirectFixture(t, nil)
	ctx := ownerCtx(handleTestOwner)
	_, err := f.s.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("other-realm"),
		CredentialId: f.userCred.ID[:],
		Secret:       f.userCred.Secret[:],
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong audience = %v", err)
	}
	_, err = f.s.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: f.userCred.ID[:],
		Secret:       make([]byte, 32),
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong secret = %v", err)
	}
	_, err = f.s.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: make([]byte, 16),
		Secret:       f.userCred.Secret[:],
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unknown credential = %v", err)
	}
	if f.s.admission.Size() != 0 {
		t.Fatalf("rejected evidence issued handles: %d", f.s.admission.Size())
	}
}

func TestCallerHandleStaysBoundToItsAdmittedOwner(t *testing.T) {
	f := newDirectFixture(t, nil)
	token := f.establishUser(t, ownerCtx(handleTestOwner))
	// Another admitted service cannot present the first service's reference.
	_, err := f.s.service.AuthorizeView(ownerCtx(handleOtherOwner), &realmpb.AuthorizeViewRequest{
		Caller:       token,
		Requirements: []*realmpb.Requirement{f.userRequirement()},
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("cross-owner caller = %v", err)
	}
	_, err = f.s.service.ReleaseHandle(ownerCtx(handleOtherOwner), &realmpb.ReleaseHandleRequest{Handle: token})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("cross-owner release = %v", err)
	}
	if f.s.admission.Size() != 1 {
		t.Fatalf("cross-owner misuse removed the handle: %d", f.s.admission.Size())
	}
}

func TestForkCallerDirectNeverExtendsTheParentLease(t *testing.T) {
	f := newDirectFixture(t, nil)
	ctx := ownerCtx(handleTestOwner)
	parent := f.establishUser(t, ctx)
	f.clock.advance(9 * time.Second)
	out, err := f.s.service.ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: parent})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetExpiresInSeconds() != 1 {
		t.Fatalf("derived lease = %d, want the parent's remaining second", out.GetExpiresInSeconds())
	}
	f.clock.advance(time.Second)
	for _, handle := range [][]byte{parent, out.GetCaller()} {
		if _, err := f.s.service.ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: handle}); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expired handle accepted: %v", err)
		}
	}
	// Forking an expired parent is refused the same way.
	if _, err := f.s.service.ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: parent}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expired parent fork = %v", err)
	}
}

func TestForkCallerDirectRequiresLiveKernelCaller(t *testing.T) {
	foreign := newForeignFixture(t)
	f := newDirectFixture(t, foreign.configure)
	ctx := ownerCtx(handleTestOwner)
	parent := f.establishUser(t, ctx)
	// A malformed reference cannot even be looked up.
	if _, err := f.s.service.ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: append(append([]byte{}, parent...), 0)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("wrong-size handle = %v", err)
	}
	foreignHandle := foreign.establish(t, f.s, ctx, 10*time.Second)
	// The kernel refuses to derive from a foreign identity without credentials.
	if _, err := f.s.service.ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: foreignHandle}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("foreign fork = %v", err)
	}
}

func TestReleaseHandleDirectIsIdempotent(t *testing.T) {
	f := newDirectFixture(t, nil)
	ctx := ownerCtx(handleTestOwner)
	token := f.establishUser(t, ctx)
	for i := 0; i < 2; i++ {
		if _, err := f.s.service.ReleaseHandle(ctx, &realmpb.ReleaseHandleRequest{Handle: token}); err != nil {
			t.Fatalf("release %d = %v", i, err)
		}
	}
	if f.s.admission.Size() != 0 {
		t.Fatalf("size after release = %d", f.s.admission.Size())
	}
	if _, err := f.s.service.ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: token}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("released handle accepted: %v", err)
	}
}

func TestHandleCapacityDirectRejectsWithoutEviction(t *testing.T) {
	f := newDirectFixture(t, func(c *Config) {
		c.MaxHandles = 2
		c.MaxHandlesPerCaller = 2
	})
	ctx := ownerCtx(handleTestOwner)
	first := f.establishUser(t, ctx)
	second, err := f.s.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: f.adminCred.ID[:],
		Secret:       f.adminCred.Secret[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.service.EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
		Audience:     proto.String("realm-a"),
		CredentialId: f.userCred.ID[:],
		Secret:       f.userCred.Secret[:],
	}); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("capacity = %v", err)
	}
	// The two stored references are untouched by the refusal.
	if f.s.admission.Size() != 2 {
		t.Fatalf("size = %d", f.s.admission.Size())
	}
	for _, handle := range [][]byte{first, second.GetCaller()} {
		if _, err := f.s.admission.Lookup([32]byte(handle)); err != nil {
			t.Fatalf("stored handle rejected: %v", err)
		}
	}
}
