# P6 network access and open federation contract

**Status: P6 was delivered within its declared implementation and measured scope. The user stopped further environment-specific validation for now. Sections 1–7 define base network access; sections 8.1–8.6 define and implement restricted delegation, identity trust, Facet/mount control, and public federation protocols. Final delivery evidence and untested boundaries are in the development plan; contract text is not test evidence. The protocol is gRPC + Protobuf + TLS. Existing two-host measurements do not establish untested environmental conditions.**

**Stage 2 successor notice:** the [RealmAttachment contract](realm-attachment-contract.md) supersedes the
sections 3/8.1 slice limits of local-only authorization assembly and no RealmAttachment implementation.
Receiving owners contact the designated Realm through their original Store contact.
Local/network profiles are explicitly configured and do not fall back on failure.
P6 original-caller, Actor, issuer-trust, target-identity, bounded-session, and independent-authorization rules remain.
Bounded network-Realm product-combination validation and P6 two-host evidence are recorded separately;
neither substitutes for real multi-host Realm or production acceptance.

This contract implements [mainline D6–D11](ucupa-concept-mainline-baseline.md)'s real communication boundaries,
retaining owner, commit, and permission rules from [P5 public behavior](public-access-and-observation.md),
[mount control/query](core-mount-contract.md), and [Facet fact continuity](facet-local-truth-and-core-lifecycle.md).
Phase order/delivery state appear only in the [development plan](development-plan.md).
This defines the network increment, not another routing or authorization model.

## 1. Environments and proof boundaries

The first implementable slice is a separate caller process accessing a durable Core's P5 public operations over gRPC/HTTP/2/TLS.
Core service and caller share no pointers, CallerContext, Store, locks, or test memory; they exchange only public bytes.
Local validation uses separate listening ports.
Moving to another host address reuses the same protocol/program without a second remote API.
Network methods/structured messages use Protobuf, with no JSON/HTTP, REST gateway, gRPC-Web, or protocol fallback.
gRPC is chosen for established RPC, generated-client, and connection-management facilities,
not as evidence that it outperforms another design.

Complete P6 also includes the two-way Facet network chain, mount service-control messages,
public nested federation, and independent-implementation interoperability.
The first Core endpoint alone does not establish these; section 8 specifies their required boundaries.
Separate processes on one machine are one executable validation environment,
not a reduction of the P6 target or a spatial-model restriction.

| Validation environment | Can establish | Cannot substitute for |
|---|---|---|
| Focused gRPC handler/TLS tests | Messages, defensive inputs, identity, and error mapping | Separate processes, persistent recovery, or cross-host operation |
| Separate processes on one machine + TCP/TLS + independent Stores | Actual serialization, authentication, process isolation, disconnection, exit, and recovery | Real host networks, firewalls, or machine clock differences |
| The same programs on two real hosts | Address reachability, certificate deployment, independent restart, and failure behavior in that environment | Arbitrary networks, scale, or HA |
| Independent implementations passing a shared corpus | Interoperability of the declared public semantics | Two identical Go processes or separate clients counted as two implementations |

Without a cross-host environment, no passing record is fabricated.
After the local network slice, other locally verifiable P6 work may continue.
Real cross-host acceptance is recorded separately; partial measurements do not establish every runtime environment.
Current P6 work stops at recorded evidence.
Untested scenarios are not an unconditional construction queue;
the development plan records progress and later on-demand boundaries.

## 2. NextHop and target authentication

### 2.1 One explicit network-address profile

Network `NextHop` extends code, shared-value definitions, and affected schemas with an explicit network-address form,
rather than secretly mapping a local name to a remote URL in transport.

- Network NextHop is fixed as `https://host:port`: explicit port 1–65535 without leading zeroes;
  no path, query, fragment, or userinfo.
  Here `https` means a TLS-protected HTTP/2 service entry, not a JSON/HTTP API.
  Native gRPC is the sole application protocol.
  After address parsing/validation, the adapter constructs a fixed host:port gRPC target;
  it does not pass the raw URI to a default resolver to infer meaning.
- Host is lowercase ASCII DNS, canonical IPv4, or canonical bracketed IPv6;
  percent encoding, zones, whitespace, and trailing dots are rejected.
  Exact canonical checks and positive/negative cases belong in shared schema/value tests.
  Automatic input cleanup must not change a persistent contact.
- `local:` still means a genuinely supported in-process adapter, not legacy protocol compatibility.
  Network endpoints reject local inputs rather than falling back to the local connection table.
- An address always comes from the designated entry/relationship contact.
  DNS resolves only that host; it does not locate a FactAuthorityRef.
- The client connects to that fixed host:port.
  Environment proxies and resolver-provided service config are disabled;
  no xDS, SRV discovery, custom Ref resolver, fallback service, or peer-provided load-balancing/retry policy.
  IP resolution of one DNS host is not additional UCUPA routing.
  HTTP redirects, gRPC metadata, and errors cannot teach new contacts.
  Missing h2/TLS fails without plaintext/JSON downgrade.
  Address changes still require the relationship owner's authorized CAS/OperationID commit;
  a network library cannot update the contact for it.

### 2.2 Check address, key, and fact identity separately

The first profile uses gRPC transport credentials, TLS 1.3, ALPN `h2`,
standard certificate-chain/hostname validation, and an additional trusted
`expected FactAuthorityRef → server SPKI SHA-256` binding.
That binding specifies which key may represent the expected target, without an address; it is not a Ref-routing table.
Initial trust comes from controlled caller configuration, not automatic acceptance of SpaceReference/trustHint.
Tests use a temporary test CA and independent service keys per owner,
without a global CA infrastructure, online certificate platform, or K8s.

Before sending credentials, sessions, or business bodies, the client completes TLS and expected-owner key-binding checks.
A Ref in a response alone is insufficient.
Every RPC also has exactly one `ucupa-target` metadata value,
which the receiver compares exactly with the actual AuthorityRef of its opened Store.
Missing/duplicate/invalid metadata is rejected.
Arbitrary `:authority` cannot replace target identity, certificate hostname, or owner.
A host whose configured owner does not match the Store refuses to listen;
it does not create/replace a Store to manufacture identity.
Distinct fact owners do not share service keys in this profile.
Moving a process between machines does not change AuthorityRef; key changes require explicit trust-material updates.

TLS terminates at the service process.
The first profile does not trust proxy-reported identities or `X-Forwarded-*`.
It does not use `InsecureSkipVerify` or temporarily allow expired certificates.
Reconnect checks identity again.
Trust configuration is fixed for this run; changing it requires reassembly,
not retaining an old connection to bypass revocation.
Malicious writable clones of the same key/Store are outside this slice's security guarantee.
The sole writer remains constrained by original owner rules.

## 3. Request identity and bounded sessions

Network authentication reuses Realm `CredentialEvidence` and `EstablishCaller`,
not `TrustLocalCaller(client-reported Principal)` or serialization of local CallerContext/views.
The original P6 slice used explicitly local receiver-side authorization assembly
and did not implement Stage 2 RealmAttachment or automatic cross-Realm trust.
The successor notice above defines the delivered contact extension; automatic trust remains excluded.

Each EstablishCaller creates a new establishment.
Reestablishing it for every business request cannot preserve an old handle's identity.
A bounded receiving-service run-local session retains one authenticated CallerContext:

1. `SessionService.EstablishSession` is unary with typed fields `audience:string`,
   `credential_id:bytes`, and `secret:bytes`.
   ID/secret are exactly 16/32 bytes, retaining Realm nonzero/audience checks.
   No Principal, Realm address, or caller-reported authorization is accepted.
   Maximum request size is 4 KiB, sent only through verified TLS.
2. The host adapter submits evidence to the assembled Realm.
   Successful validation generates a random 32-byte opaque session,
   returning typed `session:bytes` and positive `expires_in_seconds:uint64`.
   No secret, CallerContext, Proof, view, or private Realm state is returned.
3. A session stores only the random token's SHA-256 index, original CallerContext,
   and this run's absolute expiry basis, not the credential secret.
   Lifetime is fixed at creation; requests, Poll, and retries do not extend it.
   No refresh or cross-restart recovery is provided.
