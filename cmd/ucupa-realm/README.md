# Realm network product host

`go run ./cmd/ucupa-realm -init <bootstrap.json> -store <path>` performs one explicit trusted
bootstrap and creates a new durable Realm Store.
`go run ./cmd/ucupa-realm -config <file>` opens an initialized Store as a fixed network service.
The program does not automatically seed Principals, grant permissions, or treat a missing Store
as a successful reopen. `-init`/`-config` are mutually exclusive.
Initialization prints RealmID only, never credential secrets.

Bootstrap is at most 64 KiB, with up to 256 Principals, bindings, and credentials each.
Unknown fields are rejected. Every scope kind accepts only its exact fields.
Facet keys are 32-byte lowercase hex; credential ID/secret are 16/32-byte lowercase hex:

```json
{
  "realm_id": "realm-a",
  "principals": ["admin", "user"],
  "bindings": [
    {"subject": {"realm": "realm-a", "principal": "admin"},
     "permission": "ucupa.realm.binding.put",
     "scope": {"kind": "realm", "realm": "realm-a"}},
    {"subject": {"realm": "realm-a", "principal": "user"},
     "permission": "ucupa.test.read",
     "scope": {"kind": "core_element", "authority": "<FactAuthorityRef UUID>",
               "element": "<ElementID UUID>"}}
  ],
  "credentials": [
    {"subject": "user", "audience": "realm-a",
     "id": "<16 bytes as lowercase hex>", "secret": "<32 bytes as lowercase hex>"}
  ]
}
```

Scope kinds are `core_element`/`facet_element`/`anchor_closure` (authority+element),
`facet_key` (authority+element+facet), `realm`, and `authority`.
Binding subject realm must equal realm_id and the Principal must be registered.
Credential audience must equal realm_id. This is a bootstrap file, not remote permission
administration; running-service binding changes use the original Realm administration API.

Replace runtime paths/identities with actual values and explicitly fill every duration/capacity;
zero values supply no defaults:

```json
{
  "realm_store": "realm.db",
  "realm_id": "realm-a",
  "listen": "127.0.0.1:8445",
  "next_hop": "https://127.0.0.1:8445",
  "certificate_file": "realm.pem",
  "private_key_file": "realm.key",
  "client_roots_file": "client-roots.pem",
  "admitted_services": [
    {"spki_sha256": "<32-byte SPKI SHA-256 as lowercase hex>",
     "owner": "<FactAuthorityRef UUID>"}
  ],
  "handle_ttl": "10s",
  "challenge_ttl": "10s",
  "rpc_deadline": "5s",
  "handshake_timeout": "1s",
  "connection_lifetime": "30s",
  "shutdown_timeout": "2s",
  "max_handles": 128,
  "max_handles_per_caller": 16,
  "max_challenges": 64,
  "max_challenges_per_owner": 16,
  "max_requirements": 16,
  "max_connections": 64,
  "max_concurrent_rpcs": 32,
  "max_response_bytes": 1048576,
  "max_streams": 32,
  "max_metadata_bytes": 8192
}
```

`realm_id` must match the actual Store or startup fails.
`handle_ttl`/`challenge_ttl` use whole seconds; `connection_lifetime` cannot be shorter
than `rpc_deadline`. `admitted_services` binds every client certificate SPKI to one fact owner.
mTLS service admission and user fact permission are separate: an unbound service cannot
establish callers or obtain authorization. Client roots are one explicit PEM bundle, at most
256 KiB; empty/unparseable trust fails.

Optional `foreign` controls foreign identity establishment. Without it,
`EstablishForeignCaller` always returns unavailable, fail-closed.
If present it must be complete or startup fails. Each trusted issuer supplies an Ed25519 PEM
signing public key, pinned issuer Core (`next_hop`, `expected_core`, `ca_file`,
lowercase-hex `spki_sha256`), and this service's delegation identity at that Core (`verifier`):

```json
{
  "foreign": {
    "max_proof_ttl": "30s",
    "issuers": {
      "identity-home": {
        "signing_public_key_file": "issuer-signing.pub",
        "next_hop": "https://127.0.0.1:8446",
        "expected_core": "<issuer Core FactAuthorityRef UUID>",
        "ca_file": "issuer-core-ca.pem",
        "spki_sha256": "<issuer Core service SPKI SHA-256 as lowercase hex>",
        "verifier": "<this service's delegation FactAuthorityRef UUID>"
      }
    }
  }
}
```

Online liveness reuses the network contract's existing P6 proof check.
The receiver connects to issuer Core with its own service certificate, which must include
clientAuth EKU. Issuer read_delegation pins that certificate's SPKI and maps it to `verifier`.
Explicit proof rejection is unauthenticated; unreachable issuer or incomplete verification
is unavailable. There is no local/offline Allow or extended proof/challenge lifetime.
Up to 256 issuers are trusted. Cross-product evidence is in
`acceptance/realm_foreign_process_test.go`.

`EstablishForeignCaller.actor` is the relay Core bound by the original P6 signature,
supplied after the receiving Core authenticates service identity. It is not the receiving
service owner. Realm checks the signature against actor; mTLS `admitted_services` still
determines the receiver owner, which binds challenge/result handle.
Missing actor or signature mismatch is rejected. No old-message fallback or self-reported
Principal establishment is accepted.

Service handles are finite run-local references bound to RealmID, run, receiver owner,
and original caller. They cannot cross owner/caller/Realm/run, are not persisted, and all
expire on Realm restart. Every operation verifies online; a handle is not offline Allow.

SDK target verification precedes credentials. Before `CheckTarget` succeeds, no user
credential or protected authorization body is sent. Missing/failed verification or a prior
success on a rebuilt connection cannot authorize sending.

### Verification

Independent product startup/shutdown, identity, and run-local handle tests:

```powershell
go test ./cmd/ucupa-realm -count=1 -v
```

Tests build the actual product binary and exercise SDK over TCP/TLS: bootstrap, duplicate
bootstrap refusal, CheckTarget, caller establishment, view checks, RealmID after forced
restart, invalid old caller/view, valid fresh caller, and fail-closed
`EstablishForeignCaller` without foreign configuration.

Foreign identity across one ucupa-core issuer and one ucupa-realm receiver:

```powershell
go test ./acceptance -run TestRealmProductForeignIdentityAcrossProcesses -count=1 -v
```

Loopback is not real multi-machine or production validation.
Full repository delivery still requires `go run ./cmd/verify`.
