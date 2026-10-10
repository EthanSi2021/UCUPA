package policy

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"go.etcd.io/bbolt"

	"ucupa/shared/values"
	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

var (
	credentialStoreActor = values.PrincipalRef{
		Realm:     "realm-credential-store",
		Principal: "manager",
	}
	credentialStoreOld   = testCredentialEvidence(credentialStoreActor.Realm, 0x11, 0x31)
	credentialStoreSpare = testCredentialEvidence(credentialStoreActor.Realm, 0x22, 0x42)
	credentialStoreNext  = testCredentialEvidence(credentialStoreActor.Realm, 0x33, 0x53)
)

func TestRealmCredentialRecordCodecMatchesIndependentGoldens(t *testing.T) {
	var verifier [32]byte
	for index := range verifier {
		verifier[index] = byte(index + 1)
	}

	tests := []struct {
		name   string
		record credentialRecord
		golden []byte
	}{
		{
			name: "active",
			record: credentialRecord{
				principal: "alice",
				verifier:  verifier,
				active:    true,
			},
			golden: credentialStoreIndependentRecord("alice", true, verifier),
		},
		{
			name:   "permanently revoked",
			record: credentialRecord{principal: "alice"},
			golden: credentialStoreIndependentRecord("alice", false, [32]byte{}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := encodeCredentialRecord(test.record)
			if err != nil {
				t.Fatalf("encodeCredentialRecord: %v", err)
			}
			if !bytes.Equal(encoded, test.golden) {
				t.Fatalf("encoded credential record:\n got %x\nwant %x", encoded, test.golden)
			}

			decoded, err := decodeCredentialRecord(test.golden)
			if err != nil {
				t.Fatalf("decode independent golden: %v", err)
			}
			if decoded != test.record {
				t.Fatalf("decoded credential record: got %#v, want %#v", decoded, test.record)
			}
		})
	}
}

func TestRealmCredentialOperationCodecMatchesIndependentGoldens(t *testing.T) {
	actor := values.PrincipalRef{Realm: "realm-a", Principal: "alice"}
	target := credentialStoreID(0x11)
	replacement := credentialStoreID(0x22)
	var replacementVerifier [32]byte
	for index := range replacementVerifier {
		replacementVerifier[index] = byte(0x80 + index)
	}
	localExpected := credentialStoreRevision(0x41, 7)
	changedRevision := credentialStoreRevision(0x41, 8)
	foreignExpected := credentialStoreRevision(0x52, 19)

	tests := []struct {
		name    string
		binding credentialOperationBinding
		outcome retainedCredentialOutcome
		golden  []byte
	}{
		{
			name: "Rotate changed success",
			binding: credentialOperationBinding{
				actor: actor, kind: credentialOperationRotate, target: target,
				replacement: replacement, replacementVerifier: replacementVerifier,
				expected: localExpected,
			},
			outcome: retainedCredentialOutcome{
				kind: credentialOutcomeSuccess, revision: changedRevision,
			},
			golden: credentialStoreIndependentOperation(
				1, actor, target, replacement, replacementVerifier, localExpected,
				1, &changedRevision,
			),
		},
		{
			name: "Rotate credential conflict",
			binding: credentialOperationBinding{
				actor: actor, kind: credentialOperationRotate, target: target,
				replacement: replacement, replacementVerifier: replacementVerifier,
				expected: localExpected,
			},
			outcome: retainedCredentialOutcome{kind: credentialOutcomeConflict},
			golden: credentialStoreIndependentOperation(
				1, actor, target, replacement, replacementVerifier, localExpected,
				4, nil,
			),
		},
		{
			name: "Rotate revision conflict",
			binding: credentialOperationBinding{
				actor: actor, kind: credentialOperationRotate, target: target,
				replacement: replacement, replacementVerifier: replacementVerifier,
				expected: foreignExpected,
			},
			outcome: retainedCredentialOutcome{kind: credentialOutcomeRevisionConflict},
			golden: credentialStoreIndependentOperation(
				1, actor, target, replacement, replacementVerifier, foreignExpected,
				3, nil,
			),
		},
		{
			name: "Revoke changed success",
			binding: credentialOperationBinding{
				actor: actor, kind: credentialOperationRevoke, target: target,
				expected: localExpected,
			},
			outcome: retainedCredentialOutcome{
				kind: credentialOutcomeSuccess, revision: changedRevision,
			},
			golden: credentialStoreIndependentOperation(
				2, actor, target, contract.CredentialID{}, [32]byte{}, localExpected,
				1, &changedRevision,
			),
		},
		{
			name: "Revoke no-op success",
			binding: credentialOperationBinding{
				actor: actor, kind: credentialOperationRevoke, target: target,
				expected: localExpected,
			},
			outcome: retainedCredentialOutcome{
				kind: credentialOutcomeSuccess, revision: localExpected,
			},
			golden: credentialStoreIndependentOperation(
				2, actor, target, contract.CredentialID{}, [32]byte{}, localExpected,
				1, &localExpected,
			),
		},
		{
			name: "Revoke not found",
			binding: credentialOperationBinding{
				actor: actor, kind: credentialOperationRevoke, target: target,
				expected: localExpected,
			},
			outcome: retainedCredentialOutcome{kind: credentialOutcomeNotFound},
			golden: credentialStoreIndependentOperation(
				2, actor, target, contract.CredentialID{}, [32]byte{}, localExpected,
				2, nil,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := encodeCredentialOperation(test.binding, test.outcome)
			if err != nil {
				t.Fatalf("encodeCredentialOperation: %v", err)
			}
			if !bytes.Equal(encoded, test.golden) {
				t.Fatalf("encoded credential operation:\n got %x\nwant %x", encoded, test.golden)
			}

			binding, outcome, err := decodeCredentialOperation(test.golden)
			if err != nil {
				t.Fatalf("decode independent golden: %v", err)
			}
			if binding != test.binding || outcome != test.outcome {
				t.Fatalf(
					"decoded credential operation: got %#v/%#v, want %#v/%#v",
					binding, outcome, test.binding, test.outcome,
				)
			}
		})
	}
}