4. Later RPCs use exactly one `authorization` metadata value:
   `Bearer <strict unpadded base64url session>`.
   Session decoding must yield 32 bytes.
   Only metadata uses this text form; the entire Protobuf message does not become JSON.
   The bearer is valid only at this service and is not forwarded to a next hop.
   A connected gRPC channel is not an authenticated caller.
   The owner still executes original AuthorizeView/ValidateView on every operation,
   so credential/permission revocation does not wait for session expiry.
   Unverifiable authentication does not permit access.
   Errors disclose no credential, subject, or resource existence.
5. `SessionService.ReleaseSession` uses the same bearer and an empty request to release the local session.
   A well-shaped unknown/already released token returns the same empty success,
   not a token-existence oracle or anonymous business-call exemption.
   Identity-related owner resources follow original TTL reclamation,
   without deleting facts/outcomes or requiring transport to enumerate private handles.
6. After disconnection, a still-valid session may reconnect and continue the original read chain.
   Expiry or host/Realm restart requires authentication, Enter, and Read again.
   A new session is not inserted into old handles.
   Invalid observation basis requires a new prepared Read, not silent continuation from latest.

This first session profile is a protected bearer, without stolen-token anti-replay/device-binding guarantees.
It reduces repeated transmission of long-lived credentials, not final authorization checks.
Secrets come only from controlled input, never CLI arguments, committed examples, logs, or exception bodies.
No cookies, browser auto-login, or open CORS automatically attach credentials to cross-site requests.

## 4. RPCs, messages, and failure semantics

### 4.1 Explicit methods, not a JSON tunnel

The first Core network service is `ucupa.core.CoreService`;
the session service is `ucupa.network.SessionService`,
implemented locally by each fact service, not a separate authentication service or new Realm.
All first-profile methods are unary:

| RPC | Accepted public operation |
|---|---|
| Enter / Close | `enter` / `close` |
| ReadElement / ListDirect / QueryRegion | `read` / `list` / `query` |
| BeginObserve / PollChanges / ReleaseObserve | `begin-observe` / `poll` / `release-observe` |
| CreateAnchor / CreateEntity | `create-anchor` / `create-entity` |
| UpdateAnchor / UpdateEntity | `update-anchor` / `update-entity` |
| DeleteAnchor / DeleteEntity / RecoverOutcome | `delete-anchor` / `delete-entity` / `recover-outcome` |

Each method has its own request/response messages, inheriting P5 requirements, scope, and semantics without generic CRUD.
No `Call(bytes json)`, `google.protobuf.Struct/Any`, or custom JSON codec is the main public entry.
Facet content/service-control methods follow the same rules in their slice;
the current `.proto` does not pre-expose unauthorized control interfaces.
gRPC streaming support does not automatically convert Observe to push.
The first profile retains prepared Read→Begin→Poll, cursors, and GAP.
A later streaming design must prove backpressure, reconnect, per-batch authorization, and existing gap semantics;
a stream alone does not establish continuity.

### 4.2 Protobuf representation and one semantics

`.proto` uses proto3 with explicit presence and is the sole network-field/number source,
not multiple handwritten bindings.
Shared network values/sessions belong in `shared/api/`;
Core/Facet methods belong in their own `api/`.
Dependency direction is owner proto → shared proto.
Concrete field numbers, all messages, and positive/negative cases are fixed before slice generation;
this method table is not a complete compilable protocol.

| Semantics | Network representation and validation |
|---|---|
| UUID, RealmID, PrincipalID, NextHop | Explicit string fields with existing accepted validation; no name/address conversion |
| OperationID, digests, token value, session | bytes with original length/role constraints; token owner/kind remain explicit, without private sequence numbers |
| Query UpDepth / path up | Explicitly present sint64 / uint64; only UpDepth permits -1, retaining zero/omitted/upper-bound rules |
| Pose, Range, query sphere | Structured messages and finite double, retaining numerical/range/intersection rules without float32 precision reduction |
| Required/optional fields and exclusive outcomes | message/optional/oneof express presence; missing required fields reject rather than using Protobuf zero values to supply business input |
| Facet TypeRef and exact payload | Fixed opaque 32-byte TypeRef and unchanged Payload bytes; Facet neither parses artifacts nor validates content formats, and does not convert to Struct or re-encode content |

Numeric zero follows original JCS semantics.
Protobuf negative-zero bit patterns cannot create another business request; NaN/Infinity reject.
After protobuf decoding, recursively reject unknown fields, undefined enums,
missing required oneof, and incorrect roles/lengths.
Ignoring unknown content is not a legacy-compatibility mechanism.
Re-encoded bytes need not equal input, and deterministic protobuf serialization is not canonicalization.
Field-order/encoding differences that decode to the same valid values do not create different business requests.

**Original OperationID bindings, Facet digests, and public Revision semantics remain.**
The server uniquely maps validated typed requests into P5 canonical semantic inputs.
The first implementation may deterministically map fields into canonical JSON for existing PublicCall.
That operation envelope is server-internal, not the complete gRPC payload or another network endpoint.
Facet content's exact Payload bytes travel unchanged as specified above;
this does not express the whole RPC in JSON.
Adapters represent messages, not adjudicate business rules.
Omitted/explicit optional values retain P5 distinctions.
RecoverOutcome uses the typed original command, without an expired handle,
to produce the original canonical request through the same mapping.
Raw Protobuf bytes, gRPC metadata, and new sessions do not enter operation identity.
Responses likewise become typed values.
Generated bindings do not replace original cut/role/permission validation.

Local PublicCall and JCS schemas remain genuinely supported local representations,
not a legacy HTTP service awaiting deployment.
Proto does not duplicate domain rules.
Cross-representation conformance establishes equivalent decisions/outcomes for equal inputs:
one semantics, not two versions.
Encoding/decoding optimization requires measurements and refinement of shared values,
not rewritten commit rules to claim binary-protocol performance.

### 4.3 Separate domain results from gRPC status

Every business response explicitly has `oneof { result, failure }` with exactly one branch.
Failure enums retain exact PublicError meanings.
Valid owner results use gRPC `OK`; network errors do not imply a persistent outcome.

| Result source | gRPC representation | Meaning |
|---|---|---|
| Owner success | OK + typed result | Receive/validate the whole result and final status; headers are not commit evidence |
| Explicit owner PublicError | OK + typed failure | Preserve DENIED/NOT_FOUND/CONFLICT/GAP/INDETERMINATE and others, not generic INTERNAL |
| Missing/invalid/expired session or failed authentication | UNAUTHENTICATED | Neither existence nor absence, and not a substitute for owner permission denial |
| Invalid message-field/metadata shape | INVALID_ARGUMENT | No owner call; no internal error text |
| Message/metadata/connection/request-admission limits exceeded | RESOURCE_EXHAUSTED | Not empty facts or a new query mode |
| Wrong expected target, unavailable authorization dependency, service shutdown | UNAVAILABLE | No target substitution or NOT_FOUND claim |
| RPC timeout/cancellation/unexplained internal failure | DEADLINE_EXCEEDED / CANCELLED / INTERNAL and others | Not proof that the owner did not commit; recovery follows the next section |
| Unsupported service/method | UNIMPLEMENTED | Missing RPC, not Element NOT_FOUND or JSON/HTTP fallback |

No JSON error envelope or guessed mapping of HTTP status to domain status is provided.
Unknown/missing/invalid terminal states never count as success.
Do not cache business responses, automatically decompress unapproved messages,
or expose secrets/private state/hidden graphs in status details or trailers.
Identity/authorization interceptors do not decide spatial/type validity for the owner;
domain rejection still occurs at the original boundary.

### 4.4 Deadlines, retry, and recovery

Clients distinguish complete owner outcomes from failure to obtain a result.
TLS/EOF/truncation/timeout/gRPC failure is not commit rejection.
Once dispatch to the owner is possible, the local write-recovery state is outcome unknown;
that does not fabricate an owner INDETERMINATE receipt.
Only proven failure before sending permits a not-dispatched state.
Recovery retains original OperationID, actor, canonical request, and preconditions.
After session loss, authenticate again and call RecoverOutcome.
Do not change ID/CAS to silently rewrite or substitute observations for an outcome.

Disable policy retries, hedging, and WaitForReady.
Reject resolver-provided service config and configure no local automatic business retry.
grpc-go `WithDisableRetry` does not disable every transparent retry:
calls proven unsent or unprocessed by the server application may still be retried.
Only these pre-application transparent retries are accepted,
with fault tests showing no duplicate entry to the commit path.
The option is not a guarantee of absolutely zero retries.
All attempts/reconnects use one call deadline rather than a new full timeout each time.
A runtime that cannot establish that boundary fails acceptance.
Once application handling begins, only the caller explicitly exactly retries/recovers under the owner contract.
gRPC's retry commit concept is not fact commit.
Cancellation stops waiting without rolling back committed facts;
the Stage 2 successor also retains cancellation checks at the final local execution boundary.
Lost Poll responses do not advance the client cursor or require a message bus/permanent history.

