package composition

import (
	"context"
	"errors"

	"ucupa/shared/values"
	realm "ucupa/ucupa-realm/contract"
)

// realmContactSource is the receiving owner's current durable contact in
// shared value shape, so one adapter serves both products. Disclosing it is
// trusted local assembly, not a permission check.
type realmContactSource interface {
	CurrentRealmContact() (values.RealmID, values.NextHop, error)
}

// RealmCallerSource is the connections-port face for caller establishment and
// derivation. Every call names the owner's persisted address and expected
// Realm; the local and network profiles are the two wired implementations.
// Under a network contact both calls are real remote waits, so each carries
// the original request's context for cancellation.
type RealmCallerSource interface {
	EstablishCaller(context.Context, values.NextHop, values.RealmID, realm.CredentialEvidence) (values.CallerContext, error)
	ForkCaller(context.Context, values.NextHop, values.RealmID, values.CallerContext) (values.CallerContext, error)
}

// RealmNetworkAuthentication implements the network consumer's establishment
// port. It holds no Realm object: every fork and establish reads the receiving
// owner's current durable contact and reaches the Realm only through the
// trusted connections binding. A superseded or unbound contact makes
// establishment unavailable instead of silently using an old endpoint.
type RealmNetworkAuthentication struct {
	Connections RealmCallerSource
	Owner       realmContactSource
}

// ReleaseCaller retires a temporary fork which is no longer an identity basis.
// Retained signed proofs and live sessions keep their caller until their own
// lifetime ends. Local sources have no remote allocation to release.
func (a RealmNetworkAuthentication) ReleaseCaller(ctx context.Context, caller values.CallerContext) {
	releaser, ok := a.Connections.(interface {
		ReleaseRealmEvidence(context.Context, values.NextHop, values.RealmID, [32]byte)
	})
	if !ok || ctx == nil || caller.Establishment == ([32]byte{}) {
		return
	}
	realmID, address, err := a.contact()
	if err == nil && realmID == caller.Subject.Realm {
		releaser.ReleaseRealmEvidence(context.WithoutCancel(ctx), address, realmID, caller.Establishment)
	}
}

func (a RealmNetworkAuthentication) contact() (values.RealmID, values.NextHop, error) {
	if a.Owner == nil || a.Connections == nil {
		return "", "", values.PublicUnavailable
	}
	realmID, address, err := a.Owner.CurrentRealmContact()
	if err != nil || realmID == "" || !address.Valid() {
		return "", "", values.PublicUnavailable
	}
	return realmID, address, nil
}

// ForkCaller derives a grant-specific caller without repeating or retaining
// the user's secret. Revocation is still checked by the original Realm. The
// wait honors the original request's context.
func (a RealmNetworkAuthentication) ForkCaller(ctx context.Context, caller values.CallerContext) (values.CallerContext, error) {
	realmID, address, err := a.contact()
	if err != nil {
		return values.CallerContext{}, err
	}
	fork, err := a.Connections.ForkCaller(ctx, address, realmID, caller)
	if err == nil {
		return fork, nil
	}
	if errors.Is(err, realm.ErrUnauthenticated) {
		return values.CallerContext{}, values.PublicDenied
	}
	return values.CallerContext{}, values.PublicUnavailable
}

// Establish presents the original credential to the Realm bound to the
// owner's current durable contact. The wait honors the original request's
// context.
func (a RealmNetworkAuthentication) Establish(ctx context.Context, audience values.RealmID, id [16]byte, secret [32]byte) (values.CallerContext, error) {
	realmID, address, err := a.contact()
	if err != nil {
		return values.CallerContext{}, err
	}
	caller, err := a.Connections.EstablishCaller(ctx, address, realmID, realm.CredentialEvidence{Audience: audience, ID: realm.CredentialID(id), Secret: realm.CredentialSecret(secret)})
	if err == nil {
		return caller, nil
	}
	if errors.Is(err, realm.ErrUnauthenticated) {
		return values.CallerContext{}, values.PublicDenied
	}
	return values.CallerContext{}, values.PublicUnavailable
}
