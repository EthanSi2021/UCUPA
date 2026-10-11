# Core gRPC product host

This program hosts a durable Core over the network; it is not another fact module.
The [network contract](../../docs/architecture/network-access-contract.md) defines protocol,
authentication, and failure semantics. The [plan](../../docs/architecture/development-plan.md)
records accepted delivery and validation limits. The host provides single-Core public operations,
optional restricted read delegation, mount/Facet control, and, when configured, multi-source
query/pagination, path reads, listing, and observation along mounts. Supported lifecycle services
are described below. Explicitly authenticated typed service-evidence ports expose neither
local Go objects nor Realm administration.

## Startup

Explicitly initialize independent Core/Realm Stores, Principals, credentials, and grants through
the owner APIs before starting. The service opens Stores only: no new database, automatic
permission seed, or identity replacement. Host configuration contains no caller secret.
Deployment must protect private keys, Stores, and configuration; never commit production secrets.

```powershell
go build -o ucupa-core.exe ./cmd/ucupa-core
./ucupa-core.exe -config C:/private/ucupa/core.json
```

This is a shape example, not production defaults. Replace paths, identities, and addresses
with initialized values. Paths resolve from process cwd; absolute paths are recommended.
Listen and advertised addresses may differ, but the advertised address cannot be `0.0.0.0`.

```json
{
  "listen": "127.0.0.1:8443",
  "next_hop": "https://127.0.0.1:8443",
  "expected_core": "00000000-0000-4000-8000-000000000001",
  "realm_id": "my-realm",
  "realm_next_hop": "local:my-realm",
  "core_store": "C:/private/ucupa/core.db",
  "realm_store": "C:/private/ucupa/realm.db",
  "certificate_file": "C:/private/ucupa/server.pem",
  "private_key_file": "C:/private/ucupa/server.key",
  "session_ttl": "30s",
  "rpc_deadline": "5s",
  "handshake_timeout": "1s",
  "connection_lifetime": "1m",
  "shutdown_timeout": "3s",
  "max_sessions": 16,
  "max_sessions_per_caller": 8,
  "max_connections": 16,
  "max_concurrent_rpcs": 8,
  "max_streams": 8,
  "max_metadata_bytes": 8192,
  "max_response_bytes": 8388608,
  "context_ttl": "1m",
  "observation_ttl": "1m"
}
```

- `realm_profile`: omitted/`local` uses the local kernel; `network` uses a pinned Realm
  service. Fields are mutually exclusive; mismatches fail before listening.
- `realm_id`/`realm_next_hop` in local profile identify the opened Realm/local contact.
  The Core Store's persistent contact must match exactly. Configuration detects misassembly;
  it cannot override or invent contacts.
- Explicit positive network durations/counts are required. Session TTL is whole seconds;
  connection lifetime cannot be shorter than RPC deadline.
- Omitted `snapshot_retention` disables historical snapshots; a positive duration enables
  retention. No duration default exists.
- Optional `public_max_contexts`, `public_max_contexts_per_caller`,
  `public_max_groups_per_observation`, `public_max_bytes`,
  `public_max_concurrent_reads` assemble the existing P5 resource policy without copying its
  rules. Omission keeps P5 count defaults, unlike snapshot retention. Reads also retain
  bounded receipts: raising session capacity alone is insufficient.
- `max_response_bytes` bounds canonical internal responses and network messages.
  Too small means resource failure, not truncation. Large results require appropriate query
  batches/receive limits; a small request does not imply a small response.
- Ctrl+C/SIGTERM stops admission, waits finitely, then closes the owner. Exhausted connection
  lifetime closes the connection only; unexpired sessions can reconnect. Restart invalidates
  sessions/public read contexts, not durable facts or operation receipts.

## Network Realm profile

To use the independent `cmd/ucupa-realm` service for identity/authorization, explicitly select:

```json
{
  "realm_profile": "network",
  "realm_endpoints": [
    {
      "next_hop": "https://127.0.0.1:9443",
      "ca_file": "C:/private/ucupa/realm-ca.pem",
      "spki_sha256": "<Realm service SPKI SHA-256 as strict base64url>",
      "service_certificate_file": "C:/private/ucupa/core-service.pem",
      "service_private_key_file": "C:/private/ucupa/core-service.key"
    }
  ]
}
```

