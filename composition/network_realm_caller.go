package composition

import (
	"context"
	"math"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
	realm "ucupa/ucupa-realm/contract"
	realmsdk "ucupa/ucupa-realm/sdk/go"
)

// EstablishCaller presents the original credential to the pinned Realm and
// retains the returned run-local handle in the caller's Establishment slot.
// The handle is never persisted; its loss after a Realm restart is an
// authentication failure, not data loss. The wait honors the original
// request's context.
func (connections *NetworkRealmConnections) EstablishCaller(ctx context.Context, address values.NextHop, expected values.RealmID, evidence realm.CredentialEvidence) (values.CallerContext, error) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.CallerContext{}, err
	}
	var response *realmpb.EstablishCallerResponse
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.IdentityService().EstablishCaller(ctx, &realmpb.EstablishCallerRequest{
			Audience: proto.String(string(evidence.Audience)), CredentialId: evidence.ID[:], Secret: evidence.Secret[:],
		})
		response = result
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return values.CallerContext{}, ErrNetworkRealmUnavailable
	}
	if invokeErr != nil {
		return values.CallerContext{}, networkIdentityError(invokeErr)
	}
	handle, ok := networkHandle(response.GetCaller())
	if !ok {
		return values.CallerContext{}, realm.ErrUnauthenticated
	}
	subject := values.PrincipalRef{Realm: values.RealmID(response.GetSubjectRealm()), Principal: values.PrincipalID(response.GetSubjectPrincipal())}
	if !subject.Valid() {
		return values.CallerContext{}, realm.ErrUnauthenticated
	}
	return values.CallerContext{Subject: subject, Credential: evidence.ID, Establishment: handle, Proof: handle}, nil
}

// ForkCaller derives one fresh handle from the caller's current handle at the
// Realm itself. The fork keeps the original subject and credential; only the
// run-local establishment changes, so forked handles die with the run too.
// The wait honors the original request's context.
func (connections *NetworkRealmConnections) ForkCaller(ctx context.Context, address values.NextHop, expected values.RealmID, caller values.CallerContext) (values.CallerContext, error) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.CallerContext{}, err
	}
	if caller.Establishment == ([32]byte{}) {
		return values.CallerContext{}, realm.ErrUnauthenticated
	}
	var response *realmpb.ForkCallerResponse
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.IdentityService().ForkCaller(ctx, &realmpb.ForkCallerRequest{Caller: caller.Establishment[:]})
		response = result
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return values.CallerContext{}, ErrNetworkRealmUnavailable
	}
	if invokeErr != nil {
		return values.CallerContext{}, networkIdentityError(invokeErr)
	}
	handle, ok := networkHandle(response.GetCaller())
	if !ok {
		return values.CallerContext{}, realm.ErrUnauthenticated
	}
	return values.CallerContext{Subject: caller.Subject, Credential: caller.Credential, Establishment: handle, Proof: handle}, nil
}

// BeginChallenge obtains one single-use identity challenge from the pinned
// Realm. The Realm binds it to this Core's authenticated service owner; the
// composition registers the same value in its own request-bound registry, so
// one proof stays single-use at both ends and never outlives either expiry.
func (connections *NetworkRealmConnections) BeginChallenge(ctx context.Context, address values.NextHop, expected values.RealmID) ([32]byte, time.Time, error) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return [32]byte{}, time.Time{}, err
	}
	var response *realmpb.BeginChallengeResponse
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.IdentityService().BeginChallenge(ctx, &realmpb.BeginChallengeRequest{})
		response = result
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return [32]byte{}, time.Time{}, ErrNetworkRealmUnavailable
	}
	if invokeErr != nil {
		return [32]byte{}, time.Time{}, networkIdentityError(invokeErr)
	}
	challenge, ok := networkHandle(response.GetChallenge())
	if !ok || challenge == ([32]byte{}) {
		return [32]byte{}, time.Time{}, realm.ErrStore
	}
	seconds := response.GetExpiresInSeconds()
	if seconds == 0 || seconds > uint64(math.MaxInt64/int64(time.Second)) {
		return [32]byte{}, time.Time{}, realm.ErrStore
	}
	return challenge, time.Now().Add(time.Duration(seconds) * time.Second), nil
}

// EstablishForeignCaller presents one P6 proof and its challenge to the pinned
// Realm. The Realm runs the whole foreign chain there — signature, its own
// single-use challenge, configured issuer trust, online liveness and the
// post-wait expiry recheck — and returns the verified subject with a fresh
// run-local handle whose lease never exceeds the signed claim.
func (connections *NetworkRealmConnections) EstablishForeignCaller(ctx context.Context, address values.NextHop, expected values.RealmID, actor values.FactAuthorityRef, payload, signature []byte, challenge [32]byte) (values.CallerContext, time.Time, error) {
	endpoint, err := connections.selectEndpoint(address, expected)
	if err != nil {
		return values.CallerContext{}, time.Time{}, err
	}
	if !actor.Valid() || len(payload) == 0 || len(payload) > 4096 || len(signature) != 64 || challenge == ([32]byte{}) {
		return values.CallerContext{}, time.Time{}, realm.ErrUnauthenticated
	}
	var response *realmpb.EstablishForeignCallerResponse
	invokeErr := connections.invoke(ctx, endpoint, func(ctx context.Context, client *realmsdk.Client) error {
		result, callErr := client.IdentityService().EstablishForeignCaller(ctx, &realmpb.EstablishForeignCallerRequest{
			Actor: proto.String(string(actor)), Payload: payload, Signature: signature, Challenge: challenge[:],
		})
		response = result
		return callErr
	})
	if !connections.stillBound(address, expected, endpoint) {
		return values.CallerContext{}, time.Time{}, ErrNetworkRealmUnavailable
	}
	if invokeErr != nil {
		return values.CallerContext{}, time.Time{}, networkIdentityError(invokeErr)
	}
	handle, ok := networkHandle(response.GetCaller())
	if !ok {
		return values.CallerContext{}, time.Time{}, realm.ErrUnauthenticated
	}
	subject := values.PrincipalRef{Realm: values.RealmID(response.GetSubjectRealm()), Principal: values.PrincipalID(response.GetSubjectPrincipal())}
	if !subject.Valid() {
		return values.CallerContext{}, time.Time{}, realm.ErrUnauthenticated
	}
	seconds := response.GetExpiresInSeconds()
	if seconds == 0 || seconds > uint64(math.MaxInt64/int64(time.Second)) {
		return values.CallerContext{}, time.Time{}, realm.ErrUnauthenticated
	}
	return values.CallerContext{Subject: subject, Establishment: handle, Proof: handle}, time.Now().Add(time.Duration(seconds) * time.Second), nil
}

// TrustLocalCaller has no network counterpart: the network Realm establishes
// callers only from credentials or foreign proofs, never from a trusted local
// subject name. The network profile fails closed.
func (connections *NetworkRealmConnections) TrustLocalCaller(values.NextHop, values.RealmID, values.PrincipalRef) (values.CallerContext, error) {
	return values.CallerContext{}, realm.ErrUnauthenticated
}

// networkIdentityError maps identity RPC failures onto the local kernel's
// authentication error shapes: an unknown, expired or rejected handle is one
// rejection, and any transport failure stays an availability failure.
func networkIdentityError(err error) error {
	if code := status.Code(err); code == codes.Unauthenticated || code == codes.InvalidArgument || code == codes.NotFound {
		return realm.ErrUnauthenticated
	}
	return realm.ErrStore
}