## 5. Configuration, lifecycle, and bounded resources

Service entry opens only explicitly initialized durable owner/Realm Stores.
Initialization and service startup are separate.
Missing files, identity mismatch, and Store corruption reject startup;
normal restart does not reseed permissions or generate another identity.
One process hosts only the owner declared by that slice.
Independent services do not share writable Stores, and database paths are not client addresses.

Network configuration includes listen address, advertised NextHop, expected owner,
certificate/private-key files, Realm assembly, and session/transport limits.
Controlled client configuration holds target trust material and caller credentials.
Addresses still come from specific entries/relationships, not reverse lookup through the trust table.
Listen/advertised addresses are separate; `0.0.0.0` is not a publishable NextHop.
Local validation listens only on loopback and does not automatically open firewalls.

- `SessionTTL` is required positive integer seconds, with no default duration.
  TLS/HTTP2 connection establishment, unauthenticated-message reads, each RPC, and shutdown have explicit positive timeouts.
  Clients set a deadline for every RPC; the server tightens it with configured maximum execution time.
  A caller without a deadline still cannot execute forever.
- First-test configuration uses concrete bounded values as test inputs,
  not production defaults or accepted performance targets.
- Total connections, HTTP/2 concurrent streams, service-wide execution/authentication,
  metadata size, total sessions, and per-subject sessions are explicitly bounded.
  Limiting connections alone does not permit unlimited RPCs on a channel.
  A post-decode interceptor alone does not bound memory:
  connection/stream/message receive limits constrain pre-decode use, then application admission runs.
  Slow handshakes/messages and invalid authentication have rejection/reclamation boundaries.
- The first business Protobuf request limit is 4 MiB.
  Converted P5 canonical input also retains its original 4 MiB/structural limits;
  compact wire encoding cannot bypass kernel limits.
  Message complexity/recursion/repeated entries have separate explicit checks.
  Both endpoints explicitly bound send/receive/metadata;
  maximum legitimate-result counterexamples establish response limits, and compressed messages reject.
- Owner PublicAccessPolicy and SnapshotRetention remain independently configured.
  Network enablement silently enables neither snapshots, observation, nor default retention durations.
- Expiry uses server monotonic elapsed time.
  Returned remaining time is a hint, not client-clock authorization or cut-composition evidence.
- Normal shutdown stops admission, boundedly waits for admitted requests, then closes owner/Realm.
  Forced-exit recovery follows original Store rules.
  Shutdown neither discards acknowledged commits nor combines independent Stores into one transaction.

## 6. Implementation placement and replacement boundaries

| Location | Allowed responsibilities/dependencies in this increment |
|---|---|
| `shared/values` | Minimal NextHop/shared business values; standard library only, without Protobuf/gRPC or network state |
| `shared/api`, `ucupa-core/api` | Sole shared/session and Core proto sources/public conformance; no duplicate fields/domain rules |
| `shared/bindings/go/protobuf`, `ucupa-core/bindings/go/protobuf` | Messages/stubs generated only from their proto; shared bindings do not import owner bindings; no handwritten business logic |
| `ucupa-core/transport/grpc` | Server RPC/session/message mapping; generated bindings/Core public operations/authentication ports, without Store access or Realm imports |
| `ucupa-facet/api`, `ucupa-facet/bindings/go/protobuf` | Facet protocol source/sole generated bindings; public AttachmentKey reuses shared messages, and private control evidence stays out of content results |
| `ucupa-facet/transport/grpc` | Typed content adaptation to original Facet PublicCall; authenticated content caller separate from fixed Core-check service caller |
| `internal/protovalue` | Private field conversion shared by both transports; no owner dependency, authentication, or duplicate adjudication; kernel/SDK/Composition do not import it |
| `internal/grpcbound`, `internal/grpcdial` | Connection admission/deadlines and designated-target TLS dialing shared by two real consumers; former imported only by transport, latter only by SDK; no owner state/routing |
| `ucupa-facet/sdk/go` | Generated-stub target/TLS/bounded sessions/deadlines/result checks; no owner or Core SDK import |
| `ucupa-core/sdk/go` | Actual client target/TLS/session/deadline assembly; reuse generated stubs without duplicate methods or Core implementation imports |
| `composition` | Wire Realm credential establishment into consumer ports; no authorization decisions or session/fact ownership |
| `cmd/ucupa-core` | Configuration, durable open, assembly, listening, shutdown; no duplicate domain rules |
| `acceptance` | Real subprocesses, independent Stores, TLS, and fault acceptance; fixtures are not product hosts |
| `cmd/verify` | Precisely allow actual new packages and verify one-way transport/SDK dependencies; arbitrary public subpackages remain prohibited |

Sessions belong to server transport's run-local authentication adaptation,
not Core spatial state or Realm authorization truth.
Adapter-private code is split by responsibility within its package,
without a generic gateway/host library or empty directories.
With gRPC, bindings are necessary generated artifacts with actual protocol sources,
not new product modules.
Corresponding Facet locations are established when its network slice begins.
Shared mechanisms are considered only when two actual consumers exist,
without protocol copying for directory symmetry or a speculative service framework.

Implementation pins concrete protoc, protoc-gen-go, protoc-gen-go-grpc, Go-runtime dependency versions,
and generation commands.
Temporary-directory regeneration/comparison is part of the sole verify entry;
missing tools cannot skip generation checks.
Development needs generation tools; running services does not need protoc.
Shared bindings are the engineering rules' narrow generated-artifact exception,
not permission for shared/values to depend on network libraries.
The checker opens only precise output paths/dependencies.
Proto packages/services have no UCUPA version suffix.
No REST/JSON transcoding or compatibility reader is added.
External-tool version locks are not a multi-version product design.

Implementation directly updates NextHop validation, shared schemas, and affected tests,
retaining genuinely used local adapters.
No `/v1`, old-format reader, URL→local-name conversion, or backward-compatible Store.
Any required persistent encoding change first identifies its invariants/marker in the owner contract,
rather than rewriting historical data through transport.
This contract authorizes a defined network increment,
not advance relabeling of P5/P3 local current contracts as network-capable.

## 7. First network-slice acceptance counterexamples

This is an acceptance counterexample list, not a passing record for every row.
Actual implementation, executed tests, and outstanding evidence are in the development plan's P6 delivery record:

| Counterexample/scenario | Required evidence |
|---|---|
| Caller and Core in separate local processes | Enter → create unknown UUID → Discover → same-basis Read → prepared Observe → update/delete → Poll, using only public bytes |
| Unknown CA, wrong hostname/pin, wrong Ref/Store | No credentials sent to an unauthenticated target; reject wrong owner without downgrade/redirect |
| Plaintext/h2c, non-h2, environment proxy, injected resolver service config, credentials/path in address | No downgrade/alternate target, authentication-data leak, or restored Ref routing |
| Oversized/truncated message, missing terminal status, UNIMPLEMENTED/UNKNOWN, fake-success response | Library/protocol state is neither fact success nor NOT_FOUND |
| Reordered equivalent Protobuf, unknown field/enum, missing required/oneof, NaN | First yields the same canonical operation; others reject under shape/numeric rules, not operation identity from raw wire bytes |
| Same original command retried via local JCS and gRPC | Same actor/OperationID/semantics returns original outcome; real field changes conflict, without another commit history |
| Forged Principal/CallerContext, wrong audience/secret, revoked credential | No wrong identity or existence disclosure; a valid session cannot bypass revocation |
| Permission revoked/regranted after an authorized read | Old view/continuation/observation invalidates under original rules; reauthentication cannot silently reconnect history |
| TCP disconnect/reconnect while session remains valid | Same caller establishment retained; continuation/Poll only under original conditions |
| Session expiry/release, host or Realm restart | No old-session recovery; identity/basis rebuilt, not snapshot expiry misreported as no record |
| Commit persisted, response interrupted, then process terminated | Reopen and recover confirmed outcome with the same actor/original OperationID, without a second mutation |
| Pre-send failure, REFUSED_STREAM/GOAWAY, disconnect after handler, reconnect | Distinguish safe transparent retry from handled calls; no automatic redispatch of handled calls or reset deadline |
| Returned request cancellation races with owner commit | Outcome may remain unknown; no fictional rollback; recovery returns the real durable result |
| Full sessions/connections/streams, slow messages, giant metadata, repeated invalid login, requests during shutdown | Bounded memory/connections/time, without unlimited waits; full verification includes race |
| Hidden/outside-closure updates or unauthorized calls | No extra public details/events driven by private commit frequency; status/trailers leak no evidence |
| External consumer and generation checks | Only entry/trust/credentials/public proto/stub needed, without Store reads; regeneration has no differences |
| Request-size/throughput/latency/retention measurements | Record method, load, and measurements; gRPC alone is not a speed claim, and optimization must not weaken correctness |