func TestRealmStorePersistsExactCredentialRowsAndOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, credentialStoreSeed(), path)
	initial := owner.CredentialSetRevision()
	rotated := mustRotateCredential(
		t,
		owner,
		credentialStoreOld,
		credentialStoreOld.ID,
		credentialStoreNext,
		initial,
		"credential-store-rotate",
	)
	revoked := mustRevokeCredential(
		t,
		owner,
		credentialStoreSpare,
		credentialStoreNext.ID,
		rotated,
		"credential-store-revoke",
	)

	spareVerifier := credentialStoreIndependentVerifier(credentialStoreSpare)
	nextVerifier := credentialStoreIndependentVerifier(credentialStoreNext)
	wantCredentials := map[string][]byte{
		string(credentialStoreOld.ID[:]): credentialStoreIndependentRecord(
			credentialStoreActor.Principal, false, [32]byte{},
		),
		string(credentialStoreSpare.ID[:]): credentialStoreIndependentRecord(
			credentialStoreActor.Principal, true, spareVerifier,
		),
		string(credentialStoreNext.ID[:]): credentialStoreIndependentRecord(
			credentialStoreActor.Principal, false, [32]byte{},
		),
	}
	wantOperations := map[string][]byte{
		"credential-store-rotate": credentialStoreIndependentOperation(
			1,
			credentialStoreActor,
			credentialStoreOld.ID,
			credentialStoreNext.ID,
			nextVerifier,
			initial,
			1,
			&rotated,
		),
		"credential-store-revoke": credentialStoreIndependentOperation(
			2,
			credentialStoreActor,
			credentialStoreNext.ID,
			contract.CredentialID{},
			[32]byte{},
			rotated,
			1,
			&revoked,
		),
	}

	err := owner.state.store.db.View(func(tx *bbolt.Tx) error {
		credentials, err := atomicBucketFacts(tx.Bucket(atomicCredentialBucket))
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(credentials, wantCredentials) {
			return fmt.Errorf("credential rows:\n got %#v\nwant %#v", credentials, wantCredentials)
		}
		operations, err := atomicBucketFacts(tx.Bucket(atomicCredentialOpBucket))
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(operations, wantOperations) {
			return fmt.Errorf("credential operations:\n got %#v\nwant %#v", operations, wantOperations)
		}

		meta := tx.Bucket(atomicMetaBucket)
		if meta == nil {
			return fmt.Errorf("metadata bucket is missing")
		}
		wantGeneration := make([]byte, 8)
		binary.BigEndian.PutUint64(wantGeneration, token.Generation(revoked))
		if got := meta.Get(atomicCredentialGenKey); !bytes.Equal(got, wantGeneration) {
			return fmt.Errorf("credential generation: got %x, want %x", got, wantGeneration)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect durable credential facts: %v", err)
	}
}

func TestRealmStoreKeepsBindingAndCredentialOperationNamespacesIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, credentialStoreSeed(), path)
	bindingInitial := owner.BindingSetRevision()
	credentialInitial := owner.CredentialSetRevision()
	target := contract.PermissionBinding{
		Subject:    credentialStoreActor,
		Permission: "ucupa.test.read",
		Scope:      values.CoreElementScope(realmCoreRef, atomicElementID),
	}
	bindingCommitted := mustPutBinding(
		t, owner, credentialStoreActor, target, bindingInitial, "same-operation-bytes",
	)
	credentialCommitted := mustRotateCredential(
		t, owner, credentialStoreOld, credentialStoreOld.ID, credentialStoreNext,
		credentialInitial, "same-operation-bytes",
	)
	if owner.BindingSetRevision() != bindingCommitted ||
		owner.CredentialSetRevision() != credentialCommitted {
		t.Fatal("independent operation namespaces did not preserve both revisions")
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("close independent namespace owner: %v", err)
	}

	reopened := mustOpenDurableRealm(t, path)
	if replay := mustPutBinding(
		t, reopened, credentialStoreActor, target, bindingInitial, "same-operation-bytes",
	); replay != bindingCommitted {
		t.Fatalf("binding replay: got %#v, want %#v", replay, bindingCommitted)
	}
	if replay := mustRotateCredential(
		t, reopened, credentialStoreNext, credentialStoreOld.ID, credentialStoreNext,
		credentialInitial, "same-operation-bytes",
	); replay != credentialCommitted {
		t.Fatalf("credential replay: got %#v, want %#v", replay, credentialCommitted)
	}
}

