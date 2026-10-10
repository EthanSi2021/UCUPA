# Facet gRPC product host

`go run ./cmd/ucupa-facet -config <file>` opens an initialized Facet Store and contacts its
original Realm using an explicit local/network profile. Local opens an independent Realm Store;
network opens none. The program creates no facts, users, credentials, permissions, or attachments.
It listens only after identity/configuration checks. Runtime configuration is not spatial truth
and creates no routing system or RealmAttachment.

Replace example paths/identities with actual values; supply all runtime durations/capacities:

```json
{
  "listen": "127.0.0.1:8444",
  "next_hop": "https://127.0.0.1:8444",
  "expected_facet": "00000000-0000-4000-8000-000000000001",
  "realm_id": "facet-realm",
  "realm_next_hop": "local:facet-realm",
  "facet_store": "facet.db",
  "realm_store": "facet-realm.db",
  "certificate_file": "facet.pem",
  "private_key_file": "facet.key",
  "session_ttl": "30s",
  "rpc_deadline": "5s",
  "handshake_timeout": "1s",
  "connection_lifetime": "1m",
  "shutdown_timeout": "2s",
  "max_sessions": 128,
  "max_sessions_per_caller": 8,
  "max_connections": 64,
  "max_concurrent_rpcs": 32,
  "max_streams": 32,
  "max_metadata_bytes": 8192,
  "max_response_bytes": 4194304,
  "context_ttl": "1m",
  "observation_ttl": "1m",
  "core_connections": [{
    "next_hop": "https://127.0.0.1:8443",
    "expected_core": "00000000-0000-4000-8000-000000000002",
    "ca_file": "core-ca.pem",
    "spki_sha256": "<32-byte SHA-256 as unpadded base64url>",
    "credential_file": "core-service-credential.json"
  }]
}
```

`realm_id`/`realm_next_hop` identify the opened authorization Realm and local contact.
The Facet Store's persistent contact must match exactly; configuration detects misassembly,
not overrides or missing-contact reconstruction.

For the S2P3c network profile, omit `realm_store`/`realm_next_hop` and use:

```json
{
  "realm_profile": "network",
  "realm_endpoints": [{
    "next_hop": "https://realm.example:8445",
    "ca_file": "realm-ca.pem",
    "spki_sha256": "<32-byte SHA-256 as unpadded base64url>",
    "service_certificate_file": "facet-realm-service.pem",
    "service_private_key_file": "facet-realm-service.key"
  }]
}
```

`realm_id` still asserts the original Store's RealmID. Up to 256 endpoints each fix an address,
Realm TLS pin, and this Facet's mTLS service certificate. An endpoint must match the persistent
address; Realm explicitly admits that certificate for this Facet owner.
Target verification is not service admission. Configuration cannot override contacts, mix
profiles, or fall back after failure.

Ordinary requests authorize online through their original ctx and recheck the view after Core
reads. The final fact boundary does not wait for network I/O.
Contact maintenance uses `instance.OpenRealmMaintenance`, an established CallerContext,
and the original exclusive Store. An explicit same-Realm candidate is permitted only when
the original address is unavailable; denial does not justify another authorization source.

Optional `public_max_contexts`, `public_max_contexts_per_caller`,
`public_max_groups_per_observation`, `public_max_bytes`, and
`public_max_concurrent_reads` map to the original Facet PublicAccessPolicy without changing it.
Host configuration is at most 64 KiB; Core contacts at most 256. Excess fails startup,
without truncation.

The fixed-service credential file has exactly three fields:

```json
{"audience":"core-realm","credential_id":"<16 bytes as base64url>","secret":"<32 bytes as base64url>"}
```

This is the service identity at the target Core, not a content user's credential.
The Core host needs corresponding
`facet_services:[{"realm":"core-realm","principal":"facet-service","facet":"<Facet UUID>"}]`.
Realm separately grants attachment-read, proposal-read, and required attachment-observation
permissions on exact Elements. Binding grants no permission.
Users authenticate separately to Facet; its Realm authorizes content operations.

Reverse contacts use only the relation's specified address, matching configured client address,
Core identity, and TLS pin. Unknown address, wrong identity, offline Core, or insufficient
permission does not fall back to a local object or imply absence.
Service calls read the local credential file as needed. A valid session avoids another login;
expiry or changed CredentialID establishes a new session without automatic business retries.
A new session cannot continue old preparation/cursor; old observation follows GAP rules.
After early target restart an old session may fail first. Explicit rejection clears it;
only a later explicit call logs in again. Failed requests are not retried unconditionally,
and old observation does not switch sessions.

Network relationship establishment is Core user Propose → Facet user
`Relationships().AcceptAttachment` with public proposal, specified Core address, and operation
ID → Core user Activate. First acceptance checks the current Core proposal with the fixed
service identity; activation reverse-reads Facet acceptance evidence.

For reverse service reads, configure `core_services` on Facet:

```json
{"core_services":[{"realm":"facet-realm","principal":"core-service","core":"00000000-0000-4000-8000-000000000002"}]}
```

This binding limits the Core the service can check; it does not replace Realm's
`ucupa.facet.element.acceptance-read` grant.
Core needs corresponding `facet_connections`; see the [Core host](../ucupa-core/README.md).
`Relationships().InspectAcceptance`/`UpdateCoreContact` use public contact tokens and can
repair an offline Core's address. Relation writes/retries still check original actor, command,
and CAS. Content interfaces accept no private service evidence.

### Single-writer handoff

P7.1 Facet `writer` fields, 32-byte private installation file, standard-base64 operation ID,
and stopping the old process before exclusive Store open follow
[Core handoff](../ucupa-core/README.md#single-writer-handoff).
The subject is this Facet's original Store/writer, not Core's Store or authorization.
Its persistent Realm contact constrains authorization identity.
Trusted initialization persists the initial switch; the host neither enables handoff
automatically nor repairs old formats.

SDK `Writer().InspectWriter/HandoffWriter` requires separate
`ucupa.facet.writer.inspect`/`ucupa.facet.writer.handoff` grants on this Facet AuthorityScope;
content permissions do not substitute. Original content, acceptance, contacts, signing material,
and outcomes remain. Old sessions and in-flight Core-check reads must stop disclosing.
Control does not automatically take over, copy a database, or change Realm authorization/contacts.
Current namespace replacement and remote Realm profiles reuse transfer/S2P3 mechanisms,
not second writer or authorization kernels.

### Verification

Independent product tests:

```powershell
go test ./acceptance -run TestFacetAndCoreIndependentProducts -count=1 -v
go test ./acceptance -run TestFacetRelationshipIndependentProducts -count=1 -v
go test ./acceptance -run TestWriterFacetIndependentProducts -count=1 -v
go test ./acceptance -run 'TestFacet(Relationship|Lifecycle|Replacement)NetworkRealmProducts' -count=1 -v
```

The first two cover preestablished content and network establishment from an empty relation,
reads/writes, bidirectional offline contact repair, both restarts, and post-detach refusal.
The third covers original-Store writer handoff, designated installation, and original outcomes.
They use independent Realm/Stores and actual TCP/TLS, closing initialization objects before
starting product binaries. They do not prove real multi-machine operation, automatic federation,
or a second implementation's interoperability.

The final command covers three additional same-machine combinations of independent Realm/Facet
products: relationship content, recovery/retirement, and content transfer with independent
source/target Realms. Those product processes hold no Realm kernel objects.
Full repository delivery still requires `go run ./cmd/verify`.