Full verification remains `go run ./cmd/verify`.
Network subprocess tests use the same entry alongside P1–P5 repository regressions.
Compilation, handler tests, or loopback TCP unit tests do not replace subprocess gates.

## 8. Complete P6 network-chain boundaries

These are complete-network-chain acceptance requirements.
Sections 8.1–8.6 define concrete protocols; the development plan records actual evidence.
Base transport alone does not establish them:

1. **Facet forward/reverse access:** content callers and fixed Facet service identities use distinct target-bound sessions.
   Core attachment/acceptance/contact and Facet CoreControlSource require explicit public service messages,
   retaining F0 capture/fresh-check/recheck.
   Private Core views/cursors are not handed to consumers.
   Wire/permission mapping precedes removing the slice's in-process reverse-adapter dependency.
2. **Mount control:** ApplyMount, Prepare/Finish, decision reads, and recovery each have explicit caller roles/receiver authorization.
   LocalCallers/local PrincipalRef cannot be serialized as trusted evidence.
   Before A durably reserves a slot, it must have B caller evidence required by the contract.
3. **Public federation callers:** B independently establishes the original caller's identity/authorization at B.
   No forwarding A's session, long-lived secret, or self-reported Principal,
   or substituting A's service permissions for the user.
   Fixed service-control identities serve only authorized control ports.
   Target-bound identity-transfer evidence, invalidation, and recovery are separate security boundaries;
   the first bearer session does not solve them.
4. **Nested source graph:** extend two-Core cases to A→B→C and sibling branches.
   Verify external parents, cycle rejection, matching acceptance/contact cuts,
   conservative pruning, independent-root boundaries, source deduplication, and provenance.
   UpDepth still selects semantic scope, not network-hop count.
5. **Multi-source pagination/observation and privacy:** P3 private ConnectedCut/full internal graphs cannot become public encodings directly.
   Implement D7/D10/D11 independent caller/views, disclosable sources, opaque closure, and resume.
   A new source first acquires read basis/cursor; old continuations do not insert sources.
   Unproven closure yields PARTIAL/UNAVAILABLE/GAP.
   Request deadlines/work limits are independent of UpDepth, without a generic budget module
   or silently smaller search scope under resource pressure.
6. **Interoperability and real hosts:** an independent client proves protocol consumption only.
   Complete openness also needs two implementations sharing no internal Go types, Store, or gateway code to pass the public corpus.
   Real-host gates separately record network/certificate/time/process-recovery evidence.

Base access and complete network chains are separately accepted.
Bounded validation is not universal federation at arbitrary scale or in arbitrary environments.
P7 writer/owner transfers, P8 production-operation acceptance, Stage 2 Realm contacts,
K8s, and Console are outside this P6 increment, not denied by its separately delivered successors.

### 8.1 Accepted restricted delegation and on-demand access

**The user accepted restricted delegation and explicitly authorized implementation; progress remains in the development plan.**
The first session permits a caller to access one target directly;
it does not let entry A obtain the original caller's permissions at B.
Local CallerContext carries Realm run-local proof, not cross-service identity through serialization.
A fixed A service identity cannot replace the user's B permissions.

Use explicit short-lived, target-bound delegation,
not an alternative architecture requiring applications to aggregate all target results themselves.
Evidence binds B, original caller, delegated A, permitted Anchor scope/operations, and deadline.
A gets neither the user's long-lived secret nor power to expand/redelegate to C.
This requires no advance enumeration/login to every target:
after a disclosable target is discovered through relationships,
the caller's client authenticates/acquires delegation on demand under explicit authorization policy.
Missing identity/trust evidence yields explicit failure/uncovered scope,
not B identity establishment bypassed with A's session.
This supplement first establishes target issuance/use.
Safe identity transfer for hidden sources, public source graphs, and automatic relay still require section 8's federation protocol.
Making a client directly contact hidden B is not a privacy design;
the first delegation interface alone is not transparent federation.

#### Minimum read-only delegation protocol at target Core

- Add typed `ReadDelegationService` creation/revocation methods.
  Only a directly authenticated target-user session creates delegation;
  delegation cannot create another delegation.
  Inputs are target Reference, delegated Core identity, read/list/query subset,
  observation allowance, and positive integer lifetime.
  Target Enter checks the requested entry, and every later operation checks original-caller current permissions through Core/Realm.
- Receiver startup explicitly enables the capability,
  supplying trusted delegated-Core/client-certificate-SPKI bindings, client CA,
  maximum lifetime, and global/per-subject counts.
  Bindings contain no service address and are not Ref routing or request-learned trust.
  Ordinary user connections may omit client certificates;
  delegated connections require authenticated TLS client certificates matching the bound delegate.
- After local Realm validation, derive a new delegation-specific CallerContext retaining original CredentialID/subject.
  This neither substitutes delegated-Core permissions nor establishes identity from request-reported Principal.
  Distinct contexts isolate handles between delegations.
  Credential rotation/revocation and Realm restart still fail original validity checks.
  This local context is neither transmitted nor persisted.
- Target retains bounded run-local delegation records:
  random 32-byte token SHA-256 index, original user, dedicated context, delegate identity,
  sole entry handle, allowed operations, and absolute expiry.
  Expiry is no later than the original user session and cannot extend through calls/reconnects.
  Only token, target/delegate identities, handle, and remaining lifetime are returned,
  not CallerContext, authorization view, or credentials.
  Remaining seconds round upward as a caller estimate; target uses exact absolute expiry.
  Owner handles/snapshots retain their own limits and cannot be extended by delegation.
  Releasing the original user session does not revoke issued delegation,
  whose deadline still cannot exceed that session's original deadline;
  early termination uses explicit revocation.
  A lost creation response may leave a quota-consuming delegation until expiry,
  without affecting spatial facts, automatic creation retry, or treating it as received.
- Delegated calls use `authorization: Delegation <strict base64url token>`.
  read/list/query use only this delegation's handle.
  PrepareObserve/Begin/Poll/Release require explicit observation allowance,
  with tokens still bound by the owner to that dedicated caller.
  No Enter, writes, RecoverOutcome, identity administration, or new delegation.
  This limits only the delegated chain, not users' direct-write capabilities.
- Check delegation validity before/after every call.
  Current owner authorization, prepared Read, ABA, snapshot, and GAP rules remain.
  Original users may revoke their delegations through a newly established valid session.
  Well-shaped unknown/revoked tokens return the same success without an existence oracle.
  Deleted delegation handles cannot resume.
  Best-effort owner-entry close leaves unreleased resources under original TTL/capacity limits.
  Tokens do not survive service-host restart; a new request yields a different caller/token, not continuation of old observation.
  Address changes cannot change target or borrow old trust.

Fixed Facet service-control messages retain section 8 item 1's disclosure boundaries;
user delegation cannot turn service permissions into user-content read permission.
This P6 supplement did not implement Stage 2 RealmAttachment in advance
or introduce centralized authentication/routing.
The successor notice defines the subsequently delivered contact increment.

### 8.2 Explicit identity-issuer trust (delivered)

The user accepted explicit target trust in designated identity issuers:
the target validates a short-lived proof relayed by an entry and still independently authorizes.
The entry does not self-declare user identity.
There is no overarching global Realm or trust learned from mounting.

The minimal identity issuance/validation capability belongs to Realm,
with Composition wiring network ports, not Core importing Realm.
Keys, trust, counts, and lifetimes are explicit startup configuration,
not Element data or changes to Realm bindings/Store format.

- Issuers sign only for currently valid, credential-authenticated local callers.
  Trusted-assembly callers, foreign proofs, and existing delegations cannot be re-signed.
  Proof contains only issuer Realm, subject, delegated Core,
  random target-generated challenge, issue/expiry times, and random proof ID.
  No secret, CallerContext, permission view, target address, or hidden source graph.
  Signing uses fixed Ed25519 and domain-separated canonical bytes.