func TestOpenDurableAcceptsCanonicalCredentialHistoryObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, credentialStoreSeed(), path)
	initial := owner.CredentialSetRevision()

	got, err := owner.RevokeCredential(
		credentialStoreOld, credentialStoreNext.ID, initial, "history-not-found",
	)
	assertCredentialMutationError(t, got, err, contract.ErrNotFound)
	rotated := mustRotateCredential(
		t, owner, credentialStoreOld, credentialStoreOld.ID, credentialStoreNext,
		initial, "history-claim-later",
	)
	got, err = owner.RotateCredential(
		credentialStoreSpare, credentialStoreSpare.ID, credentialStoreNext,
		rotated, "history-conflict",
	)
	assertCredentialMutationError(t, got, err, contract.ErrCredentialConflict)
	current := testCredentialEvidence(credentialStoreActor.Realm, 0x44, 0x64)
	later := mustRotateCredential(
		t, owner, credentialStoreSpare, credentialStoreSpare.ID, current,
		rotated, "history-change-conflict-source",
	)
	revoked := mustRevokeCredential(
		t, owner, current, credentialStoreNext.ID, later, "history-revoke",
	)
	if noop := mustRevokeCredential(
		t, owner, current, credentialStoreNext.ID, revoked, "history-noop",
	); noop != revoked {
		t.Fatalf("credential no-op: got %#v, want %#v", noop, revoked)
	}
	unknown := credentialStoreID(0x55)
	got, err = owner.RevokeCredential(current, unknown, initial, "history-revision-conflict")
	assertCredentialMutationError(t, got, err, contract.ErrRevisionConflict)
	if err := owner.Close(); err != nil {
		t.Fatalf("close canonical-history owner: %v", err)
	}

	reopened := mustOpenDurableRealm(t, path)
	got, err = reopened.RevokeCredential(
		current, credentialStoreNext.ID, initial, "history-not-found",
	)
	assertCredentialMutationError(t, got, err, contract.ErrNotFound)
	got, err = reopened.RotateCredential(
		current, credentialStoreSpare.ID, credentialStoreNext,
		rotated, "history-conflict",
	)
	assertCredentialMutationError(t, got, err, contract.ErrCredentialConflict)
	if noop := mustRevokeCredential(
		t, reopened, current, credentialStoreNext.ID, revoked, "history-noop",
	); noop != revoked {
		t.Fatalf("reopened no-op replay: got %#v, want %#v", noop, revoked)
	}
	got, err = reopened.RevokeCredential(current, unknown, initial, "history-revision-conflict")
	assertCredentialMutationError(t, got, err, contract.ErrRevisionConflict)
}