This fragment is not a complete configuration; numbers are illustrative.
Omit `realm_store`/`realm_next_hop`; profiles are exclusive.
Each endpoint fixes Realm address, TLS roots, SPKI pin, and this Core's registered mTLS identity.
Endpoints reuse host `rpc_deadline`/`handshake_timeout` and message limits.
Duplicate/invalid endpoints fail before listening.

Open first verifies the current persistent contact online; no service starts before success.
Runtime authorization, revalidation, P6 source identity, and contact maintenance remain online
through that connection, outside fact locks. Unreachability fails closed, never offline Allow.
Realm must explicitly admit the service identity; an unadmitted service gets no access.
Configuration creates no permission; Realm independently decides.
Evidence: [network Realm processes](../../acceptance/network_realm_contact_process_test.go)
and [maintenance/lock checks](../../acceptance/network_realm_contact_maintenance_test.go).

## Client

`ucupa-core/sdk/go` `New(Config)` takes explicit NextHop, expected Core, trusted CA,
expected service SPKI SHA-256, call/connection deadlines, and receive limits.
Identity-to-key binding contains no address and is not Ref lookup.
After TLS target verification, `Establish(ctx, realmID, credentialID, secret)` establishes
a finite session. `client.Core()` exposes generated `Enter`, `ReadElement`, `QueryRegion`, etc.
Types come from [core.proto](../../ucupa-core/api/core.proto) and generated bindings, not
a second SDK business-method model.

Callers cannot override SDK metadata, codec, compression, target, or retries.
Connections use no environment proxy or redirects.
The host registers Protobuf only, no compressor/JSON codec, REST, gRPC-Web, or plaintext.
`Release` releases a session; `Close` closes the client connection. Neither deletes facts.

## Optional read delegation

To let a specified Core read this target for a caller, configure `read_delegation`.
Omission disables it; trust is never automatic:

```json
{
  "read_delegation": {
    "client_ca_file": "C:/private/ucupa/actor-ca.pem",
    "client_spki_sha256": {
      "00000000-0000-4000-8000-000000000002": "<delegate Core client SPKI SHA-256 as strict base64url>"
    },
    "max_ttl": "10s",
    "max_grants": 64,
    "max_per_caller": 4
  }
}
```

This is an illustrative fragment. Durations are positive whole seconds; counts are positive;
per-caller cannot exceed global capacity. The delegate needs a verifiable client-authentication
certificate and key. The target checks CA chain and exact identity-to-key binding.
This address-free table is not Ref routing. Ordinary users may use server-authenticated TLS
without client certificates.

1. After discovering a disclosable target, the user directly authenticates it under explicit
   policy; prelogin to every Core is unnecessary.
2. `client.Delegations().CreateReadDelegation` specifies Reference, delegate Core,
   read/list/query subset, observation flag, and expiry. Target uses the user's original
   permission to create bounded entry. Response contains a short-lived token/handle, not
   secret, session, or CallerContext.
3. The delegate creates an SDK client for the same target, with `Actor`/
   `ActorCertificate`, then `UseReadDelegation(result)`. Use the original handle for
   generated `ReadElement`, `ListDirect`, `QueryRegion`. No new Enter, writes,
   write-outcome recovery, or onward delegation is allowed.
4. The user may `RevokeReadDelegation` through a new valid session.
   Delegation expires and is invalid after host restart; reconnect does not extend it.

Every read checks current target-side user permission/credentials, not the delegate Core's
service permission. Each delegation has its own run-local caller; handles/observation cannot
mix. Observation cannot bypass P5 revoke/regrant GAP.
Releasing the original user session is not revocation, but its original expiry still bounds
delegation. Revoke explicitly for early termination.
Lost creation replies may leave only a finite capacity reservation, not a spatial write;
the SDK does not automatically resend creation.

Issuance/use/revocation alone does not enable automatic cross-mount relay.
Automatic federation additionally needs explicit identity exchange and `federation` configuration.
Absent identity/trust policy explicitly fails; no trust learning or expansion occurs.

## Explicit identity issuance and exchange

Omission disables the capability; no keys/trust are generated automatically.
Both sides need `read_delegation` peer CA/pins: issuer admits target liveness checks;
target admits entry Core exchange/delegation use.

```json
{
  "identity_issuer": {
    "signing_key_file": "identity-private.pem",
    "max_ttl": "30s", "max_proofs": 100, "max_per_caller": 10
  }
}
```

The key is one PKCS#8 Ed25519 private-key PEM, managed separately from TLS identity.
This is not complete configuration; numbers are examples. Target configuration:

```json
{
  "external_identity": {
    "max_ttl": "30s",
    "issuers": {
      "issuer-realm": {
        "signing_public_key_file": "issuer-public.pem",
        "next_hop": "https://issuer.example:443",
        "expected_core": "<explicit issuer Core UUID>",
        "ca_file": "issuer-ca.pem",
        "spki_sha256": "<strict base64url SHA-256>"
      }
    }
  }
}
```

The public key is PKIX Ed25519 PUBLIC KEY PEM. The target uses its configured TLS certificate
for reverse client authentication; it must support clientAuth. Addresses, issuer, signing keys,
and Core pin are explicit configuration, not learned from requests/mounts.
Existing RPC/connection/message limits bound checks, without implicit defaults.

Sequence: entry `Delegations().BeginIdentityChallenge` → user at issuer
`IdentityProofs().IssueIdentityProof` → entry at target `Delegations().ExchangeIdentity` →
new actor client `UseReadDelegation`. Users need no hidden target address.
Exchange supports federation assembly, but configuring it alone does not enable queries.
A signature alone is insufficient: every target use checks original issuer credential liveness,
then the target's own Realm authorizes.

## Results and recovery

### Optional public federation

Entry `federation` reuses specified addresses, expected Core, and TLS trust, not an
identity-to-address directory:

```json
{"federation":{"ttl":"20s","max_parts":64,"peers":[{"next_hop":"https://core-b.example:443","expected_core":"00000000-0000-4000-8000-000000000002","ca_file":"peer-ca.pem","spki_sha256":"<base64url-encoded-32-byte-pin>"}]}}
```

Omission disables automatic public federation. This illustrative fragment requires 1–256 peers,
positive whole-second TTL, and positive `max_parts`, bounding concurrent retained source
leases for all callers at this entry.
Duplicate addresses, self Core, invalid addresses/identity, or missing CA/pin/client key fail
startup. Peers configure trust for a relation's **specified address**, not ownership.
Startup neither connects to nor scans these Cores.
Multi-level reads require explicit trust for each actual source address. Intermediate Cores
do not onward-delegate with their own permissions on behalf of the requester.

Entry also needs `identity_issuer`. Sources need `external_identity` trusting the original
issuer Realm and `read_delegation` CA/peer pins authenticating entry. Issuer must admit reverse
credential checks. Trust configuration grants no spatial permission; each target Realm
independently authorizes the original requester's actual operation.
Lease lifetime is bounded by configured TTL, original session, and source expiry.
Pagination does not renew leases or recapture state.