- The target boundedly stores each challenge, binding target, delegate, scope, and action.
  Clients relay only the random challenge and need not know hidden target addresses.
  Issuer signatures on unknown challenges alone grant no target permission.
- Target accepts only explicitly configured issuer Realm → public-key trust.
  The issuer namespace must match exactly;
  no arbitrary subject mapping, request-provided public key, or foreign issuer mapped to local administrator.
  Signature, delegate, challenge, and bounded lifetime all match.
- Issuer boundedly retains the original caller behind each proof and rechecks original credentials for liveness.
  Revocation/rotation, Realm restart, and issuer-process restart invalidate the proof.
  Before/after use, target also checks liveness through an explicitly configured/authenticated issuer connection.
  Failure cannot permit offline access merely because a signature is unexpired.
  This establishes identity validity at issuer read time,
  not a globally atomic revocation instant across independent owners.
- Target identity establishment is followed by current target authorization/original ABA checks.
  Signatures carry no directly redeemable permissions.
  Challenge exchange, target caller, delegated-connection key, and query state are bound,
  preventing replay from another entry or continuation of old observation.
- Implementation records separately distinguish Realm issuance/validation,
  typed network exchange, and automatic federation relay.
  Passing the first does not close the whole chain.
  Without actual foreign-identity network-session establishment,
  a bare subject passed to `TrustLocalCaller` is not completed network authentication.

The concrete exchange is:

1. The delegated Core uses configured matching mTLS identity to call target `BeginIdentityChallenge`,
   specifying Reference, read-only action, observation flag, and lifetime.
   Target boundedly stores the request and generates a random 32-byte challenge.
   Only challenge/remaining time are relayed to the user, without target-address disclosure.
   Global/per-delegate challenge capacity uses `max_grants`/`max_per_caller`,
   separately counted from established delegation slots, all with hard bounds.
2. The user calls `IssueIdentityProof` through its valid issuer-Core session.
   Issuer signs only the current credential-associated subject, with lifetime no later than original session expiry.
   Returned `payload/signature` are exact-byte signing material, not arbitrary JSON business RPC.
   Payload is fixed seven-field canonical JSON under domain-separated Ed25519.
   Extra fields/noncanonical encodings reject; requests cannot switch algorithm.
3. Delegate submits the proof to the same target's `ExchangeIdentity`.
   Target consumes the one-use challenge and verifies issuer namespace, public key, delegate, challenge, and lifetime.
   It then calls `CheckIdentityProof` with its own mTLS identity through fixed configured issuer address,
   expected Core, CA, and pin.
   Service peer binding reuses explicit `read_delegation` connection trust;
   an unconfigured issuer capability cannot be called.
4. Only both successful checks let Composition establish a new target-local caller.
   Subject namespace is unchanged, without foreign-to-local subject mapping.
   Explicitly trusted issuers for the same logical Realm are also permitted;
   target Realm still checks local-subject existence and actual permissions.
   Original Core Enter validates selected scope/action and returns the same `ReadDelegation` format.
   A valid signature cannot bypass insufficient permission.
5. Before/after every such delegated use, recheck signature expiry and issuer liveness.
   Confirmed invalidity terminates delegation.
   Unreachable liveness returns UNAVAILABLE, without fabricated revocation or offline Allow.
   Recovery still requires rechecking.
   Issuer/target restart restores no old proofs/challenges/delegations.
   Exchange failure/lost response requires a new challenge,
   not old-challenge replay to obtain a new caller.
   This identity path permits no writes, re-signing, or redelegation.

These are explicit-relay identity-exchange primitives.
Automatic source discovery, hidden source graphs, and recursive query/pagination/observation
require section 8's public-federation implementation.
The primitives alone do not establish automatic entry discovery,
automatic whole-graph identity acquisition, or unconditional trust in successor Cores.

### 8.3 Fixed Facet service recheck ports

Core `AttachmentControlService` is a service port, not a consumer spatial-read interface.
Explicit startup binding associates authenticated Realm subject and Facet identity; disabled by default.
It uses a separately authenticated target-Core session, not read-only delegation
or identity established from request subject fields.
The binding limits which Facet may be requested;
original Core attachment-read/observe authorization still executes independently.
Credential/authorization validity before/after calls remains governed by owner authenticated reads/prepared checks.

`ReadControl` takes exact acceptance relationship and observation-preparation choice,
returning minimum attachment state, Core cut, public proposal comparison value, and service-only preparation.
`CheckControl` validates preparation for the same session/relationship.
Later service observation uses existing typed Begin/Poll/Release,
without putting service preparation/cursors in Facet consumer responses.
Other sessions cannot borrow them.
Network failure is not attachment absence;
response identity, Element, revision, and key must agree.
`ReadProposalControl` uses separate proposal-read permission/current caller validation
to return a minimum pending projection.
It does not prepare observation or accept unauthenticated PrincipalRef as a network caller.

Relationship writes use authenticated Core owner entry points
`InspectFacetControl`/`ApplyFacetControl`.
Consumers provide only closed propose/cancel/activate/detach/contact commands and public comparison tokens,
not private revisions, cuts, acceptance evidence, or subject fields.
Proposal-control reads require independent proposal-read permission;
ordinary Element reads do not disclose pending proposals.
Comparison tokens share the existing public attachment projection's issuance domain.
Check original caller/original authorization view before outcome reads or commit,
and recheck the same view after Activate's fixed-service round trip.
Revoke/regrant cannot refresh an old request.
Revocation after commit but before disclosure prevents success-content disclosure,
not rollback of the persistent decision.
Later reauthentication by the original subject can recover the exact result using original ID/command.
Public input resolves against current owner state or original operation history;
address maintenance does not call an unreachable old service.
This entry reuses the sole attachment transitions/durable journal,
without spatial-fact or storage-encoding changes.
Corresponding `FacetControlService` InspectFacet/ApplyFacet use the same owner decisions.

Initial acceptance uses Facet `FacetRelationshipService`:
caller submits public Core/Element/proposal, designated Core address, and OperationID.
Facet's fixed service identity calls Core `ResolveProposalControl`,
which exactly compares the public proposal with the current pending proposal for the same Facet.
No consumer-supplied private proposal key, history scan, or guessed latest proposal is required.
After cancellation/proposal change, an old token cannot obtain a new relationship.
Facet saves the original public acceptance-command comparison;
exact retry and its own Core-address maintenance do not require Core online.
Original Facet caller/view checks occur before/after external waits and under the sole commit lock.
Service identity does not replace acceptance-user authorization.

During activation, fixed Core service identity calls Facet
`FacetAcceptanceService.ReadAcceptance`,
checking complete relationship, Facet cut, and accepted-at.
Disconnection does not mean unaccepted.
Facet `core_services` configuration limits fixed Realm subjects to expected Cores
and still independently requires acceptance-read permission.
Optional Core `facet_connections` and Facet `core_connections`
select exact connections only by relationship-designated address.
Each holds expected owner, CA, pin, and fixed-service credential file;
no reverse lookup by Ref.
Address maintenance changes only original relationship contact CAS,
not proposals, acceptances, or content facts.

Facet host gets fixed-service credentials from explicitly configured local files as needed.
SDK reuses them while the original session remains valid.
Conservative local expiry, CredentialID changes, or explicit old-session rejection
cause only the next explicit call to reestablish a session,
without automatically retrying failed business requests.
Rejection clears only the used old session, not a concurrently established new one.
Original preparation/cursors remain caller-bound and cannot continue with a new login.
Early target restart leaves the current failed request unavailable.
Connection configuration must match relationship address/expected Core;
neither Ref lookup nor temporary local objects repair failures.

Service revisions explicitly encode nonzero 32-byte incarnation and nonzero uint64 sequence.
Relationships also carry Core, Facet, and Element; Element determines the attachment-revision subject.
Core cut has a separate, noninterchangeable message.
This is exact evidence encoding between authorized services,
not consumer public Revision, serialization of local Go objects, or global time.
Unknown fields, malformed evidence, and unknown states reject;
zero values cannot manufacture success.

### 8.4 Mount-write approval (delivered)

Local ApplyMount LocalCallers cannot be serialized as network identity.
Sections 8.1/8.2's accepted delegations are explicitly read-only and cannot become mount-write permission.
The user explicitly accepted preservation/verification of the original requester and independent target authorization
and authorized implementation.