func TestOpenDurableAcceptsExplicitActiveZeroVerifier(t *testing.T) {
	fixture := createCredentialStoreFixture(t, false)
	mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
		return tx.Bucket(atomicCredentialBucket).Put(
			credentialStoreOld.ID[:],
			credentialStoreIndependentRecord(
				credentialStoreActor.Principal, true, [32]byte{},
			),
		)
	})
	opened := mustOpenDurableRealm(t, fixture.path)
	record := opened.state.credentials[credentialStoreOld.ID]
	if !record.active || record.verifier != ([32]byte{}) {
		t.Fatalf("zero verifier record: got %#v, want explicit active zero verifier", record)
	}
}

func TestOpenDurableRejectsMalformedCredentialEntriesWithoutModification(t *testing.T) {
	t.Run("wrong-length credential key", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, false)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(atomicCredentialBucket)
			record := bytes.Clone(bucket.Get(credentialStoreOld.ID[:]))
			return bucket.Put(bytes.Repeat([]byte{0x71}, len(contract.CredentialID{})-1), record)
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})

	t.Run("zero credential key", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, false)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(atomicCredentialBucket)
			record := bytes.Clone(bucket.Get(credentialStoreOld.ID[:]))
			return bucket.Put(make([]byte, len(contract.CredentialID{})), record)
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})

	t.Run("truncated active credential record", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, false)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			record := credentialStoreIndependentRecord(
				credentialStoreActor.Principal,
				true,
				credentialStoreIndependentVerifier(credentialStoreOld),
			)
			return tx.Bucket(atomicCredentialBucket).Put(
				credentialStoreOld.ID[:],
				record[:len(record)-1],
			)
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})

	t.Run("revoked credential record with trailing verifier", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, false)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			record := credentialStoreIndependentRecord(
				credentialStoreActor.Principal,
				false,
				[32]byte{},
			)
			record = append(record, bytes.Repeat([]byte{0x81}, 32)...)
			return tx.Bucket(atomicCredentialBucket).Put(credentialStoreOld.ID[:], record)
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})

	t.Run("credential names unregistered Principal", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, false)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			return tx.Bucket(atomicCredentialBucket).Put(
				credentialStoreOld.ID[:],
				credentialStoreIndependentRecord("missing", true, [32]byte{}),
			)
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})

	t.Run("nested credential bucket", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, false)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(atomicCredentialBucket)
			if err := bucket.Delete(credentialStoreOld.ID[:]); err != nil {
				return err
			}
			_, err := bucket.CreateBucket(credentialStoreOld.ID[:])
			return err
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})

	t.Run("credential operation with trailing bytes", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, true)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			bucket := tx.Bucket(atomicCredentialOpBucket)
			operation := bytes.Clone(bucket.Get([]byte("credential-store-fixture-rotate")))
			operation = append(operation, 0xff)
			return bucket.Put([]byte("credential-store-fixture-rotate"), operation)
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})

	t.Run("Rotate with impossible not-found outcome", func(t *testing.T) {
		fixture := createCredentialStoreFixture(t, false)
		mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
			operation := credentialStoreIndependentOperation(
				1,
				credentialStoreActor,
				credentialStoreOld.ID,
				credentialStoreNext.ID,
				credentialStoreIndependentVerifier(credentialStoreNext),
				fixture.initial,
				2,
				nil,
			)
			return tx.Bucket(atomicCredentialOpBucket).Put([]byte("rotate-not-found"), operation)
		})
		assertAtomicOpenRejectedUnchanged(t, fixture.path)
	})
}