Public Read uses the original receipt as `basis`, independently checking Read permission
on the original closed source snapshot. Source-issued one-time challenges prove an independent
Read request bound to original query basis, target Element, and requester.
Query delegation does not become Read or refresh snapshot/authorization.
Query/List support multi-source prepared Observe with independent per-source commit order
and an aggregate entry cursor. Exact Read preparation observes actual path/target, not
unrelated Elements. Historical receipts cannot retroactively prepare.
Source disclosure, completeness, mixed retention, and reclamation follow
[network contract §8.5](../../docs/architecture/network-access-contract.md#85-public-federation-source-projection-and-composition-delivered).

### Mount approval exchange

Optional `mount_approvals` requires explicit `max_ttl`, `max_approvals`, `max_per_caller`,
for example `{"max_ttl":"30s","max_approvals":64,"max_per_caller":8}`.
Omission disables it. Service identity reuses `read_delegation` CA/peer pins,
but approval has separate capacity/scope; read grants do not substitute.

Users authenticate directly to target and call `MountApprovals().CreateMountApproval` with
one exact command binding and expiry. The authorized Core uses configured mTLS for
`CheckMountApproval`; target revalidates original credentials/view.
Users may `RevokeMountApproval`; restart invalidates old approvals.
The returned Principal is attribution, not transferable authentication evidence.

`MountPeer()` supplies service-only PrepareMount/FinishMount/ReadMountDecision over mTLS.
PrepareMount checks exact request/approval, original Principal, command commitment, and view.
FinishMount/ReadMountDecision fulfill established obligations using service recovery authority,
not still-valid user approval. Peer messages are not ordinary-consumer operations;
Bearer/read grants cannot replace service identity.

Users obtain comparisons through `MountControl().InspectMount` and exact target approval
binding through `DescribeMount`, then submit `ApplyMount`.
Optional `mount_connections` assembles connections with explicit
`next_hop`, `expected_core`, `ca_file`, and `spki_sha256`:

```json
{"mount_connections":[{"next_hop":"https://core-b.example:443","expected_core":"00000000-0000-4000-8000-000000000002","ca_file":"peer-ca.pem","spki_sha256":"<base64url-encoded-32-byte-pin>"}]}
```

Omission disables ordinary mount control; explicit empty arrays are invalid.
Connections use the host certificate for mTLS, retain no user approvals, and select/check owners
only by relation-specified address, without Ref search/fallback.
Every target explicitly trusts source peer certificates.

Startup performs one RPC-deadline-bounded recovery traversal: durable decisions may Finish/ReadDecision;
undecided writes cannot continue from a bare Principal. Offline peers retain unfinished operations
and gates, without claiming success or automatic Abort. Original callers can reauthenticate and
retry exact original commands. Shutdown cancels/waits for traversal before closing service,
connections, and Store.

`MountContact().InspectMountContact`/`RepairMountContact` maintain one direction via public
contact tokens. Parent needs MountControl, carrier MountAccept, contact reads MountRead.
Only local records change; the old address need not be accessed, and a waiting Range operation
does not block repair. New addresses need explicitly usable connections; no discovery, redirects,
or Ref lookup. Retry the original command after repair; address changes do not switch fact
ownership or bypass permissions.

Evidence: [TLS/permission counterexamples](../../acceptance/network_mount_approval_test.go),
[independent-product restart](../../acceptance/network_mount_approval_process_test.go),
[peer TLS Prepare/decision/Finish](../../acceptance/network_mount_peer_test.go),
[public control black box](../../acceptance/network_mount_public_test.go), and
[two products/forced restart](../../acceptance/network_mount_product_test.go).
The peer-only test retains local parent control and is not proof of a network application entry.
Product tests close fixture owners before launching hosts.

### Facet relationships and bidirectional contacts

Users call `FacetControl().InspectFacet` for proposal/contact comparisons, then `ApplyFacet`
for propose, cancel, activate, detach, or contact.
First acceptance is requested at Facet Relationships. Its fixed service checks this Core's
current proposal; `facet_services` explicitly binds service Principal to Facet.
That binding does not grant user or proposal-read/attachment-read permissions.

Activation requires a Core-to-Facet acceptance read, for example:

```json
{"facet_connections":[{"next_hop":"https://facet.example:443","expected_facet":"00000000-0000-4000-8000-000000000003","ca_file":"facet-ca.pem","spki_sha256":"<base64url-encoded-32-byte-pin>","credential_file":"facet-service-credential.json"}]}
```

The credential file has exactly `audience`, `credential_id` (16-byte base64url), and
`secret` (32-byte base64url): this Core's fixed-service credential at the target Facet Realm,
not original-user secrets.
Facet needs corresponding `core_services` and acceptance-read grants;
see the [Facet host](../ucupa-facet/README.md).

Omission assembles no connections. Empty arrays, duplicate addresses, or invalid identity/trust
fail startup. At most 256 entries, 1 MiB CA files, and 4 KiB credential files.
Only relation addresses select connections; no Ref lookup/fallback.
Offline peers do not prevent local contact repair. Repair changes no proposal, acceptance,
or ownership. [Empty-relation product tests](../../acceptance/network_facet_relationship_product_test.go)
cover network establishment, content, bidirectional offline repairs, both restarts, and
post-detach refusal.

### Single-writer handoff

P7.1 has independent same-machine product-process evidence for explicit sequential handoff
of the same original Core Store, not copied storage or outage takeover.
`writer` contains only `handoff` and installation fields.
Trusted initialization persists the switch; every open must match, with no later switching.
The original Store's Realm contact constrains authorization; there is no arbitrary assembly label.
Strict decoding rejects `authorization_binding`. Ordinary enabled startup:

```json
{"writer":{"handoff":true}}
```

`handoff` is capability configuration, not authorization evidence.
Mismatch fails startup. Omitted `writer` opens only handoff-disabled Stores;
it does not enable handoff. Public admission/session configuration remains explicit.

After normal authentication, `Writer().InspectWriter` reads epoch/original decision.
`Writer().HandoffWriter` submits original operation ID, expected epoch, and SHA-256 commitment
of target installation secret. Realm must separately grant
`ucupa.core.writer.inspect`/`ucupa.core.writer.handoff` on this Core's exact AuthorityScope;
ordinary spatial/read-delegation permission is insufficient.
Secret bytes never enter RPC, logs, or ordinary responses.

`values.NewWriterInstallation(operation)` generates independent random qualification.
Protect its 32 raw secret bytes and give them only to the designated instance; source sees
Commitment only. After source fence, stop the old process and release the original Store
before starting the new instance. For operation `handoff-1`:

```json
{"writer":{"handoff":true,"install_operation":"aGFuZG9mZi0x","install_secret_file":"writer-installation.bin"}}
```

`install_operation` is standard JSON base64 of 1..128 raw bytes, not a UTF-8 name.
`install_secret_file` is exactly 32 binary bytes, not hex/base64 text; supply both fields together.
The new instance exclusively checks same Store, owner, switch, operation, and secret, installs
the next epoch, then listens. Ordinary Open cannot take over pending installation.
Wrong switches/secrets/old qualifications fail. Exact restart of an installed epoch does not
advance it again. Later ordinary restart can omit installation fields but retains `handoff`.

Lost replies require reauthentication and original-ID inspection/exact retry, not a new target,
ID, or timeout-as-no-commit assumption. Old sessions/snapshots/observations/in-flight reads cannot
continue disclosure. New runs retain original identity, facts, and business outcomes.
See [writer product tests](../../acceptance/writer_product_test.go).
No old-format reader/converter or global copied-Store anti-dual-write mechanism exists;
deployment must satisfy the original-Store premise.

### Source transfer control

The host assembles actual `owner.Transfers()` into `TransitionControlService`.
`client.Transitions()` exposes BeginTransition/InspectTransition/CancelTransition.
These three actions alone manage original source intent; Recover separately drives supported
accept/install paths. Overall bounded P7.2 completion is recorded in the plan.

Establish a real user session first. Export/inspect target the exact source Element boundary.
Ordinary CRUD, read delegation, and fixed-service identity do not substitute.
Transfer OperationID cannot reuse an ordinary-write ID.
Begin retains expected, target, final placement, and contacts without immediately closing content.
Cancel is allowed only before fence. Exact retry retains ID/input/expected; restart requires
reauthentication of the original caller.
INDETERMINATE or disconnection is not proof of no commit. Inspect original key or retry exact
Begin/Cancel after authentication. SDK validates attribution, not business orchestration.
[Source-product tests](../../acceptance/transition_product_test.go) cover independent source
restart; individual control cases alone are not a complete multi-party transfer proof.

### Target preflight approval

Enable `transition_approvals` explicitly with positive-whole-second `max_ttl`,
`max_approvals`, and `max_per_caller`. Omission disables it; Mount capacities/permissions
are not inherited. Certificate trust reuses explicit `read_delegation` peers but grants no
read/transfer permission.

SDK `TransitionApprovals()` exposes generated methods: Create/Revoke use original-user session;
Check uses fixed-source certificate identity. Approval covers the exact original transfer and
expires no later than that session. After restart, authenticate and request exact approval again.
Tokens are not durable acceptance or a condition for retaining accepted contents.
Revocation does not cancel durable obligations.
Evidence: [target approval/restart](../../acceptance/transition_approval_process_test.go).
Approval tests alone do not establish source-to-install completion.

### Bounded source evidence reads

`TransitionSourceService` uses the same `owner.Transfers()`.
SDK `TransitionSource()` exposes generated chunk/decision reads.
Only certificate-authenticated `read_delegation.peers` matching the original target can read;
ordinary-user bearer is insufficient. Durable-obligation reads need not revive expired approval
and cannot initiate export/install.

The 64 KiB chunk/1 MiB manifest limits also obey host concurrency, response, and deadline limits.
`transport/grpc/transitionconn` reuses pinned specified-address clients and requires explicit
total pull deadline/concurrency. No independent directory/Ref resolution exists.
Evidence: [network source/reopen](../../ucupa-core/tests/transfer/network_source_test.go).
This tests the evidence boundary, not the whole transfer profile.

### Bounded target acceptance and installation

Go hosting explicitly assembles `TransitionTarget`/`TransitionConnections` for the same
owner and existing certificate trust. Missing/mismatched owner configuration fails.
SDK `TransitionTarget()` exposes generated methods.
`transitionconn.NewTargetConnection` binds address, original command, and original approval.
One explicit connection set reads source evidence and advances target operations; no old
one-way assembly name or second address index remains.

Acceptance collects immutable source evidence outside the lock, then serially checks approval/
session and invokes the actual owner. Admission holds only prefetched call-local evidence and
bounded commit work, not network waits, persistent caches, or a second coordinator.
Approval/grant revocation prevents first acceptance but does not erase durable obligations.
Exact retry returns original Store results.
[TLS target tests](../../ucupa-core/tests/transfer/network_target_test.go) cover changed approvals,
concurrency, reply loss, reopen, local recovery, and source-session release during public Recover.
Those cases alone do not validate carrier/external-mount combinations.

### Specified connections and public recovery

Optional `transition_connections` specifies `peers`, `max_concurrent_pulls`,
and `pull_deadline`; omission disables network advance.
Each peer supplies `next_hop`, `expected_core`, `ca_file`, `spki_sha256`, matching the
same peer/pin in `read_delegation` trust. Peer/pull counts are 1–1024; deadline is explicitly
positive. Duplicate/self addresses, wrong identity/pin, missing trust, and invalid limits fail
before listening. Host closes SDK clients; no discovery, address switching, or background
transfer loop occurs.

Original user obtains target `TransitionApproval`, then calls source
`Transitions().RecoverTransition` with original `binding`/`approval_token`.
Source retains current user session/original authorization; service certificates identify peers
only. Target independently checks original authorization.
Each call advances the original operation once; SDK does not auto-retry or rebuild approval.
Source-session release/expiry, shutdown, and cancellation stop uncommitted steps, not durable
obligations. [Transfer product tests](../../acceptance/transition_product_test.go) cover two
actual products, recovery, install, restart, and original outcomes.
Simple carrier has separate evidence below; outgoing external-edge continuation remains unsupported.

### Parent-side carrier control and recovery

The same `TransitionControlService`/`client.Transitions()` exposes
`BeginCarrierTransition`, `InspectCarrierTransition`, `CancelCarrierTransition`, and
`RecoverCarrierTransition`. Connect to parent Core, not old source carrier, using a normal
user session established at parent.
Begin/Cancel require parent+mount `transition.commit`; Inspect separately requires
`transition.inspect`. These controls do not obtain source/target approvals for the user or
equate reservation with fencing/installation.
Use `MountControl().InspectMount` for original mount comparison; its own connection
configuration still applies.

Before fence permit, reservation/cancellation does not require online source/target.
After permit, disconnection does not justify cancellation.
Begin exact retry returns original reservation; Inspect current progress; Cancel compares
original reservation. Restart requires reauthentication, preserving raw operation bytes/outcomes.
Cancellation advances mount comparison without changing spatial Elements, carrier, or address.
New operations cannot use the old comparison.
[Parent product tests](../../acceptance/carrier_product_test.go) validate that control boundary.

The supported simple-carrier public recovery chain uses explicit original binding plus source
and target approvals. [Three independent TLS-host tests](../../ucupa-core/tests/transfer/network_carrier_three_party_test.go)
cover completion, original results, retries, parent/source/target restart, permission/identity/
binding/capacity failures, repaired contacts, source outage, and safe rejection of external
mount boundaries. These hosts are same-machine evidence, not real multi-machine/production
or a claim that every component-only test is a complete product workflow.

### Carrier parent evidence reads

Actual `Transfers()` implements `TransitionParentService`.
Existing service-peer trust is reused; absent trust yields unavailable, not new authentication.
Source/target use their fixed TLS certificates and original specified parent address through
`client.TransitionParent().ReadCarrierPlan/ReadCarrierDecision`.
These are read-only original-obligation evidence, not user grants/state advances;
binding mismatch rejects.

`transitionconn.Connections` reuses existing addresses, concurrency, and deadlines without
automatic routing or target replacement.
[Parent network tests](../../ucupa-core/tests/transfer/network_carrier_parent_test.go)
use real three-party Stores/TLS, parent restart, and wrong certificates/identities.
They network only parent checks and use trusted local source/target controls; the full supported
public chain is covered separately above.

### Ordinary read/write outcomes

Check both RPC error and response `result/failure`.
A complete domain failure is an owner reply; timeout/disconnection is not proof of failed commit.
Possibly dispatched writes remain indeterminate. Use `RecoverOutcome` with original
OperationID/request/CAS, never a new ID or automatically updated CAS.
Same-basis Read uses the original query/list handle and receipt, not a handle from another Enter.

See [independent processes](../../acceptance/network_process_test.go) and
[spatial operations/observation](../../acceptance/network_core_test.go).
Test CAs, credentials, and Stores are generated temporarily, not deployment material.