Where the original mount contract requires carrier B caller evidence,
the caller authenticates directly to B using B's existing Realm permissions
and requests short-lived approval bound to target B, source A, OperationID, exact relationship, and command.
A independently checks its own original caller permissions and verifies B approval before occupying a control slot.
Approval cannot be redelegated, change command, derive user permissions from service identity,
or authorize general content/spatial writes.
Read-only delegation is unchanged.
Operations whose original contract requires no new B approval do not gain that precondition.

Implementation must cover one-time use, expiry, lost responses,
recovery after durable prepare, revocation, service identity,
and corresponding concrete protocol/counterexamples.
Approval timeout is not relationship revocation;
short-lived credential expiry cannot prevent safe closure of a durable decision.
Acceptance of this direction alone proves neither network implementation nor verification.
Short-lived approval verifies the original request, not a new authorization model.

Carrier authenticated owner entry `PrepareMountControl` receives source identity established by the service adapter
from the authenticated connection and original requester CallerContext from the target-authentication boundary.
ParentActor/CarrierActor message fields cannot establish a caller.
CarrierActor equals caller.Subject exactly.
Target obtains the view under original MountAccept permission/exact Anchor target,
then rechecks credentials/view under the owner lock before reading an existing prepare outcome
or executing the sole persistent transition.
Denied authorization, ABA, cancellation, or capacity exhaustion occupies no carrier control slot and changes no fact.
Exact retry reuses existing durable prepare without reclosing gates or adding a cut.
Same OperationID/different request still conflicts.
This entry neither issues approval nor validates network TLS;
it does not by itself replace approval exchange/service-connection adaptation.
Durably decided Finish/recovery does not revert to Prepare or require expired user credentials.

A separate `MountApprovalService` handles approval exchange,
enabled only by explicit `mount_approvals` configuration.
Service mTLS peer trust reuses configured connection identities,
not read-only grants or their permissions.
`CreateMountApproval` accepts only a direct user session,
recording target, delegated actor, Element, OperationID, and a 32-byte commitment to the exact command.
It retains original caller/MountAccept authorization view without returning the view on the network.
Lifetime cannot exceed remaining user-session lifetime.
Global/per-subject counts have hard limits; approvals left by lost responses expire.
`CheckMountApproval` accepts only that actor's authenticated mTLS connection and identical binding,
not a user Bearer or read-only delegation.
Every check validates original credentials/view;
revoke/regrant cannot refresh old approval.
Returned original Realm/subject are attribution for A, not a trusted CallerContext at A.
`RevokeMountApproval` requires directly authenticated original subject;
a new login by the same subject may revoke its old approval,
without disclosing other subjects' token existence.
Expired/restarted/revoked tokens do not recover.
Unavailable and unauthorized outcomes remain distinct.

Check validates before slot occupancy; it does not consume approval or write on repeated checks.
Actual Prepare also matches command commitment,
validates the captured view at the carrier commit boundary,
and binds the sole durable request.
Check success is not a cache of unlimited future write eligibility.
`MountPeerService` has separate typed Prepare, Finish, and ReadMountDecision RPCs.
Service identity derives from explicitly configured mTLS keys;
subject fields only attribute the original request.
Prepare validates approval/command, serializes local approval revocation during commit,
uses approval expiry as deadline, and revalidates the original view under owner lock.
Revocation after approval checking cannot be bypassed.
Finish/decision reads reuse existing relationship/operation recovery, without new user approval.
ServiceRevision/ServiceCoreCut explicitly encode incarnation/uint64 sequence,
not Store encoding or ordinary consumer tokens.
Parsing proves shape, not authorization or fact issuance.
Revoke may omit CarrierActor under the original contract;
network adaptation adds no B-user approval requirement.

At the peer-port implementation checkpoint, the three peer RPCs were connected,
while parent authenticated application entry, production two-way connection assembly,
and the process-recovery chain were still required.
That checkpoint was not the complete A→B mount-application write loop.
The subsequently implemented chain and its evidence are described below and in the development plan.

`MountCommandCommitment` computes SHA-256 for canonical MountEstablish/MountUpdate.
Input is `ucupa.mount-command`, one zero byte,
then UCUPA canonical JSON for parentCore, carrierCore,
operationId (base64url), element, kind (establish/update),
carrierNextHop, parentNextHop, expectedElement, expectedMount, and final.
Revision is `[base64url incarnation, ElementUUID, decimal sequence string]`,
or null for absence.
Absent final is null; otherwise it contains parent, existing canonical pose, and radius.
Invalid omitted fields still reject under original command validation.
Address, comparison basis, final spatial tuple, operation ID, and both identities participate in binding;
raw Protobuf bytes are not semantic input.
`PrepareApprovedMountControl` validates original approval view under owner lock
without acquiring a new view to repair ABA,
then reuses original prepare transition/durable exact retry.
Original local authenticated prepare is not a fallback for network approval.

Parent `AuthorizeMountControl` captures the original user's required-permission view for this command.
`ApplyMountControl` consumes it and invokes call-bound
`MountControlTransport.CheckApproval` at the designated carrier address before original reserve.
After the network round trip, the reservation lock rechecks original view,
actual relationship address, and its revision.
A contact replaced during checking cannot reserve a slot.
After Prepare returns, parent rechecks original view under the decision lock.
Confirmed revocation/ABA may decide Abort and close the exact request through the original flow;
an unavailable authorization dependency is not denial and cannot invent an Abort.
Disclosure still checks original authorization.
Exact retries of decided operations use retained B-subject attribution without reissuing B approval.
Pose-only updates/revocation still require no new B approval.

`MountConnections` chooses preconfigured pinned SDKs only by relationship-designated address.
`WithApproval` creates an immutable per-call binding,
not a process-wide write-authorization cache.
An unapproved connection may Finish/ReadDecision, not Prepare.
Autonomous recovery through this network port cannot continue undecided writes from persisted Principal names.
The original user must reauthenticate and exactly retry;
durably decided operations continue Finish/Settle.
Recovery does not implicitly create callers, refresh views, or restore expired approvals.
Parent authenticated owner entry/production network adaptation are implemented.
Ordinary caller Inspect/Describe/Apply gRPC, independent-process host assembly,
and network contact repair are connected.
The development plan records targeted crash/restart evidence before/after Prepare/Finish.

Ordinary control comparison values use owner MAC tokens:
Element `record-revision` and independently domain-separated mount `mount-revision`.
ServiceRevision, incarnation, sequence, cut, and peer admission keys are not public control responses.
Parent `InspectMountControl` projection requires the original user's MountRead.
`ResolveMountControl` requires corresponding control permission
and resolves only current matching tokens or retained operations' exact original commands for a trusted adapter.
It creates no control slot, writes no fact, and grants no later execution permission.
Retained operations match original subject, operation ID, and every command field;
different subject/command conflicts.
Apply rechecks captured original view.
Fact changes after resolution are still decided by CAS on the sole commit path.
Token MAC keys persist with owner, keeping comparisons stable on normal reopen.
Invalid/cross-owner/cross-purpose tokens cannot substitute for comparisons.

That owner conversion is implemented and tested through disk reopen.
MountControlService Inspect returns public comparison tokens.
Describe creates only target/exact approval binding,
not authorization, control slots, persistent intent, or frozen future permissions.
Apply recaptures view from this request's original user session,
checks B approval, and calls the sole owner commit path.
Services assemble only MountConnections without per-call approval;
user approval is not global configuration, and read-only delegation cannot become control access.
SDK checks response Element, operation ID, approval source/target, and decision state against the request.
Network outcomes contain no internal revision/cut.

Real TLS black-box tests cover establishment, range updates, pose updates, revocation,
and exact retry from ordinary-user network entry.
Host `mount_connections` assembles fixed-TLS-pin peer SDKs at designated addresses without user approvals.
After listening is established, startup runs one recovery traversal bounded by RPC deadline.
Shutdown cancels/waits for it before closing dependencies.
Undecided network operations do not refresh original authorization;
durably decided operations may settle, and temporary unreachability retains recoverable state.
Two independent product processes verify network establishment,
exact retry after forced termination/restart, revocation, and another restart after revocation.