func TestOpenDurableRejectsImpossibleCredentialHistoryWithoutModification(t *testing.T) {
	tests := []struct {
		name        string
		withRotate  bool
		mutateStore func(*bbolt.Tx, credentialStoreFixture) error
	}{
		{
			name:       "missing changed generation",
			withRotate: true,
			mutateStore: func(tx *bbolt.Tx, _ credentialStoreFixture) error {
				return tx.Bucket(atomicCredentialOpBucket).Delete(
					[]byte("credential-store-fixture-rotate"),
				)
			},
		},
		{
			name:       "duplicate changed generation",
			withRotate: true,
			mutateStore: func(tx *bbolt.Tx, _ credentialStoreFixture) error {
				bucket := tx.Bucket(atomicCredentialOpBucket)
				operation := bytes.Clone(bucket.Get([]byte("credential-store-fixture-rotate")))
				return bucket.Put([]byte("duplicate-credential-change"), operation)
			},
		},
		{
			name:       "current active source contradicts rotation",
			withRotate: true,
			mutateStore: func(tx *bbolt.Tx, _ credentialStoreFixture) error {
				return tx.Bucket(atomicCredentialBucket).Put(
					credentialStoreOld.ID[:],
					credentialStoreIndependentRecord(
						credentialStoreActor.Principal,
						true,
						credentialStoreIndependentVerifier(credentialStoreOld),
					),
				)
			},
		},
		{
			name: "Revoke no-op contradicts active target",
			mutateStore: func(tx *bbolt.Tx, fixture credentialStoreFixture) error {
				operation := credentialStoreIndependentOperation(
					2,
					credentialStoreActor,
					credentialStoreOld.ID,
					contract.CredentialID{},
					[32]byte{},
					fixture.initial,
					1,
					&fixture.initial,
				)
				return tx.Bucket(atomicCredentialOpBucket).Put([]byte("impossible-noop"), operation)
			},
		},
		{
			name: "Revoke not-found contradicts claimed target",
			mutateStore: func(tx *bbolt.Tx, fixture credentialStoreFixture) error {
				operation := credentialStoreIndependentOperation(
					2,
					credentialStoreActor,
					credentialStoreOld.ID,
					contract.CredentialID{},
					[32]byte{},
					fixture.initial,
					2,
					nil,
				)
				return tx.Bucket(atomicCredentialOpBucket).Put([]byte("impossible-not-found"), operation)
			},
		},
		{
			name: "Rotate conflict contradicts unclaimed replacement",
			mutateStore: func(tx *bbolt.Tx, fixture credentialStoreFixture) error {
				operation := credentialStoreIndependentOperation(
					1,
					credentialStoreActor,
					credentialStoreOld.ID,
					credentialStoreNext.ID,
					credentialStoreIndependentVerifier(credentialStoreNext),
					fixture.initial,
					4,
					nil,
				)
				return tx.Bucket(atomicCredentialOpBucket).Put([]byte("impossible-conflict"), operation)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := createCredentialStoreFixture(t, test.withRotate)
			mutateAtomicStore(t, fixture.path, func(tx *bbolt.Tx) error {
				return test.mutateStore(tx, fixture)
			})
			assertAtomicOpenRejectedUnchanged(t, fixture.path)
		})
	}
}

type credentialStoreFixture struct {
	path    string
	initial contract.CredentialSetRevision
	rotated contract.CredentialSetRevision
}