MountContactService Inspect/Repair support both directions.
Original users repair under MountControl/MountAccept respectively; reading still requires MountRead.
Comparison tokens use an independent mount domain within contact-revision,
bound to relationship, direction, and incarnation.
Facet contact tokens, opposite directions, or another mount's version cannot substitute.
Repair waits for no peer and occupies no control slot,
remaining possible during disconnection/paused range operations.
Original subject/operation ID/comparison/new address must match exactly for retry;
old responses do not overwrite current addresses.
The original persistent address-change transition remains,
without changes to spatial facts or carrier ownership.
Tests cover authorization ABA without writes, two-way repair during pauses,
and rejection of old comparisons, changed direction/subject/retry.
Real two-process tests cover address changes/repair while the peer is stopped,
reverse contacts, and subsequent restart.

Independent-process mount-establishment counterexamples also cover forced exit
before/after durable Prepare and before/after durable Finish.
A is the product host; faulting B uses production owner, Realm, and gRPC adaptation,
injecting process exit only at those boundaries.
Recovery uses the normal product host, not fabricated prepared/decision evidence.
Startup recovery does not automatically decide pending operations:
the original user reauthenticates and gets a new approval for the same commitment.
Committed operations retain the decision and settle without new approval.
A service-identity channel independently reads original durable decisions.
Lost responses are not noncommit evidence.
These cases do not replace range-update/revoke/authorization counterexamples
or Facet/multi-level-query fault matrices.

### 8.5 Public federation source projection and composition (delivered)

Public access continues to use Enter, Read, ListDirect, and QueryRegion,
without requiring clients to traverse an internal source graph first.
Entry Core coordinates through verified mounts,
using section 8.2 original-requester proofs for each source Core's independent caller/view establishment.
Source accepts only bounded read bases bound to target, coordinator, original caller, and request digest.
Query, List, and Read each check their own permissions;
internal path projection cannot implicitly upgrade Query to Read, List, or MountRead.

- Source projection has closed path/read/read-at/list/query shapes.
  Paths contain only necessary parent edges/Poses.
  Query gives matching summaries, required intersecting mounts, and explicit gaps,
  not an entire subtree's Element/Facet facts.
  Read returns same-cut Element/Attachment.
- Query source projection carries fixed 32-byte `coverage`.
  During actual scanning, source deterministically records local-closure adjacency,
  mount prune/continue/unknown decisions, and candidate intersections,
  then commits with a private random basis and domain separation,
  binding cut, original authorization view, and exact projection conditions.
  Adjacency follows the same direct-parent membership filter/UUID pagination as ListDirect:
  each parent Anchor records complete pages of direct children,
  including a complete empty page for an empty parent and the last-page complete marker.
  A child Anchor outside the traversal closure, missing query root, or canceled request cannot produce evidence.
  This is temporary private projection at a fixed cut, not a second index.
  It neither calls public List, upgrades Query permission, nor publishes private adjacency transcripts.
  Internal traversal digests do not directly leave Core, and no second persistent index exists.
  Evidence is independent of page size/continuation:
  source executes the same complete local-candidate checks at that fixed cut on every page.
  It does not claim remote-branch closure.
  Nonzero `Gap` remains incomplete; nonzero coverage cannot conceal failure.
  Non-Query projection must not carry the field.
  Coordinator captures original coverage during planning;
  subsequent scans match it exactly and bind it into aggregate closure.
  Without an actual Query scan plan/nonzero evidence, there is no aggregate query closure.
  Enumerating visited owners alone does not prove closure.
  SDK rejects missing/all-zero/non-32-byte Query evidence.
  The field also enters public sourceGraph query-source nodes,
  but is not geometric evidence consumers can independently recompute apart from an authenticated source.
- Source read parts also bind original PrincipalRef,
  request-local cut stamp, and authorization-view stamp.
  Actual source salts both stamps with random private basis,
  binding owner, boundary, normalized purpose, and original cut/authorization establishment.
  They disclose no Store scope/serial, AuthorizationView, credential, or Proof,
  and do not link equal values across separate captures.
  Coordinator checks returned subject equals original caller;
  both stamps are bounded nonzero fixed-length values, without old-shape compatibility.
  They are authenticated-source opaque evidence, not independent signatures,
  access credentials, or extra authorization; source still checks actual validity.
  This evidence is delivered over source gRPC and bound into public source graph/aggregate closure.
  Coordinator retains a private rooted source-evidence graph:
  each node fixes the original part, and children retain both parent outgoing and their own incoming evidence.
  Nodes enter only through verified parent edges.
  Before/after closure, continuation, and observation checks,
  original subject, purpose, cut/view stamps, preparation,
  and direction-independent contact evidence must remain unchanged.
  The graph provides neither new address resolution nor automatic owner/topology disclosure permission.
- `read-at` is an independent exact read at an existing basis, not a Query/List permission upgrade.
  Source separately acquires/revalidates Read view,
  retaining the first successful Read view per Element within that basis.
  Later use cannot reacquire authorization to bypass revoke/regrant.
  This association is bounded by source capacity/original-basis lifetime,
  without expanding query authorization or changing its observation preparation.
  A derived public receipt borrows only the already closed source set,
  without recapturing sources or extending snapshots.
  Read authorization is continually checked through the derivation chain.
  Closing/expiry of original basis invalidates derived bases;
  derived reads do not independently own/release the original composition.
  A network Query/List grant does not itself permit `read-at`.
  Source also needs independent request proof bound to this exact Read.
  The subject having Read permission does not replace delegated operation scope.
  Source issues a one-use challenge bound to original grant,
  authenticated coordinator, original basis, and target Element.
  Entry obtains/submits a new proof for the original requester.
  Challenge/proof lifetimes cannot extend original session, source lease, or snapshot.
  Source verifies identity/original authorization before/after execution;
  replay, target substitution, or grant substitution rejects.
- Coordinator retains exact per-segment Pose composition,
  without adding structural depth at mounts or first rounding accumulated projection into one floating-point Pose.
  UpDepth selects Scope from Start's verified Anchor chain.
  Basis belongs to that Scope, then intersecting descendants are traversed.
- Each source first captures its cut.
  After all necessary sources are collected, coordinator rechecks each captured cut is still current.
  All captures preceding all rechecks proves those cuts once coexisted,
  without global clocks/cross-owner transactions.
  Mixed retention configurations cause all sources to be recaptured as CurrentVerified before page one;
  their Retained bases are not reused.

- Source set closes before page one.
  Continuation retains original sources, conditions, scope, and UUID order.
  Previously unreachable/unaccepted branches are not inserted later.
  An unreachable relevant descendant within defined scope creates an identity-free/count-free coverage gap,
  not an empty query.
  Invalid Start, Basis, or entry basis produces no query result.
  Authorization/contact invalidation at accepted sources does not silently switch to latest.
- Entry issues aggregate receipts/continuations bound to internal composition,
  while disclosed record Revisions still identify actual fact owners.
  SDK accepts that ownership difference only in explicit read-result records,
  not mutations, handles, service grants, or continuation tokens.
  Aggregate Read/List/Query also return fixed 32-byte `closure`,
  absent from local nonaggregate results.
  Using a source-private random basis as key, it domain-separates commitment to original request,
  caller establishment, scope/actual depth, coverage gaps, and closed source graph.
  Nodes bind original cut/view stamps, source basis, and both mount/contact sides
  without exposing graph contents/counts.
  Continuations retain the same commitment.
  Independent queries recapture sources rather than using a linkable global digest.
  Read through a historical receipt retains original composition closure;
  added exact-Read authorization is independently bound/validated by its derived receipt.
  Closure grants no permission/offline proof and cannot turn PARTIAL/unknown branches into COMPLETE.
  SDK rejects present-but-empty/all-zero/non-32-byte markers;
  authorization/source checks still run before every return.
  Aggregate markers also bind hidden-branch owner-local denial closure.
  Public `sourceGraph` and closure are both present or both absent.
  The graph projects only authorized sources in parent-before-child order,
  with owner, boundary, original PrincipalRef, cut/authorization stamps, and readMode.
  Nonroot nodes give parent owner/mount Anchor
  and direction-independent delegation/acceptance commitments;
  Query sources also give queryScope/coverage.
  Those commitments bind actual scan/bilateral relationships,
  not manufactured evidence from a visible-owner list.
  Hidden sources produce no nodes, counts, or placeholders.
  Private addresses, bases, grants, and credentials stay out of the public graph.
  Pages/derived Read retain the original graph without recapturing sources.
  SDK rejects duplicate owners, a root other than entry,
  absent prior parents, missing matching evidence,
  Query without scan evidence, and record owners absent from the graph.
  Query-source nodes also carry `parents`:
  parent Anchors disclosable on scope/all matching paths, sorted by UUID,
  each with parent and adjacency.
  Source derives domain-separated adjacency commitment from private basis, original coverage,
  parent, and that parent's complete direct-adjacency page chain.
  Private complete chains include empty/final pages and continuation,
  but child lists, page counts, and internal cursors are not directly disclosed.
  Traversal alone does not publish parent Anchors of nonmatching branches.
  Scope at external roots/mount boundaries is already disclosed query context.
  The set does not depend on this page's match count.
  Later Retained-mode writes do not replace evidence;
  subsequent source scans replacing any parent's evidence reject.
  Source/public SDKs cross-check matching paths:
  scope belongs to the declared path; every parent below it has corresponding evidence;
  an Anchor match also has its own adjacency evidence.
  Duplicate paths, Entity used as scope, or missing evidence reject.
  Public COMPLETE Query sources cannot lack parent-Anchor closure;
  PARTIAL sources unable to form cut-based computation cannot fabricate evidence.
  Prefixes above scope belong to other sources,
  not forcibly included in this source's proof/permission scope.
  Maximum 4096 entries per source.
  Collection before/after is bounded by owner temporary memory/coordinator composition capacity;
  excess fails explicitly, not a truncated complete result.
  Query proof does not authorize List:
  raw adjacency pages and nonmatching pruned edges are not disclosed through evidence,
  remaining bound into coverage/opaque closure.
  The graph is an authenticated entry's source-composition statement,
  not an independent signature/offline geometric proof.
  The development plan's D10 reconciliation distinguishes public/internal undisclosable evidence;
  field checks do not replace behavior/fault validation.
- Source observation reuses the stream prepared when its cut was captured.
  Original source grant calls Begin/Poll without later observation-permission acquisition or cut recapture.
  Source binds basis, original caller, authenticated coordinator,
  and preparation/cursor before invoking the sole observation queue.
  Ordinary writes need not leave the old cut current.
  Authorization changes, invalid relationships, queue gaps, and cursor substitution follow original observation rules.
  Source cursors/change groups are coordinator-private evidence,
  not direct public outputs for hidden sources.
- Address-based connection assembly configures expected targets/TLS trust without bare-Ref resolution.
  Source counts, composition lengths, temporary outputs, and held resources are bounded;
  capacity exhaustion fails explicitly.
  Close/expiry first cancels disclosure/in-flight requests, then boundedly releases sources.
  Resources still count until reclamation finishes;
  close cannot bypass capacity or retry original business operations for cleanup.

Public multi-source query/pagination, structural-path exact reads,
and direct listing are connected.
Owner multi-level counterexamples and real TLS public-entry tests have run.
Independent-authorized exact reads at historical aggregate bases are connected to public owner Read.
Network independent-Read proof exchange is connected and verified over real TLS.

Query/List multi-source prepared Observe connects public preparation→Begin→Poll→Release.
Entry holds public cursors; source cursors remain private.
Change groups from authorized sources retain their own owner/basis.
Public page size does not change source batches.
Retry may take differently sized prefixes of the same batch;
acknowledged old cursors return GAP.
Every return, including cached retry, revalidates original source observation authorization/relationships.
Releasing preparation also reclaims related receipt source leases.
Unclosed potential branches are not packaged as a completely observable source set:
preparation fails rather than downgrading to single-source success.
Real TLS public SDK tests cover post-write observation,
changes between initial cut and Begin, source identity,
revoke/regrant, and repeated Poll.

Exact-Read multi-source preparation is also connected:
it binds the actual disclosed path at original cut;
intermediate sources observe only path facts,
and the terminal source additionally observes exact Element/Attachment.
Unrelated Element writes do not disclose change frequency.
Path interests count against bounded capacity at preparation and cannot expand after Begin.
Additional binding requires original cut still current,
preventing omission of changes before binding.
Read through an existing historical receipt cannot request new preparation:
it returns INVALID, consistent with the local contract's observation from current reads,
not retrospective fabrication of uncaptured history.
Opaque closure, public source graph,
and a three-Core independent-product-process query/observation/disconnect-recovery matrix
are connected and pass targeted tests.
The public graph incorporates complete-adjacency closure commitments for disclosable Query parent Anchors.
Mainline D10 reconciliation/evidence entry points are in the development plan's closure record.
This subsection's evidence alone is not complete P6 acceptance;
final evidence must cover all section 8 and delivery gates.

### 8.6 Hidden-branch denial views and silent observation

Established lack of discovery permission differs from unverifiability.
Trusted Realm `CaptureDenialView` issues `AuthorizationDenialView`
only for a verified caller missing one exact requirement.
Its type/MAC domain is separate from Allow views.
Allowed requests, forged/invalid identity, or invalid requirements yield no denial evidence.
View binds original caller establishment, requirement,
and that binding's latest change epoch, not whole-Realm revision.
Unrelated authorization/spatial writes cannot turn it into a hidden-change counter;
added authorization and grant/revoke ABA invalidate old evidence.

Trusted source ports Core `CaptureFederationDenial`/`CheckFederationDenial`
apply only to denied Query/List themselves on an established incoming mount.
Missing Observe permission does not justify hiding discoverable content,
and exact-Read denial does not become NOT_FOUND.
Evidence binds actual Core, coordinator, caller, original request,
denial view, and original incoming contact/acceptance state,
with lifetime at most Core ContextTTL.
Validation still needs the original request and cannot borrow across queries.
Changed relationships, expiry, or changed view explicitly invalidate it.
Private fields are neither public messages, access credentials, nor another persistent truth.
Network adaptation keeps it behind bounded authenticated context
and rechecks original identity/online proof before/after use.
Clients cannot access denial views or hidden-source identities.

Source `FederationReadPart` has required `hidden` to distinguish ordinary projection from denial closure,
returned through existing bounded grants/original-identity proof chains,
without new services, credentials, or resolvers.
Hidden parts retain only denial basis/incoming relationship,
not spatial snapshots/fact queues.
Ordinary projection/exact Read cannot use them to obtain facts.
Coordinator includes them in original closed graph/fixed closure, not result scan plans.
Only a continuously valid denial basis permits Query/List to exclude an undisclosable branch;
connection failure, invalid identity, and invalid relationship cannot establish exclusion.

For prepared observation, hidden-part Begin/Poll only validate original denial view/relationship
and return a fixed private cursor/empty event set.
They neither subscribe to spatial writes nor refresh deadlines,
and hidden-update count/frequency produces no public events.
SDK/coordinator reject events returned by hidden parts.
Denial-view changes, including grant/revoke ABA,
invalidate original composition without automatically adding newly visible sources.
Requery is required to capture a new composition.
Real TLS public SDK counterexamples cover stable closure on continuation after hidden writes,
identical empty-observation retry, authorization ABA invalidation,
and source disconnection still producing PARTIAL/UNRESOLVED_COVERAGE.
This does not replace remaining public-source evidence disclosure or full P6 gates.

## 9. Engineering references and status

Mechanism references: [gRPC methods/lifecycle](https://grpc.io/docs/what-is-grpc/core-concepts/),
[authentication](https://grpc.io/docs/guides/auth/),
[deadlines](https://grpc.io/docs/guides/deadlines/),
[status codes](https://grpc.io/docs/guides/status-codes/),
and [retry](https://grpc.io/docs/guides/retry/).
Go retry/resolver configuration follows [grpc-go API](https://pkg.go.dev/google.golang.org/grpc#WithDisableRetry)
and the chosen pinned implementation.
A general guide's disable-retry wording does not prove transparent retries disabled in a language runtime.
[Protobuf noncanonical serialization](https://protobuf.dev/programming-guides/serialization-not-canonical/)
constrains raw-byte use; TLS validation uses [Go crypto/tls](https://pkg.go.dev/crypto/tls).
Implementation checks actual pinned dependencies/toolchains first,
not assuming a repository upgrade from documentation-site versions.
These sources replace none of UCUPA owner, recovery, or permission evidence.

The user confirmed contract commit/P6 implementation authorization.
Section 8 increments require concrete protocols/validation; authorization alone proves no correctness.
Implementation, fault verification, user acceptance, commit,
and real cross-host acceptance are recorded separately.
Missing environments need not prevent development,
but design reasoning/existing local tests cannot establish network implementation acceptance.