func createCredentialStoreFixture(t *testing.T, withRotation bool) credentialStoreFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "realm.db")
	owner := mustCreateDurableRealm(t, credentialStoreSeed(), path)
	fixture := credentialStoreFixture{
		path:    path,
		initial: owner.CredentialSetRevision(),
	}
	if withRotation {
		fixture.rotated = mustRotateCredential(
			t,
			owner,
			credentialStoreOld,
			credentialStoreOld.ID,
			credentialStoreNext,
			fixture.initial,
			"credential-store-fixture-rotate",
		)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("close credential Store fixture: %v", err)
	}
	return fixture
}

func credentialStoreSeed() contract.Seed {
	return contract.Seed{
		ID:              credentialStoreActor.Realm,
		LocalPrincipals: []values.PrincipalID{credentialStoreActor.Principal},
		Bindings: []contract.PermissionBinding{
			administrationBinding(
				credentialStoreActor,
				contract.PermissionPutBinding,
				credentialStoreActor.Realm,
			),
			administrationBinding(
				credentialStoreActor,
				contract.PermissionRotateOwnCredential,
				credentialStoreActor.Realm,
			),
			administrationBinding(
				credentialStoreActor,
				contract.PermissionRevokeCredential,
				credentialStoreActor.Realm,
			),
		},
		Credentials: []contract.CredentialBootstrap{
			{Subject: credentialStoreActor.Principal, Evidence: credentialStoreOld},
			{Subject: credentialStoreActor.Principal, Evidence: credentialStoreSpare},
		},
	}
}

func credentialStoreID(fill byte) contract.CredentialID {
	var id contract.CredentialID
	for index := range id {
		id[index] = fill
	}
	return id
}

func credentialStoreRevision(fill byte, generation uint64) contract.CredentialSetRevision {
	var scope [32]byte
	for index := range scope {
		scope[index] = fill
	}
	return token.Credential(scope, generation)
}

func credentialStoreIndependentRecord(
	principal values.PrincipalID,
	active bool,
	verifier [32]byte,
) []byte {
	tag := byte(2)
	if active {
		tag = 1
	}
	encoded := atomicIndependentField([]byte{tag}, []byte(principal))
	if active {
		encoded = append(encoded, verifier[:]...)
	}
	return encoded
}

func credentialStoreIndependentOperation(
	kind byte,
	actor values.PrincipalRef,
	target contract.CredentialID,
	replacement contract.CredentialID,
	replacementVerifier [32]byte,
	expected contract.CredentialSetRevision,
	outcome byte,
	result *contract.CredentialSetRevision,
) []byte {
	encoded := []byte{kind}
	encoded = atomicIndependentField(encoded, []byte(actor.Realm))
	encoded = atomicIndependentField(encoded, []byte(actor.Principal))
	encoded = append(encoded, target[:]...)
	if kind == 1 {
		encoded = append(encoded, replacement[:]...)
		encoded = append(encoded, replacementVerifier[:]...)
	}
	encoded = append(encoded, token.ScopeBytes(expected)...)
	encoded = binary.BigEndian.AppendUint64(encoded, token.Generation(expected))
	encoded = append(encoded, outcome)
	if result != nil {
		encoded = append(encoded, token.ScopeBytes(*result)...)
		encoded = binary.BigEndian.AppendUint64(encoded, token.Generation(*result))
	}
	return encoded
}

func credentialStoreIndependentVerifier(evidence contract.CredentialEvidence) [32]byte {
	mac := hmac.New(sha256.New, evidence.Secret[:])
	_, _ = mac.Write([]byte("ucupa.realm.online-credential\x00"))
	var audienceLength [4]byte
	binary.BigEndian.PutUint32(audienceLength[:], uint32(len(evidence.Audience)))
	_, _ = mac.Write(audienceLength[:])
	_, _ = mac.Write([]byte(evidence.Audience))
	_, _ = mac.Write(evidence.ID[:])
	var verifier [32]byte
	copy(verifier[:], mac.Sum(nil))
	return verifier
}
