# UCUPA engineering organization rules

**Status: the sole current engineering-organization specification in this public source snapshot.
Core, Facet, Realm, transport, SDKs, and tests retain their accepted responsibility/dependency
constraints. This document does not reopen refactoring. Only the development plan records current
scope and the next gate; these rules authorize no new code, commit, or push.**

This is the sole entry for engineering organization, not spatial, permission, contact, or content
semantics. The [authority manifest](docs/architecture/README.md) identifies axioms/contracts;
the [plan](docs/architecture/development-plan.md) records implementation, validation, and next gate.
These rules replaced the old requirement to keep public types, instances, commits, and tightly
related implementation together in module roots. Source relocation follows authorized scope;
accepted behavior and reliability constraints remain.

The increment-specific explanations below retain accepted dependency/responsibility decisions,
not a public development journal or scheduled work. Historical counts/locations do not replace
the current checker or authorize wider dependencies, capacity increases, or repeated migration.

## 1. Objectives and engineering responsibility

A maintainer should find a complete responsibility, understand its dependencies, and make bounded,
verifiable changes. Few directories, short files, many abstractions, or passing tests alone do
not prove a sound structure.

- Establish fact ownership, change boundaries, and dependencies before packages/files.
  Do not create directories per method or step.
- One logical owner has one authoritative state, commit path, and recovery mechanism,
  not necessarily one Go package.
- Define each model once. Normal execution and replay share rules, not parallel kernels.
- Real consumers determine the public surface; relocation convenience does not make private
  implementation public.
- Choose placement before adding files. Remove obsolete implementation/transitional adapters
  when finishing a slice.
- Moving a giant root package unchanged into `service/`, `runtime/`, `kernel/`, or
  `internal/` is not completion.
- Greenfield uses the accepted single interface/implementation, without old-import compatibility,
  parallel entries, or protocol fallbacks. It does not permit silent semantic changes.
  Record/verify organization, interface, and domain changes separately.

The implementer owns design, naming, decomposition, verification, review, and routine execution;
the user is not asked to choose directory names or replace technical audit.
The user confirms scope and consequential behavior, calling, security, availability, or cost tradeoffs.

P7.2 parent carrier public control (2026-09-19): `internal/transfer/carrier.go` captures/rechecks
parent+mount commit/inspect permission through original Admission and holds only assembled
commit.Owner. The 127-line `commit/public_carrier.go` compares current/original input and projects
outcome/cancellation under the original lock. It reuses mount slots, intent, and cancellation
transactions, adding no table, journal, codec, network, or second state.
These comparisons require current mount, original operation, and durable signer under one lock;
another state owner is not a valid way to reduce file count.
`token.CarrierRevision` uses a fixed domain/original durable signing material, binding reservation
and current parent cut. Shared token schema/Protobuf remain synchronized without private sequence
disclosure or new authentication.
Persistence faults, reopen, outcomes, slot exclusion, and projection tests stay with commit.
Public Realm revoke/regrant, cancellation, and original-caller chains belong to `tests/transfer`,
without business-private imports.
Four consumers have exact dependency registration. Limits at this increment were commit
62 production files/8,427 lines and 113 test files/17,052 lines; checker 28 production files/4,371 lines.
No allowance was reserved for later networking or codecs.

The same day's parent network increment reuses `transport/grpc/internal/transition` and user
session lifetime. `carrier.go` converts public requests and checks comparisons/response binding;
it imports no business internal package or recovery workflow.
SDK carrier validation checks target/original-request agreement only: no state, permission grant,
or automatic retry.
TLS counterexamples stay in `tests/transfer`; independent-parent restart tests stay in acceptance.
The latter establish a mount through real public owners, close them, then start the product,
without replacing product transfer calls with private methods.
Those four consumers have exact registration. Checker limit became 28/4,375;
acceptance 71 test files/10,253 lines. Core commit, Facet, and root limits were unchanged.
This increment alone did not establish complete three-party recovery.
Design, implementation, verification, user acceptance, commit, and push remain distinct.
Discussing rules authorizes no code, later phase, commit, or push.

The same day's local three-party recovery increment splits parent recovery, source write stop,
target accept/install, approvals, and evidence reads into files within `internal/transfer`.
It holds only original Admission/commit.Owner, not workflow tables, locks, or journals.
The 70-line `commit/carrier_peer.go` checks original plan, existing fence, installed outcome,
and external edges without holds under the original lock; no new transaction/codec.
Opaque envelopes reuse `contract/transition` → `transfer/exchange`, exposing no forgeable
completion proof. Trusted local connections fix three instances, addresses, original caller,
and approval views. Private source binding determines reverse identity; requests cannot override
it or automatically refresh approval.
Public black-box `tests/transfer` uses real independent Stores and target Realm for lost replies,
all-party restart, revoke/regrant, and concurrent retry, without private-state result fabrication.
12 consumers have exact registrations; limits became commit 63/8,497 and checker 28/4,387.
Roots, Facet, Store format, and other limits stayed fixed. This increment alone did not
establish three-party gRPC or external-edge recovery.

The same day's parent-evidence network increment shares request shape in
`transport/grpc/internal/wire/carrier.go`, not separate public/service validators.
`transitionwire` converts original plan/decision and finite cuts/revisions, with no standard-library
I/O, state, locks, Store, signer, or network; persistent JSON is not reused as wire protocol.
`transition/parent.go` handles real-peer mTLS, owner evidence reads, and final disclosure recheck.
`transitionconn/parent.go` reuses Connections address/concurrency/deadline, without registry/routing.
SDK checks request/response, not business internals or Realm approval.
Composition tests remain `tests/transfer`.
12 consumers are registered; checker limit became 28/4,399, other limits unchanged.
No directory, public business operation, or persistent transaction was added.
This boundary alone was not full three-party recovery.

### S2P3c network Realm consumers (2026-10-02)

Facet authorization waits remain in original `internal/namespace` workflows.
Request-local permit files retain caller, exact requirements, contact generation, and Realm view;
no permit table, owner, lock, or journal is introduced.
Namespace-replacement source/import each use one same-package file and shared state/commit,
not a package per step. Access schedules original context-release callbacks outside locks;
it owns no grants/decisions. Transport passes authenticated CallerContext; instance maintenance
uses that identity rather than remote authorization by principal name.
Products select local/network explicitly and reuse composition's fixed TLS assembly,
not new authorization/connection kernels.

Production dependencies stay within reviewed boundaries.
Namespace registration is 30 files/5,507 lines, 215 lines above the prior frozen set.
Acceptance adds one shared network-Realm product fixture, reusing relation/recovery/transfer
scenarios rather than duplicating workflows: 95 files/18,069 lines, net 270.
These register actual responsibilities, not future matrix/scale capacity.
Single-file/dependency limits remain. Checker counterexamples exceed the current file limit
by exactly one and still reject unregistered generated/inactive sources hiding size.
That self-test adds one line: checker tests 58 files/11,272 lines, without deleted rejection assertions.
Only the plan records full verification, acceptance, and commit.

### S2P3d bounded combinations (2026-10-02)

Four same-package acceptance files reuse Core/Facet relations and cross-Core queries:
combination entry, bilateral permission failures, cross-Core network-Realm assembly, and
byte-level response loss. Explicit sharing uses one original Realm Store/product and two
service certificates. Independent combinations retain separate Stores/grants.
No copied relation/federation implementation, scenario framework, fact table, or management RPC.
Exact registration is 99 files/18,627 lines, net 558 over S2P3c, without future allowance.
File/dependency gates remain. P6 foreign Actor/receiver-owner wiring fixes stay in original
composition/Realm boundaries; fixed tools generate protocol fields, with no new persistent
format or second authorization model. The plan alone records technical delivery/full verification.

## 2. Refactoring baseline evidence

These counts came from restored P6 commit `a8e84ed8`, not the old P7 workspace:

| Module | All files including descendants | Root production Go files | Root Go test files |
|---|---:|---:|---:|
| Core | 234 | 96 | 68 |
| Facet | 63 | 20 | 16 |
| Realm | 32 | 15 | 16 |

“Production” means non-`_test.go`; generated files are separately recognized in whole-module
counts. Counts guide review, not business complexity. Problems at that baseline were:

1. Core instance, spatial values, Element operations, mounts, Facet contacts, snapshots,
   public access, and Store shared one root package. `convergence.go` also combined public
   tokens, fact tables, receipts, commits, and fault recovery.
2. Facet public values, content, acceptance, Core checks, and commit shared one root.
   Splitting files alone creates no dependency boundary.
3. Many methods belonged to one public receiver; Go cannot move its methods into other packages.
4. Keeping all public types/commits in the root makes internal reverse dependencies cyclic.
   Whole-model copies or per-method callbacks to avoid this worsen structure.
5. Transport needs review by request category, session, and connection lifetime too.

P6 is an executable regression baseline, not proof of ideal organization.
Previously extracted P7 directories do not automatically define this target.

## 3. Module shell and public surface

### 3.1 Shared conventions, actual responsibilities

The placement convention below is not a mandatory template.
Core, Facet, and Realm follow common rules; their internal business packages need not be identical.

```text
<module>/
  README.md                 responsibilities, entry points, implementation status
  instance/                 construction, open, assembly, lifecycle
  contract/                 genuinely shared public Go values, requests, results, ports
  internal/                 implementation grouped by responsibility
  api/                      cross-process protocol sources, schemas, conformance
  bindings/<language>/...   sole generated protocol bindings
  transport/<protocol>/     server communication entry and private implementation
  sdk/<language>/           consumer client and private implementation
  tests/                    module public composition tests where justified
```

Module roots contain no business source, regression tests, old aliases, or forwarding entries.
README, necessary resource indexes, and tool-required metadata may remain.
A tidy root is a result, not responsibility acceptance.
One repository-root `go.mod`/`go.sum` remains: no submodules, `go.work`, `replace`,
or independent release units.

`instance` constructs/assembles dependencies and real local entry points; it is not another
large business package. Concrete types versus interfaces follow consumer/encapsulation needs,
not a universal “capability interface” rule.
Requests belong to their responsibility, not a hundred proxy methods here.
Internal release/recovery belongs to its implementation, not all in constructors.

`contract` is not a universal model package. It holds existing externally needed Go definitions,
not Store documents, writable facts, locks, internal commands, replay events, or whole owner state.
Implementation and real consumers share one definition, not field-for-field internal mirrors.
Protocol types remain generated and convert only at real transport boundaries.
Group definitions by Element, query, relation, etc.; split packages only for actual consumer/
dependency boundaries. Prefer narrow interfaces at consumers; only genuinely shared public
interfaces belong here, not every implementation's method list or copied internal state.

`api` is cross-language protocol; `contract` is local Go contract; `bindings` is generated
representation; `transport` receives remote calls; `sdk` sends consumer requests;
`instance` constructs local owners. They are not six compulsory sequential layers, and
“SDK public, everything else private” is incorrect.
SDKs may reuse necessary public pure values, not service instances/private facts.
External consumers may implement protocol clients directly; SDK use is optional.

### 3.2 Explicit local API changes

Methods previously concentrated on `*Core`/`*Facets`/`*Realm` may move by responsibility,
but are not all automatically converted into interfaces. If a concrete type expresses usage
without private-state exposure/reverse dependencies, do not add interfaces or per-method facades
merely for decomposition. A package need not correspond mechanically to one public object.

Every interface needs a real consumer, actual methods, and a dependency/substitution boundary.
Do not invent interfaces for hypothetical extensibility, easy mocks, symmetry, or unresolved cycles.
Retain already useful narrow authorization/contact ports; “as needed” does not remove existing
isolation or permission checks. Actual implementations directly satisfy required interfaces,
without another forwarding layer. A small cohesive Realm need not copy Core's package count;
interface count is not completion.

Necessary local Go API changes may relocate imports/types and operation access.
Specify and verify exact signatures/call mappings at implementation boundaries; acceptance of
these rules does not accept unwritten signatures/migrations. Method count and observable business
scope do not increase. Entrypoints, Composition, transport, and tests migrate together.
No old-call adapter remains. Record import-only moves separately from receiver/constructor changes.
Network methods, request bytes, permissions, and Store formats do not change with Go packaging.

Before moving opaque Revision, ReadCut, or Continuation, prove unique representation,
construction, issuance, decoding, and ownership validation under new dependencies.
Do not solve cycles by exposing private fields, arbitrary minting, or caller-implemented comparison.
Token shape validity is not authorization/source authentication.
List any necessary interface change explicitly; prose does not prove the boundary solved.

One bounded validated placement defines hidden values in module `internal/token` and exposes
the same type through a `contract` alias. It is not old-root compatibility or a mirror.
Construction, extraction, and shape validation stay package functions, not methods exposed by aliases.
Existing public comparisons/evidence projections retain their boundary; operations still check
source, relation, and permission. This approves only recorded values, not all tokens or new constructors.

MountKey/NextHopRevision retain original Valid/Equal respectively; reconstruction/extraction are
not public methods. CoreReadSnapshot/ConnectedCut use the same placement: Snapshot gains no public
method, ConnectedCut retains only Equal. Tokens hold bindings only; the query owner retains issuance,
registration, caller/run checks, TTL, capacity, release, and snapshots.
Shape validity does not prove retained state exists.

Query conditions, their three opaque continuations, and QueryReadMode share unique representation
in `internal/query/request`, with original public values exposed through contract aliases.
The leaf depends only on shared/values, existing geometry/token, and standard library, not
contract or read workflows. It owns shape, normalization, copying, comparisons, and binding checks,
not facts, snapshot tables, permissions, or expiry.
Partial scan positions are not issued credentials; the query owner completes binding at the
original point. Other query subpackages require their own review.

Trusted-local cross-Core envelopes have one representation in `internal/query/exchange`.
`contract/connected` aliases outer Request/Reply and retains two narrow ports.
Outer values have no exported fields/methods; packaging/extraction are internal functions.
Exchange uses root contract/bounded facts View, not query owner, commit, or Store.
Root contract cannot reverse-import the topic contract.
Internal payload fields are not a public DTO constructor.
Cleanup still ends one borrow/discards a pin; it adds no generic execution/transaction callback
or second snapshot registry.
`internal/query/connected` owns workflow using assembled query.Operations/commit.Owner.
Core returns real Connected operations and follows original close order, without method proxies.
Trusted-local caller references are not network wire or remote authentication.

Connected regressions follow their tested boundaries. `tests/connected` uses real Core/contract
operations, startup retention, and permission changes inside the injected test Authorizer,
not private-state access. Private world-merging tests use bounded facts.
Private filtering needs actual query.Lease: only `filter_test.go` may assemble the same commit.Owner,
Element entry, and query Capture for the original read set, then Finish/Close normally.
Do not add Lease-construction backdoors, copied filters, or package-wide dependency exemptions.
Retain original concurrent barriers, cleanup, and disclosure assertions.
Cross-Connected query-resource tests use `query_test` with actual assembly.
`export_test.go` returns only four lock-read numeric counters, no tables/pointers/setters,
and is unavailable in production builds. Exact test registrations do not authorize production
query-to-connected reverse dependencies.
Query counters and connected context/lease counters follow their actual owners.
Only `review_fixture_test.go` in connected assembles real Element/Mount/query;
assertion files use existing exchange values/private counters, not expanded assembly imports.

Foundation candidates, commit ordering, complete history, snapshot metadata, and Store-corruption
counterexamples stay with `internal/commit`. Fixtures establish facts by fully bound owner
commits, not direct table writes. Runtime identity-substitution tests use external `commit_test`
with real Element write/read/retry on the same open database.
Only `foundation_fault_export_test.go` injects that identity fault, reports empty reload, and
reads durable identity/operation existence. Three helpers return errors/bounded values,
not databases, writable tables, or state pointers, and must be unusable by production `go build`.
They expand no production API/dependency.

Full-state mount-gate invariance uses `commit_test` and real Element/query operations.
`mount_guard_proof_export_test.go` retains an opaque cloned proof in test builds;
comparison still calls `sameCompleteCoreState`, not just cut/disk.
Inspection returns only four counters, root existence, selected claim/kind, retained-operation
presence, and admission radius. No state/table/database pointer, setter, or production API.
Prove test symbols unavailable in production. Five structural-corruption counterexamples stay
same-package; do not export arbitrary corruption to move tests.

The independent full linear scan belongs to `internal/query/oracle`.
Under original owner read lock it validates state, selects cut, checks boundary/basis/mount
completeness, and executes original scan/geometry. It is a private reference, not public discovery,
second fact/snapshot state, or QueryRegion output used as its own oracle.
Only its fixture assembles commit/Element; assertion imports remain bounded.
Contract, transport, SDK, and other owners cannot access private reference/commit state.
External commit tests retain corruption/exact-depth cases. `oracle_fault_export_test.go`
provides three test functions for five fixed faults/fixed deep tree only, not arbitrary callbacks,
writable tables, or production symbols.
Direct adjacency and 259-page comparison use external tests in `element/read/adjacency_test.go`;
only that file may assemble commit/Element. Production read remains owner-free.
`tests/query` establishes public mount coverage through Composition/real MountPeer without
business-private access, with permission assembly at startup.

Three projection-path authorization/interleaving counterexamples use external commit tests
and real Element operations. `projection_fault_export_test.go` provides one fixed-parent
corruption function only. Preserve authorization-call counts, discovered-path target order,
invalid parents excluded from Authorizer, and mixed-cut rejection after writes during authorization.
No production fault interface is added.
Core-side Facet lifecycle/recovery tests assemble real commit/Element/facet operations and
change permission inside the same test Authorizer, not assembled fields.
Only `facet_fault_export_test.go` has two helpers for event/occupancy counts and five fixed
Store corruptions. Full-state equality/write faults use existing seams.
All three helpers must be production-inaccessible; test dependencies register by file.
Facet-history public snapshots stay `tests/query`, without commit/Store access or production
commit imports of operations.

Anchor Range persistence and twenty current-Store faults use external commit tests and the same
Element fixture, preserving actors, fixed IDs, operations, and two rejected receipts.
Do not replace it with table writes. Reuse full-state rollback proof/write seams.
Two new `_test.go` helpers corrupt only fixed historical shrink or twenty named Store cases;
no mutation callback or database/state return, and no production consumer can reference them.
Store JSON shape follows commit encoding; marker checks follow store and scan all module production Go.
Public exclusive-open, create-without-overwrite, and open-without-create tests stay `tests/instance`
with actual constructors/lifecycle.
Public declarations/opaque fields/method checks stay `contract`; leaving the root relaxes none.

Seven mount persistence/no-cut/contact-result groups likewise use external commit tests,
real Element/Mount with one durable owner, and addressed connections that still Prepare/Finish.
Retain 26 stage-specific commit faults, 18 journal faults, five unreachable-history cases,
and the third valid Store outcome.
Only test builds can fail the Nth commit, inspect address receipts, corrupt fixed journal/candidate,
or inject that third outcome. Counter fields remain private with read-only Calls;
candidate/full state are not returned. Production consumers cannot call them.
Trusted nested/federation fixtures follow their consumers; old root helpers/consumers are removed,
without restoring root production/test/compatibility entries.

Seven QueryRegion white-box/numeric-limit/corruption/durability groups use external commit tests
with real Element, query.Operations, and owner. Independent query/oracle is not a substitute query.
Two same-package files build the fixed 1,025-edge tree, 40-level tiny-rotation tree, and seven faults;
no arbitrary command/state callback is public.
Retain full-history proof/exact disk bytes. No-cut comparison returns operation-count delta
and full-fact predicate only, not history objects. Five new test symbols are production-inaccessible.

Thirteen mount-control/interleaving groups share relocated Element/Mount/commit and addressed
transport. Authorization switches, barriers, caller, ID, and commands stay original.
Checks use bounded facts/receipts. Same-package helpers return event counts/current/full
validation for two fixed bad histories, not cloned state.
No production API or weaker concurrent retry, rejection priority, old-close isolation, or gate-cleanup
assertion is allowed.
Seven nested-mount ancestor/child interleaving/reopen cases use external commit tests and five
real owners with existing operations, addresses, and full-history proof. No new exported hooks.
Reserve, post-Prepare, and denied-decision barriers retain timing.
Fixture-only fixed commits use existing operations, not generic callbacks/writable state.

The 17 federation service values share representation in `internal/query/federation/value`.
`contract/federation` aliases them and declares two contact ports/Range construction.
The leaf depends only on shared/values, root contract, and geometry; no reverse topic import.
ProjectionPage traversal stays private, without public methods, accessed by query/coverage
functions only. Wire neither encodes it nor imports business internal.
Consumers use the topic contract, not old-root aliases.
Value owns no facts, authorization, leases, registry, or Store.
Denial remains capture/check evidence, not a constructible service value.
Field order, condition digests, and wire-validation order remain unchanged.

`internal/query/federation/projection` receives bounded Element/Mount/Facet Views from one
borrow and original call-local signer for paths, records, queries, traversal, and relations.
It receives no full commit.ReadView, Core, Store, authorization, or registry.
Bounded PathIDs also bind observation without exposing fact fields.
Borrow completion, cut/permission recheck, counters, dependency binding, and coverage issuance
remain in original controlled workflows/lock order.
Shared errors preserve Federation-read semantics; unknown errors pass through, not ordinary-mutation mapping.
`internal/query/federation/transcript` owns original canonical digest bytes/fixed SHA-256/
HMAC-SHA256 construction. It exposes no raw hash or arbitrary hash/callback, keys, context,
facts, or permissions. A digest is not authorization.
MountAdmissionKey shape checks stay mount/fact, sharing bounded commit/binding OperationID
byte rules without commit.Owner, duplicate validation, or extra public methods.
Register exact leaves and consumers.

`internal/query/scan` owns closure, exact projection memo, candidates, paths, and page coverage
on one bounded read set. Dependencies are contract, current element/fact and mount/fact Views,
query/request, geometry, shared/values, and standard library, not owner, mount control, Store,
permission, or registry.
Callers retain original locks/immutable read sets and error/completeness policies.
element/fact.View reads only live Anchor/Entity tables. State embeds that same View plus historical
claims/tombstones and mutation/recovery; they are not two facts.
View guarantees no snapshot by itself. Original candidates alone publish authoritative state;
query owner controls borrows/reclamation.
UUID byte representation/order is defined once in element/fact for reads/Store ordering, without
identity changes. Current mount-content predicates share mount/fact across scans and commit guards.
Ordinary/public scans retain basis priority/gap semantics; scan position is not issued continuation.

`internal/element/read` shares same-cut exact Element/Facet reads, direct adjacency, and page
boundaries between current/historical queries.
It reads element/fact/facet/fact Views, not commit, operations, query, Store, or permission,
and promises neither independent consistency nor issued continuations.
Caller holds original lock or retained immutable view and preserves cut, errors, and disclosure.
No second long-lived facts/index is created.
Actual Element operations remain internal/element.Operations, not forwarded through query owner.

## 4. Internal implementation: complete responsibilities, one commit owner

### 4.1 Responsibility placement

These placements were checked against P6 code. Separate packages still require dependency
evidence; this is not an instruction to create every directory now.

| Module | Business responsibilities | Unique coordination/storage responsibility |
|---|---|---|
| Core | element: structure/spatial operations; geometry: exact computation; query: discovery, snapshots, continuation, cross-source reads; mount: control; facet: Core-side relations; access: contexts/observation | commit: cross-responsibility atomic commits, revisions, history, publication, uncertain-result recovery; store: database bytes/lifecycle |
| Facet | namespace: complete per-Element content, acceptance, reverse-contact lifecycle; access: contexts/observation | commit/store serve this Facet only, not Core writable state or another acceptance engine |
| Realm | policy: Principals, grants, credentials, checks; smaller responsibilities only for actual boundaries | Coordination/policy may share one cohesive package at this size; justify storage boundaries, not package symmetry |

After S2 network work, Realm roots are navigation only.
`contract` owns public values/errors/permission constants; `instance` constructs/opens/
assembles identity and returns the actual handle, not method proxies.
`internal/policy` retains one realmState, lock, binding/credential transactions, Store codec/replay.
`internal/identity` owns independent run-local proof state/issuance/verification,
without policy/persistent Store imports.
Both revisions have one private representation in `internal/token`; contract aliases expose
no fields, extraction, or minting. Internal policy functions use the same representation,
overflow checks, and durable encoding; shape is not authorization.
`transport/grpc` retains configuration, TLS registration, and host lifecycle.
`internal/admission` owns original service admission, challenges, and finite handles;
`internal/entry` directly registers RPCs against narrow Kernel/Verifier ports;
`internal/wire` encodes/decodes only. None imports business-private code or instance.
The small two-file SDK stays cohesive without empty subpackages.
Private counterexamples follow owners; host tests retain TLS/entry assembly;
`tests/instance` checks public constructors/reopen.
Registration is file-specific and applies at every level: do not reaccumulate business
under contract, instance, transport, or SDK names. Progress/evidence stay in the plan;
this clause authorizes neither commit nor upgraded contract authority.

Do not precreate transfer/handoff/chunk before slice authorization.
P7 placement follows actual contracts. Legitimate chunking is neither prohibited by its name
nor revived automatically from old code.
Facet plans no schema package for consumer content validation.
Reconcile accepted semantic gaps in the plan before structural migration.
Core's internal `facet` is Core-side relationships, not another content module;
directory spelling does not decide public relationship naming.

Core-side local/public Facet control belongs to `internal/facet.Operations`, returned by
`Facets()`. Configured-once acceptance sources/control round trips stay together, borrowing
commit.Owner's fact lock.
Fixed private control records hold original caller/View/requirements/context, not exported
transaction callbacks or root callbacks.
`internal/access.Admission` owns original public-view validation and the sole active-call count.
All public entries share it; disabled remains unavailable.
It owns no facts, Store, context TTL, or observation queue.
TTL/queues belong to `internal/access/registry`; observation/entry retain their workflows.
Facet public/federation records share bounded `facet/fact.PublicInspection`, not new reads,
authorization, or commits.

Facet service reads/preparation use actual facet operations and one Registry, not new queues.
Local reverse contacts explicitly bind
`LocalFacetCoreEndpoint{Read: c.Facets(), Control: c.Facets(), Access: c.Access()}`.
Relation reads/JSON observation use their real entries; Control/Access are both supplied or
both disabled, with same-owner checks before/after calls.
gRPC `Config.FacetControl`/`Config.AttachmentControl` receive the same Core's operations.
Fixed services still require the complete AttachmentControl port, not inferred JSON Owner.
Missing optional capability retains unavailable/Unimplemented or enabled-config rejection,
not fabricated capability. Assembly checks owner without protocol/new-request fields.

Mount local/public control, addresses, peer acceptance, and recovery belong together to
`internal/mount.Operations`, returned by `Mounts()`, using the same Admission/commit.Owner.
No root proxies remain. Original authorization/reservations are fixed private records,
not transaction callbacks. Network recovery lacking original View cannot reauthorize pending
writes; durable settlement follows original rules.
One command-digest algorithm lives with operations.
Inbound `Config.MountControl` and outbound `MountConnections` receive narrow same-owner ports;
transport imports no business internals, and contract copies no digest/normalization.
`access/value.PoseJSON` uniquely represents public Pose for projections/digests, unchanged in encoding.

### 4.2 Avoid workflow/commit dependency cycles

First distinguish deterministic facts/rules from workflows with external dependencies.
They may share a package. Extract lower `fact/` only when coordination must call rules
while workflow calls that coordinator; do not generate layers in every responsibility.

The relocated Element dependencies preserving P6 behavior are:

```text
instance ──assembly──> element operations, commit, store
element operations ──> commit, element/fact, contract
commit ──> element/fact, other required fact rules, store
element/fact ──> contract, geometry, shared/values
store ──> own byte format, standard library, existing database dependency
transport ──> actually used public contract values, protocol bindings
```

Arrows are permitted dependencies, not network hops.
Commit does not import invoking workflows; fact rules do not import commit.
Transport does not import fact implementation; business does not reverse-import instance.
Consumer-side narrow ports may be satisfied by assembly; interfaces need not all live in contract.
Facts define one rule used by normal commits and replay.
If a split creates extensive whole-model conversions, revise boundaries rather than add adapters.

Commit is explicit owner-local transaction coordination, not a generic transaction framework.
It retains operation deduplication, owner-wide cut/journal order, candidates, atomic save,
failure load, and publication. There is one authoritative aggregate.
Domain components use constrained transaction candidates/immutable reads, not synchronized mirrors.
No cross-module `Execute(any)`, full-state getter, arbitrary callback transaction, or writable map.
Specific cross-responsibility operations are allowed where needed; do not add a port per check.
Published memory and confirmed disk are not separate fact owners.

`internal/access/observe` projects publication relevance from before/after Element/Mount/Facet
read-only Views at the same boundary and finite observation selection.
It accepts no full State, journal, receipt, Store, permission, or lifecycle callback.
It classifies affected spatial/attachment subjects, not sessions, queues, TTL, capacity, locks,
or publication time.
Queue changes still occur under the sole commit lock; moving this pure projection does not
prove the whole observation lifecycle moved.
Dependencies are standard library, shared/values, Core contract, and three Core fact Views.
Other modules, transport, and SDK cannot import it.

Store reads/writes canonical storage representation, not permission, containment, or acceptance.
Owner rules/recovery retain those checks. Real persistence conversion is legitimate,
but no long-lived writable mirror graph is maintained.
If splitting adds only meaningless database forwarding, retain cohesion and record why.
Cross-owner access remains consumer ports/Composition, not shared transactions, global locks,
or routing directories.

### 4.3 Three boundary checks

For each proposed split, record counts and actual uses rather than moved-file totals:

- Authoritative state: who stores/mutates/publishes it; added mirrors or cache-consistency burden.
- Public surface: added/moved types, constructors, methods, and real consumers.
  Delete capabilities exposed solely for decomposition.
- Adapters: additional forwards, conversions, or reverse ports per real chain.
  Explain every boundary's value.

Path-only wrappers, identical field conversions, or callbacks into the old giant object for
every small step signal a design problem.
Real protocol, persistence, and cross-owner evidence boundaries remain legitimate;
root-file reduction must not erase their differences.

### 4.4 Boundaries and growth at every layer

Apply responsibility review recursively to instance, contract, business internal, transport
internal, SDK internal, and tests. These are permitted contents, not six compulsory directories
or places to fill first and review only after exceeding limits.

| Location | Cohesive contents | Not allowed as relocation cargo |
|---|---|---|
| instance | Construction, dependencies, public open/close | Whole Core/Facet receiver proxies, fact tables, decisions, session algorithms |
| contract | Actually shared values/requests/results grouped by topic | Store models, commit commands, replay events, writable state, generic cycle-avoidance interfaces |
| business internal | Complete Element/query/relation responsibilities; shared deterministic rules when needed | Moving all public files into access, workflow callbacks, parallel models, arbitrary transaction callbacks |
| transport internal | Real RPC groups and helpers; one owner per shared admission/session resource | Handler access to business internal, copied sessions, empty per-RPC forwarding |
| SDK internal | Actual independent response validation/encoding boundaries | Server decisions, fragmented request-association state requiring resynchronization, speculative layers |
| tests/acceptance | Scenario-grouped composition with minimal fixtures | Publicly exposed private-state white boxes, giant unified test package, universal helpers |

Core public requests distinguish local facts, source reads, cross-source collection, and
observation lifetime. `public_` is a filename prefix, not one relocatable responsibility.
Requests stay with operations; shared contexts/resources/observation follow actual state.
Parsing/validation shared by admission/replay belongs below both, not commit importing access workflow.

`internal/access/registry` owns the single context index, capacity, TTL, parent/child and
derived-basis release, observation queues, and collector-retirement refunds together.
Do not create tables/accountants by token kind.
Admission, query snapshots, and collector retain their responsibilities.
Registry offers bounded operations, opaque references, and finite read-only results, not writable
Context, whole tables, or mutex.
Expiry cancellation/removal occurs under the original owner lock, collector retirement outside,
refund under it again. Synchronous publication receives original before/after rather than another
current-fact read. Establish this lifecycle before relocating source algorithms; do not dump
all public operations here.

`internal/query/federation/source` owns Open/Project/Check/Release/Observe and Capture/Check Denial,
using the same commit.Owner, query, Admission, Registry, and observation.
No second source/snapshot table, root callback, or method proxy.
`Federation()` returns the real entry.
Local connections move with the workflow through collector ports; source returns original
disclosures/bounded evidence.
gRPC `Config.Federation` binds the five-method same-owner source port explicitly:
nil stays unavailable, not dynamic JSON Owner lookup. Denial gains no network fields.
Actual hosts must rebind after open/replacement/reopen.

`internal/access/observation` shares local/source observation with the same owner,
Admission, and Registry. Ordinary calls acquire the lock; source calls run the same workflow
under its existing lock. Preserve JSON, permission, relation, queue, and limit-check order:
no extra authorization/queue or earlier limit decoding that changes error priority.
Pure limit rules remain access/value.
The private context created after envelope Acquire belongs only to that Admission.
Sources recognize the same component, not another owner or request marker.
This marker is not identity/permission evidence.
Local collector retirement uses private source release; ordinary Release still admits.
Retain nil-context preconditions, cancellation/close, release idempotency, and no generic
“already admitted execution” port.
Closure requirement construction is shared in access, not copied by source/entry.
Source/observation counterexamples follow source; zero-resource counts follow Registry,
without external table access.

`internal/query/federation/coordinator` references the same commit, Admission, Registry,
source, and observation. Collect owns actual source graph/leases; Registry owns public receipts,
derived basis, and retirement refund. Coordination copies neither and calls no root.
JSON-to-typed collect and bounded Page/SourceGraph-to-JSON stay with coordination.
Preserve source opening before later decoding, two mixed-retention attempts, lock-free I/O,
and final locked recheck.
`internal/access/entry` directly invokes Read/Observe without root proxies.

`internal/access/resolve` uniquely shares token parse/ownership/lookup order across prior root,
source, and observation. Callers already hold the owner lock.
Functions create no state/tokens/admission/authorization.
Registry results retain representation. Exact consumers only: no transport, SDK, other owner,
or reverse commit usage.
Continuation condition encoding remains access/value, shared locally/federally and excluding
only limit/continuation.
Source scans verify exact WithAdmission locations/successful Acquire shape;
an “Acquire text appears earlier” static test is insufficient for arbitrary new calls.

`internal/query/local` handles local public read/list/query with the same commit, Admission,
Registry, and query: original borrow/capture, projections, page conditions, derived basis,
observation preparation, and final checks.
Structural-address checks stay with this bounded projection; no bare UUID lookup/scope change.
Public JSON mutations/recover-outcome belong to element.PublicOperations, using the same commit,
Admission, and Registry. Typed Element construction need not depend on public resources.
Both retain authorization contracts/one commit, not new Element facts/replay state.
Intent/Reference parsing in access/value interprets original permission/contact kinds,
without authorizing or reading facts.

`internal/access/entry` assembles the same six components: owner, Admission, Registry, local,
federation coordinator, and Element PublicOperations.
It retains shared capacity/JSON dispatch, not business workflows.
Successful Acquire precedes same-component marker; two defers, reservations, and error order remain.
It directly satisfies transport/Composition's narrow port without another forwarding call.
Construction/open/run scope/Close live in instance; `c.Access()` is the actual entry,
not Core.PublicCall[WithFederation] proxies.
Instances hold components/stable identity, not fact tables.
Close retains BeginClose, resource close, lock-free retirement, query close, Store close order.
Eight root-public composition groups move to `tests/access` using instance/Realm/schemas,
not business internals. Private tests follow owners; zero root files still need coverage audit.

Two public_resource overflow/admission/memory/expiry-refund groups use Registry external tests.
Only public_resource_fixture_test assembles real entry/local/source/coordinator/Element/commit,
keeping one owner/publication chain rather than manipulating tables.
public_resource_export_test provides three fixed test-only configurations (Groups=1,
Bytes=1<<19, clock +two hours) and entries/bytes counters, not arbitrary setters/writable state.
Two functions must be production-inaccessible; file-specific dependencies do not let other
Registry tests/production import entry/business owners.
Old root helpers remain only while actual old consumers exist.

`tests/federation` builds original multi-Core mounts through instance, public contracts,
and addressed Composition, not business internal, Store, transport, or other test packages.
Inject mutable policy before construction, not replacing assembled fields.
Tracked connections count actual Release; real Check proves release unusability.
That evidence does not replace Registry entries/bytes or snapshot accounting.
Six expiry/capacity/cancellation/invalid-mode/missing-Purpose cases use Registry external tests;
lost-retained-snapshot source failure stays source, exact borrow/refund stays query.
Registry retirement cases use actual source/collect with barriers for locked cancellation,
unlocked release, no early refund, exact zero after completion, and no double release on close.
No production fault port is added.

Keep original assertion layers: collect follow rejects source-admission range mismatch;
full query retains verified sources and reports UnresolvedCoverage for an unverifiable frontier.
These are distinct P6 contracts. Do not turn the latter into total failure for misplaced
assertions, or delete lower-level rejection, graph exclusion, and cleanup in favor of partial success.

Same-cut source/Facet address tests belong to tests/query/source_attachment_test with real
instance, Core contract, and federation topic values.
Old snapshots keep old address, current reads see repair immediately, and Pending control
does not leak through records. No private state API or persistence change.

`query/federation/denial` uniquely represents FederationDenial; `contract/federation` aliases
it without old-root alias/wrapper. This leaf holds original denial-contact evidence only;
package functions are not publicly exposed through aliases.
Capture/Check retain actor, request, incoming relation, expiry, and denial-view checks at
original points. It is neither permit nor public authentication token, goes nowhere on wire,
and shape matching cannot bypass authorization.

One fact owner need not mean one lock for all module resources.
Commit, snapshot, remote connection, and session locks require explicit state ownership,
lock order, external I/O, and final revalidation, not another writable fact model.
Core publication uses before/after state; Facet uses original command/result.
Storage confirmation, recovery installation, cut advancement, and observation retain atomic
boundaries/order, not unguaranteed async notification or mid-publication “latest” callbacks.

Before adding a package, record original declarations, target responsibility, state owner,
allowed dependencies, real consumers, and test placement in the plan.
Split mixed declarations, not bulk-move by filename. Review all affected direct packages,
not roots only. If a candidate already exceeds a control line, revise its boundary before
treating it as approved.

## 5. Transport, SDKs, and generated code

`transport/grpc` roots handle registration, configuration, startup/shutdown, and necessary entries.
Organize sessions, handshakes, business RPC groups, and contacts by actual coupling, such as
`transport/grpc/internal/session`. Do not move business decisions into handlers.
Cross only necessary boundaries, not empty Handler → adapter → driver → root chains.
Authentication, protocol checks, and business decisions cannot disappear merely to reduce layers.
Move RPC groups with their helpers, not constants/utilities alone.

One component owns each session state; handlers use restricted operations, not separate maps.
Preserve permission, capacity, lock-free network, and final-check order.
Unified middleware is not a reason to change error priority, authorize twice, or bypass disclosure.
Transport-private and business-private `internal` boundaries remain separate even where Go
technically permits imports.

SDK organization follows connection/session, real requests, and response validation.
Shared wire values may use one codec, but server authorization, cross-owner fact checks,
and client response validation remain distinct; deduplication cannot remove a check.
SDKs read no Store, instance state, or server session table.
Group bindings by generator/language/protocol, not one directory per message.
Do not split generated code manually or repartition protocols for file counts.
Pin generators and verify regeneration.

Repository `internal/protovalue`, `grpcbound`, `grpcdial` retain demonstrated concrete uses only.
Dependency checks distinguish business, transport, and SDK private packages, not a blanket
internal exemption. Every implementation directory must obey responsibility/size checks.

Register actual RPC group implementations directly; root methods need not repeat them.
Capacity reservation, parent-session validity, unlocked calls, and return revalidation are one
admission chain; separate “validated” handlers do not establish equivalence.
Pending client challenges, sessions, and request associations are client state, not stateless
response-validation utilities. Small Facet transport/SDK may remain cohesive without copying Core.

Core directly registers four AttachmentControl RPCs in `transport/grpc/internal/attachment`,
without root forwarders. It holds fixed service contacts/four-method consumer port only.
`internal/session` owns caller/grant private context keys/accessors;
`internal/admission` owns actual sessions/grants/pending/challenges/approvals,
sharing original mutex, invalidation, and stop order, not copies per group.
`internal/rpcfailure` uniquely maps original control errors; it imports no Server/business owner/SDK.
FacetControl's two RPCs register directly in `internal/facet`.
Unsupported ports still return Unimplemented at call time, not new construction failure.
Mount-peer errors use `internal/rpcfailure`, without old-root wrapper.
The leaf references existing contract errors, not facts/sessions/database.
The checker registers four exact packages and forbids reverse/cross-module access.

Five application MountControl/MountContact RPCs and command/contact representation belong to
`transport/grpc/internal/mount`.
One Service holds original six-method port, expected identity, and fixed mountconn;
Server directly registers it without matching root methods.
Disabled configuration, including absent connections, rejects before caller/delegation/request parsing.
Apply still resolves, parses this approval, then calls the original commit port;
approval copies do not mutate installed connections.
No session/approval table, lock, or facts are owned here.
Only service declaration/minimal fixture reference mountconn; other files use authenticated context,
wire/failure mapping, and public values.
Exact dependencies, direction, disabled/error order, no root duplicates, and no unrelated Service
state have counterexamples. Shared admission owns approval/peer negotiation;
this group alone is not all transport.

`internal/admission` directly implements six inbound services: Session, ReadDelegation,
IdentityProof, MountApproval, MountPeer, FederationRead.
Five original tables, mutex, active/running, listener registration/stop state exist once.
TLS, actual network shutdown, and host remain Server.
Serve/Stop register under the same admission lock; post-I/O rechecks retain order,
not two locks or independently renewable grant mirrors.
Core RPCs use BindEntrySession/CheckEntrySession/CheckDelegatedResult.
EntrySession fields stay private except expiry; it is no new table/identity token.
Config lives at its consumer and the host uses the same type, not old Server proxies.
One wire leaf owns original protobuf/JSON codecs, not server state accessible to kernel/SDK.
At this boundary admission was 16 production files/1,932 lines and 6 test files/854 lines,
all exactly registered, permitting original shared admission lifecycle only, not business
JSON dispatch or outbound connections.

CoreService's 15 RPCs directly register `transport/grpc/internal/entry`:
four production files/290 lines and two tests/553 lines at this boundary.
It handles protocol, operation selection, representations, and finite session checks,
not grants/business state.
Only construction holds federationconn; other files use protocol/public values, wire,
session, and the three admission operations above.
No whole admission handle may be obtained/aliased/forwarded, or stop/session/approval operations used.
Only registered fixtures use SDK/metadata/barriers for actual session/delegation final checks.
Production gains none of those dependencies. Fixtures do not replace acceptance's complete networking.
At this boundary Core's host was one 252-line file for configuration, direct registration,
TLS, startup, and shutdown.

## 6. Files, naming, tests, and size

Name files by responsibility, not numbered fragments. Do not create unbounded utils/common/manager
containers. Use gofmt and established line endings.
Comments explain invariants, lock order, failure, and tradeoffs; do not remove them to lower counts.
Multiple files per package are normal, but shared private-field access alone does not justify
keeping independent responsibilities together.

Retain file gates and package growth review. These are project control lines, not Go standards
or correctness theorems:

| Subject | Human responsibility review | Stop further growth before boundary decision |
|---|---|---|
| Handwritten production Go file | More than 500 lines | More than 800 lines |
| Go test file | More than 800 lines | More than 1,000 lines |
| Package handwritten production | More than 25 files or 5,000 lines | More than 50 files or 10,000 lines |
| Package tests | More than 50 files or 10,000 lines | Test responsibility/fixture review is required |

Physical lines include blanks/comments. Count only direct package files, not all descendants.
Count production, tests, and generated code separately.
Mixed independent responsibilities require action even below limits.
Do not automatically split packages or concatenate unrelated files.
A cohesive large-package exception needs exact path, current size, evidence, and finite ceiling,
not a permanent old-code waiver.
No renamed/hidden paths, false generation markers, or testdata relocation to evade checks.

**P7.2 codec boundaries (2026-09-18, accepted placement):**
One commit owner does not require its pure codecs in commit.
`internal/store/record` uniquely encodes bounded Element, tuple, Revision, mount, and Facet-contact
values. `internal/transfer/encoding` uniquely encodes/validates canonical transfer journal,
manifest, lineage, parent-edge, and final evidence.
Neither holds authoritative state, lock, database, signer, network, or authorization,
nor reverse-imports commit/workflows.
Manifest decoding uses an independent finite Element builder, not temporary `coreState`;
it is no second fact owner.
Commit retains state/candidates/locks/atomic save/publication/recovery.
Normal execution/replay share codecs. Do not duplicate storage types, change fields/bytes,
add public interfaces, or keep old-path forwarders.

The reviewed commit production ceiling tightened from 76 files/10,886 lines to
**61 files/8,300 lines** (actual pre-move 77/10,981).
Its remaining >50-file exception is deterministic commit/replay/relations for one aggregate,
not unlimited growth.
New store/record was 5 files/709 lines and transfer/encoding 18/2,046, without exemptions.
All 112 commit fault/recovery files remained; actual import updates brought them to 16,817 lines.
Store-shape fingerprints followed codecs.
Checker remained 28 production/58 test files, ceilings 4,343/11,239 lines, adding exact registration
and reverse/I/O/lock rejection cases.
Register each new package/caller, not unreviewed files, standard-library I/O, or transport/SDK
access to private representation. Only the plan records full runtime validation/progress;
engineering checks do not replace the repository gate or P7.2 acceptance.

### P7.2 source-evidence transport boundary (2026-09-19)

`transport/grpc/internal/transition` reads retained evidence through a narrow owner port.
`internal/transitionwire` converts bounded network values, not Store/journal codecs,
locks, or business operations.
`transport/grpc/transitionconn` owns pinned addressed clients, bounded chunk collection/digest checks,
not facts.
Register locations/files exactly; unregistered files reject.
Core/Facet roots and commit ceilings stay fixed.
Seven exact registrations/wire reverse-runtime restrictions add 14 checker lines:
28 production/4,357 lines; tests remain 58/11,239.
Existing negative tables add paths without broad dependency relaxation.

The same day's target increment: `internal/transition/target.go` handles fixed-peer accept/install
RPCs. It fetches real source evidence outside the lock, then uses admission
`AcceptFrozenTransition` and the target port.
Admission retains original approval/session lifecycle.
Call-local immutable `frozenSource` has no network/Store access, retained transfer state,
arbitrary execution, or transaction callback.
Commit alone owns state/save/recovery; `transitionwire` converts receipts and
`transitionconn` calls one addressed operation. Narrow actual ports only, not business imports.
Seven registrations plus one admission-file registration bring checker to 28/4,365.
Tests 58/11,239, commit 61/8,300, zero root source, and acceptance boundaries remain.
Counterexamples stay `tests/transfer` and actual transport/admission/wire/SDK packages.

The later recovery increment: `internal/transition/recover.go` assembles one original-operation
advance to the actual owner, not another state machine.
One `transitionconn.Connections` handles source reads and target calls without old one-way names.
`admission/entry_context.go` cancellation belongs to existing finite session entries;
idle entries need no timer/goroutine.
Active RPCs bind deadline/cancellation; release/expiry/shutdown follow session lifetime.
Final owner checks synchronously read cancellation, without acquiring admission lock under owner
or a per-recovery registry.
`cmd/ucupa-core/transition_connections.go` is product configuration, not Store/business decisions.
Two exact registrations bring checker to 28/4,367.
Existing cross-owner product tests add 87 lines: acceptance 70/10,096.
Real products, SDK, addresses, and restart use no private helper steps.
This bounded chain alone was not all P7.2; other size/dependency limits remain.

The following earlier decisions explain boundary/size evidence, not new growth allowance.
Use the current checker and applicable later registrations for effective ceilings.

The prior checker test exception was `cmd/verify`: 58 files/11,196 lines.
Tests share private source parsing, dependency rules, and command orchestration.
Fixtures are bounded injected source/temporary build directories, not business-state white boxes.
One package avoids exposing private checker functions/copying rules merely for counts.
Group files by rule responsibility; exact finite registration does not waive other test packages.
Prior production ceiling was 28 files/4,168 lines: above file review threshold but below the
production stop line. These are one pipeline's concrete rules, not business state.
Do not merge unrelated rules or expose parsers for counts.
Collect/transport exact locations and their policies live together; generic path membership
stays classification, not a central dispatcher absorbing every registration.
Root-exit, actual collect consumer, Mount RPC, shared admission, and entry AST/counterexamples
use that same test package, no new business fixtures or moved business state.

Prior commit tests were 59 files/8,603 lines for one durable owner's facts/history/fault/recovery.
P7.1 added a writer transaction test using the same actual chain for original-Store close,
installation, uncertainty, and concurrent commit.
First P7.2 tests added real-Store locked preflight, observation cuts, outcomes, restart,
and old-writer refusal only. Pure closure/placement tests stay `internal/transfer/fact`.
This finite exception does not collect ordinary query/other-owner tests into commit.
Two later intent/cancellation and Store-fault tests cover same-ID concurrency, raw non-UTF-8
identity, exact cancel retries, cancel after capacity exhaustion, damaged journal, precommit
rollback/postcommit reply loss, and actual writer handoff. They use original owner test seams,
not public contract/SDK/instance state exposure.

The recorded production ceilings progressed as follows:
P7.1 28/4,927; first P7.2 preflight 29/4,985; intent 32/5,385; fence 34/5,650;
source write guard 35/5,724; bounded read validity 36/5,807;
manifest decode/private target acceptance 38/6,132 with 62 tests/9,101;
source decision/target install/source settlement 41/6,644 with 66 tests/9,907;
return transfer 43/6,891 with 69 tests/10,379;
root succession 44/7,054 with 75 tests/11,144;
carrier parent preparation 47/7,464 with 79 tests/11,752;
controller identity/source refusal 49/7,800 with 81 tests/12,126;
carrier source close 50/8,071 with 83 tests/12,481.

`carrier_fence.go` performs original-owner durable close, outcome recovery, and finite evidence.
It shares controller-qualified fulfillment records/candidate/commit with refusal, no fence table/journal.
`transfer/fact/carrier_source.go` selects complete contents under external parent and validates
evidence shape, including empty closures, without parent copies/pseudo-roots.
Uncontinued outgoing edges explicitly reject, not silently disappear.
Original manifest codecs share member/Facet predecessor/lineage across profiles, not a copied
carrier model. Incoming fenced and ordinary detached gates remain distinct.
Read projection holds original close evidence within 64 KiB per incoming, not manifest/history.
Current/retained disclosure still checks source validity.
Two commit tests use real durable owner/seams; pure closure/empty-space observation stay their packages.
Exact registrations bring checker to 28/4,206, without general transfer/future allowance.

After carrier target review, commit ceiling was 52 production/8,259 and 85 tests/12,855;
checker 28/4,212.
Acceptance transaction/private codec use original candidate/journal/Store, not new tables,
locks, or truth. Carrier outcomes use original `targets` index/capacity and root/mount/transfer
reservation checks.
Complete closure/inactive placement rules stay transfer/fact; commit decisions do not move there
merely for counts.
Tests group acceptance, faults/concurrency/storage; placement stays rules.
The competing-mount fixture now uses a genuinely empty local request, not invalid input as
concurrency proof. Later parent binding/install/public work still needs review, not automatic allowance.

Carrier parent binding/target installation review set 57 production/8,727, 90 tests/13,472,
checker 28/4,223.
Transactions use original owner candidate/journal/commit.
Private codecs, bounded origin/lineage evidence follow the same model, without workflow,
authorization, RPC, second state/committer, or copied installation algorithms.
`targets` distinguishes installed state; ordinary writes do not mistake installed carriers for
pending reservations.
Incoming origin uses actual install evidence, not fabricated MountPrepare/PeerAttempt.
Views hold finite proof, not whole journals/manifests.
Replay registers actual issued mount/contact revisions after transfer commits so normal address
repair remains recoverable, without requester-created old tokens.

Carrier settlement/return review set 58 production/8,970, 93 tests/13,987, checker 28/4,227.
One settlement transaction shares owner/control slot/outgoing journal.
Parent releases its slot; source retains completed departure without reopening writes.
Return rules stay transfer/fact; controller-qualified lineage adds no claims/history/recovery model.
Settlement, cyclic return, and corruption groups retain exact dependencies.
Ceilings authorize no workflow, public API, or communication growth inside commit.
Parent journal binds acceptance's actor/input directly, not actor twice.
Maximum encodings check original 8 KiB control event and settlement reservation rather than
raising capacity to hide duplicates.
Tests group parent decisions, target install, real multi-member/predecessor, faults/concurrency/
corruption, and codec capacity.
Parent/source settlement and return were completed in this increment; external edges are reviewed
separately. Internal transaction success is not the public chain or all P7.2.

External-edge capture/durable carrier close set commit 60 production/9,292,
96 tests/14,434, checker 28/4,235.
`transfer_edge_close.go` is original-owner control, and `transfer_edge_store.go` shares original
manifest/journal codec, not workflow, Store, or second writable space.
Original transferState's controller/operation/mount index rebuilds only from journal and counts
against existing intent/event/manifest capacities.
Validate full manifest only at close; ordinary reads hold one bounded current close proof,
distinct from detach/carrier departure.
Boundary selection/proof belongs transfer/fact: 13 production/1,148 and 10 tests/962.
Three commit tests cover ordinary/root, nested carrier, faults/concurrency under actual commits;
observation GAP stays observe.
Target still rejects install until matching new-parent continuity, not automatically activated
by source close. Exact dependencies, no speculative packages or RPC/authorization/workflow in commit.

Private target external-edge acceptance set commit 61 production/9,447,
98 tests/14,782, checker 28/4,240.
Close-proof codec represents original acceptance event using existing admission/cut/floats/
8 MiB evidence budget; no full source input per edge or new database/recovery.
`transfer/fact/edge_acceptance.go` owns edge matching/target binding.
Commit saves that proof set/members/placement under original lock/transaction.
Two tests cover private acceptance, faults, retries, and corrupt recovery; rules stay fact.
Acceptance alone activates no parent; matching delegation continuation remains required.

External-relation continuity/target completion review (2026-09-18) set commit
68 production/10,038, 101 tests/15,213, checker 28/4,255.
The >10,000-line exception was bounded to same-owner new-parent install, child reacceptance,
completion transactions, their codecs, and incoming evidence, not RPC/auth/workflows.
They must update cut, member reservations, mounts, candidate, and publication atomically;
another writable transaction owner would break that boundary.
Pure install/completion evidence stays transfer/fact: 15 production/1,330, 11 tests/1,060.
Acceptance reserves edge-settlement receipts in original 8 MiB and consumes actual canonical
bytes at completion without full-input repetition.
One bounded compact completion-event allowance per target keeps original 64 MiB journal limit.
Targets/edges share commit/recovery; views carry finite availability, not full journal.
Installed members with uncontinued external edges are not matches/list items/routes:
queries report gaps and old snapshots do not become new state.
Query/observe/rule tests stay their packages; three commit tests cover real transactions,
restart, reply loss, corruption, and reservation capacity.
Exact file registration is not approval of all transfer or later public growth.

Parent relation holds/slice incoming review (2026-09-18) set commit 75 production/10,716,
111 tests/16,642, checker 28/4,274.
Five production files cover original mount-slot hold/release, same-snapshot validation,
source incoming matching, and existing journal/manifest encodings.
Slot, cut, history, and recovery remain one transaction.
Durable parent promise is temporary relation reservation, not owner, lock service,
authorization, or transfer database.
Evidence/closure/placement stays transfer/fact: 16 production/1,528, 12 tests/1,184.
Manifest stores exact parent-hold evidence once within 1 MiB each/8 MiB total.
Parent holds share original 1024 intents/64 MiB journal. Termination does not free identities;
no cleanup/rollback entry.
Five tests group real relation/source transfer, faults, non-UTF-8 bytes, and capacity.
Shape tests stay rules, not authorization/network/orchestration in commit.

Target incoming adds two production files for actual admission/parent hold and bounded codec.
Source manifest/target acceptance share one parent-proof codec in original index/journal.
A target proof is at most 8 KiB counted in 8 MiB total, not installation/settlement envelope space,
and journal capacity stays fixed.
Placement checks external-parent Range/local ancestors without pseudo-parents;
one transaction reserves UUID/placement.
Five tests group constraints, faults/corruption, return lineage, capacity, and bilateral relations
using real owner fixtures. These are one atomic promise, not a new acceptance owner/callback package.
Private transaction/evidence tests alone are not public/cross-process acceptance.

Parent-hold read integration (2026-09-18) retained 75 production/111 tests, ceilings
10,741/16,665; checker 28/4,278.
No new commit file: existing slots/relation validator project current relation without changing
mount facts/journal/occupancy.
Final disclosure checks current slot, not departure members/whole history.
`transfer/fact/visibility.go` carries no actor, manifest, grants, or control record and is not a permit.
Each retained relation counts 64 KiB snapshot capacity, without raised limits.
Federation projection/token checks share exact revision/admission/slot/contact rules;
ordinary mutation slots cannot open through this path.
Connected omits unnecessary parent-control evidence; public contracts add no field.
Real-Store federation read/page/observe/revocation/address cases stay federation/source;
restart read projection stays commit; capacity stays query. Register exact dependencies only.

Public source control (2026-09-18): `internal/transfer/operation.go` uses only same
commit.Owner/Admission for original caller export/inspect, admission, ctx/view checks.
`instance.Transfers()` returns real operations, no transfer service/recovery table.
`commit/public_transition.go` handles locked CAS/current-original comparisons,
original cancel recovery, and bounded projection through intent/cancel transactions.
No new journal/format/duplicate public input.
Begin records INTENT only, not target checks/export; later actual workflow checks target approval
and both views before fence. Responses expose no manifest, private cut/control sequence, or approval.
One purpose-specific `transition-revision` issuer reuses durable signing material in token;
it binds Store scope, operation, boundary, and control revision, not credentials.
Public values/JSON/shared Protobuf token kinds stay synchronized.
Real Realm/instance tests stay `tests/transfer`; persistent-fault projections stay commit.
Limits were commit 76 production/10,874, 112 tests/16,801.
The extra representation file keeps MAC/outcomes with sole owner, rather than exposing keys/
whole state to workflow for counts. Transport/SDK cannot import transfer;
instance gets only this exact assembly dependency.

Source gRPC (2026-09-18): `transport/grpc/internal/transition` `service.go`/`wire.go`
handle three controls/finite conversion, with local tests.
Dependencies are public contract, original session admission, wire/failure, not facts,
tables, grants, or retry loops. Transport host alone assembles it.
Check session before/after; actual operations authorize original caller.
SDK uses generated stubs and checks source/operation/boundary/kind/target/comparison,
not mismatched results or service orchestration.
`ErrStore` writes map INDETERMINATE, exact inspection UNAVAILABLE; disconnection is not proof
that no commit occurred. `acceptance/transition_product_test.go` reuses actual TLS/Realm/start/restart,
not complete multi-owner transfer evidence.
Checker 28 production/4,299 and 58 tests/11,211; acceptance 68/9,799.
Register actual measured use, not reusable empty allowance.

Target finite approval (2026-09-18): `internal/transfer/approval.go` uses Admission/commit.Owner
to capture/recheck original target caller/view, import, and slice final-parent place permission,
not another authorization state.
`commit/transfer_preflight.go` adds twelve availability-check lines, no transaction/format.
Transport admission owns run-local token/session/exact binding/expiry.
Create/Check/Revoke register directly with finite separate capacity, cleared on restart,
not durable Accept.
`wire/transition.go` shares command-boundary decode across source/approval.
SDK checks original input/attribution; echoed source comparison is not target-issued,
and cannot relax other response ownership checks.
Local Realm authorization tests stay `tests/transfer`; concurrency/lifetime stays admission;
two acceptance files cover product/TLS.
Admission has no business-private, Store, or outbound dependency.
Checker ceilings 28 production/4,304, 58 tests/11,213;
commit 76/10,886; acceptance 70/10,009; roots remain zero.

Source fence/canonical manifest add two files under original owner lock, publish, persist, reload.
Manifest shares spatial/Facet codecs and selected members/ancestors, not long-lived writable state.
Active-record removal, departure claims, outcomes, final cut, and fence journal are checked in one
transaction. Pure departure rules/tests stay element/fact, not coordination.
Fence tests cover real Store, concurrency, uncertain replies, manifest/order/departure corruption.
`transfer_guard.go` uses inactive members/finite control under original owner to protect source
ancestor placement, without per-write manifest decoding/history replay or another stateful rule owner.
Element update/delete, Mount slots, other source transfer, and Facet decisions share needed checks.
Recover existing outcomes first; unrelated sibling writes are not globally blocked.
One durable test covers restart, ancestor races, empty-parent reanchor, early-intent recapture,
and forged post-fence history, not ordinary query tests.
The >5,000-line review is justified by one transaction checking original result namespace/control;
exposing whole state to a second transfer-transaction package is not a remedy.
Pure input/spatial rules remain transfer/fact. Later increments still require exact review;
no unlimited fence/accept/install growth.

P7.1's three writer record/transaction/install files retain responsibility and one durable owner.
Caller admission stays `ucupa-core/internal/handoff`; RPCs directly register module
`transport/grpc/internal/writer`; instances return actual entry.
Packages/consumers register exactly. Shared values have no state; transport imports no business
internals; unregistered later packages still reject.

At this increment transfer/fact had eleven production files/974 lines and nine tests/884.
It owns no lock, Store, authorization, or second state.
Commit preflight, intent/fence/replay, private codec, and finite-read consumers register exactly.
Only codec files use original Store byte tools; only fault tests use bbolt.
Fenced Origin tuple derives from canonical manifest/replay, not caller input.
It includes original parent/envelope for ancestor protection/unclosed coverage,
without copying closure/grants.

`transfer_read.go` creates bounded Visibility from current departures/intents and checks source
validity under sole lock at final disclosure.
It decodes no manifest/history, retains no control journal, and adds no transaction owner.
Snapshots add inactive-UUID/original-boundary mapping and finite envelope evidence under original
capacity. Local, Connected, and Federation share coverage exclusion.
Connected filters evidence within original authorized scope; Federation retains existing
Gap/Complete/bound coverage, not new wire flags.
Publication borrows the same evidence: source departure without valid successor makes related
queues GAP, not ordinary deletion.
Query/federation/observe/privacy tests follow responsibilities, not commit, with no public state/setter.

`transfer_receive.go` boundedly decodes evidence for source transport/target acceptance,
sharing Element codec, graph/closure checks, and Facet values.
It copies no writer key, source operation history, or active target state.
`transfer_accept.go` saves exact fence/full manifest/target promise in original transaction,
without install/disclosure.
Source+Operation bind UUID reservation. Journal reconstructs finite member indexes;
they are neither deletion nor local OperationID claims.
Reservations constrain conflicting writes but do not add query Visibility or turn valid empty
target results into source-departure errors.
Original guard protects target parent/ancestors/UUID; unrelated siblings/disjoint transfers sharing
a parent continue. Decode, acceptance/rollback/reply-loss, parent-delete race, and corruption tests
follow the durable owner.
SourceFence/TargetAcceptance are evidence, not credentials.
At this private-increment boundary, later workflows still needed authenticated specified-source
evidence and original-caller target approval; matching hashes alone authorize no install.
INTENT is neither source fence, target reserve, nor install, and still permits ordinary reads/writes.
Checker registers actual dependencies/counterexamples, not future files.

`transfer_successor.go` binds exact acceptance at the source. The unique decision changes no
data cut, original outcome, or source unavailability.
FenceRevision remains the original event, not latest control Revision.
`transfer_install.go` atomically installs accepted closure, issues target Element Revisions,
and advances one cut through original Store/replay/publication.
`transfer/fact/install.go` assigns members deterministically, not commits/I/O.
Read sets add only a revision-count upper bound derived from finite install evidence;
full history still proves actual issuance.
Do not reject atomic batches using the old one-revision-per-cut assumption or invent intermediate cuts.
Accept/install outcomes remain separately retained. Install releases target placement;
unsettled source still has gaps.
Initial install admitted only NONE; the later Facet-predecessor increment adds ACTIVE/DETACHED
without deleting/reviving old keys or claiming Facet content transfer.
Two commit tests use real owner; assignment/page tests stay fact/query with exact dependencies.

`transfer_settle.go` checks sole source decision, original install, and closure-departure
evidence, advancing one coverage cut under original transaction.
It changes no Element Revision/ordinary outcome and waits for no network in transactions.
Recovery returns original settlement cut, not latest.
DecisionRevision/FenceRevision bind their events.
SourceClosed retains historical invalidity; SourcePending manages unfinished protection/gaps.
Settlement does not revive old snapshots or make current departure ordinary NOT_FOUND.
Only a new cut removes settled coverage gaps.
Transfer journal reserves final-event/byte capacity: at most 6,144 bounded events,
8 MiB original manifests, and 64 MiB private journal covering input/encoding overhead
without crowding out terminal obligations.
No event system/history GC.
Fault/restart/capacity tests stay commit; query cache-cut/completeness/old-page cases stay query.

Return uses original install, not another owner/outcome/claim table.
`transfer/fact/lineage.go` validates/copies bounded continuity for selected Elements:
at most 4,096 steps per slice and original manifest bytes.
It retains historical owners/operation identity, source revision/cut, target install cut,
and decision digest, not unrelated members/tuples, credentials, or manifests.
Digests are not authentication/authorization.
`commit/transfer_lineage.go` derives continuity from actual installed records during control capture.
Receiver checks its last settled departure/immutable prefix, then restores exact inactive generation
through original target transaction.
Create still cannot reuse claimed UUIDs. Active IDs, permanent deletion, old rounds, broken/mismatched
chains reject; unfinished prior settlement stays unavailable without timeout cleanup.
One legal closure may install old/new members atomically.
`transfer_lineage_store.go` preserves non-UTF-8 OperationID bytes in original manifest codec,
not another storage/compatibility domain.
element/fact removes exact verified departure only; claims/outcomes remain and incomplete state
cannot publish independently. Three return tests stay commit; pure lineage/assignment/departure
stay fact. UUID return does not revive old query basis.

Core-side Facet predecessors share manifest/journal/atomic install, not another attachment transaction.
`transfer_facets.go` captures original Core's bounded comparisons and retains predecessor;
return to that Core checks its signer.
`facet/fact/predecessor.go` candidate rules originally covered NONE→NONE and
ACTIVE/DETACHED→DETACHED: target relation revisions differ from original keys, contact/pending
clear, and namespace/payload are untouched.
`token.PublicSigner.FacetProposal` represents original comparison, not another owner's token;
format still imports no state.
Public KeyCore identifies historical key owner, while current Revision belongs to this Core.
DETACHED has no contact; SDK cannot enlarge source, permission, or current proposal.
Pure predecessor, durable multi-Core/return/fault, same-cut page, wire/SDK cases stay their packages.
Later accepted lifecycle rules also preserve RETIRED, as specified by the current contract.
Internal hash checks alone never substitute for authenticated public/network transfer.

Root successor uses explicit original-intent profile, not a guessed zero bounded tuple.
Target allows inactive empty space or continuity-proven same-root return, never overwriting an
independent root/merging unrelated prior spaces.
One root-slot reservation constrains CreateRoot, incoming Mount prepare, and competing accept;
no separately mutable root table.
Source root-role persists in departure and replay. After export, source cannot ordinarily create
a new root or report complete empty space.
Target preserves root UUID, no Parent/Pose, unbounded Range, and one cut for the closure.
Old snapshots remain invalid.
Four commit tests cover real transactions, competing acceptance, persistence faults/retry/restart,
and corruption; pure/page cases stay fact/query.
This increment alone did not establish carrier/external-edge continuation;
uncontinued external contents cannot be merged into manifests.

Carrier parent preparation reuses MountOperation exclusion/control slots and original transfer
journal, not another mount table/log/recovery.
Core replay reconstructs complete history; snapshots retain finite current-slot evidence only.
Reservation is cancellable; fence permit only ends local cancellation, not proof of source close
or target acceptance.
Contact repair is unblocked by occupied slots. Before permit, stale contact revision cannot issue
permit: cancel/reprepare against repaired contact.
Retries cannot alter original requester, expected Element/mount revisions, parties, or addresses.
Three production files cover parent atomic control, bounded validation, private codec;
pure relations stay transfer/fact.
Four commit tests cover actual transactions, faults/races, bad records, and projections,
without public-surface expansion.
Ordinary Mount retry/decision/settlement/recovery handles only original record kinds;
empty binding cannot reinterpret carrier as an ordinary receipt.
These boundaries alone were not the later close/accept/bind/authenticated public chain.

Transfer operation identity is (actual controller Core, OperationID):
source controls slice/root; parent controls carrier.
Do not use carrier source Ref as controller.
Departure/lineage retain controller explicitly; load cannot guess/fill it for compatibility.
Finite foreign-carrier fulfillment in original transferState uses that pair, separate from local
outcome namespace, sharing intent/parent-control capacity, journal, Store, lock, and recovery.
`carrier_refusal.go` saves irrevocable refusal to fence this operation.
Only a matching source result permits parent cancellation after permit.
Refusal changes no spatial facts and is not a fence; original cut/outcome remains.
Parent releases only its own slot, not source write gate.
`carrier_refusal_store.go` shares identity/revision/cut/mount/contact codecs;
proof values are not network identity.
Two commit tests cover original outcomes, capacity, faults, concurrency, damaged history;
identity/proof cases stay fact.
Checker finite ceiling was 28 files/4,201 lines with exact registration, not all transfer/commit.
Facet namespace remained one content/commit owner: 23 production/3,520 and 20 tests/2,886.
Acceptance ceiling was 67 tests/9,681, adding two real Realm/product tests, not private-state access
or a universal fixture library. Exact checker cases do not waive whole directories.

Tests follow the actual responsibility:

- Private rule/fault/algorithm tests stay with implementation; public API tests prefer external
  test packages.
- Multi-responsibility module public combinations may use `<module>/tests`;
  cross-owner black boxes remain repository `acceptance`, not one giant tests package
  or publicly exposed fact state.
- Keep helpers at minimum scope. Static data belongs testdata; Go helpers are not disguised as data.
- Retain behavioral assertions/counterexamples. Explain remaining evidence when removing duplicate
  tests; do not delete tests merely for file count.
- Prefer explicit concurrency barriers; retain permission, cancellation, disconnection, retry,
  expiry, reclamation, and restart contracts.
- Module READMEs navigate here without copying directory rules/thresholds.

Check fixtures, relative paths, process cwd, and independent consumer builds during relocation.
Missing private fault hooks do not justify production injection APIs.
Tests needing Store seams follow commit/recovery; cross-owner scenarios still use public entries.
The repository gate must run every new package, not relocated tests in an unexecuted directory.

## 7. Validate boundaries against real call chains

These were **static responsibility checks** against P6 source, not compiled new interfaces
or migration validation:

| Chain | P6 locations at review | Behavior new boundaries must preserve |
|---|---|---|
| Element Create/Update | Core core.go → convergence.go executeMutationLocked → mutation.go → publishStateLocked | Admission order, operation conflicts, mount gates, one candidate/decision, original cut/receipt. On uncertain Store result load a valid before/after state or disable owner; never return success early |
| Core-side Facet activation | Core attachment_control.go | Original error/replay order; unlock for acceptance evidence; reauthorize/recheck relation and commit under original lock. No permanent old permit |
| Facet content writes | Facet content.go/mutation.go commitLocked/store_operations.go | Core checks, F0/view constraints, one history rule, postcommit observation, original uncertainty recovery/errors. P6 type-validation changes are a separate business revision |
| Realm authorization/credentials | Realm realm.go, binding.go, credential.go, store_commit.go | One appropriate transaction/lifecycle for Principals/grants/credentials. Copying public handles must not copy writable state/locks |

Validate proposed wiring with at least Element update plus uncertain-Store recovery, and
cross-responsibility deletion/attachment/public-read cases.
One pure mathematics package is not evidence for the whole structure.
The implementer must provide dependency DAG, opaque-value placement, and local API mapping before
implementation. A code prototype needs corresponding authorization; document derivation is
not compilation proof.

## 8. Acceptance and enforcement

These rules are accepted. Source/checker placement follows actual responsibilities;
empty roots or passing packages are not complete acceptance.
Each slice needs before/after declaration, state, dependency, and call-chain mapping,
not just a directory tree.

Common implementation gates:

1. One fact owner/state/final commit-recovery path; original lock and authorization timing.
2. Same requests, outcomes, errors, exact bytes, Store bucket/key/value/sequence, receipts,
   and replay. Verify separately accepted local Go API mappings; do not claim all public
   interfaces unchanged.
3. No old-package compatibility, full mirror models, useless forwards, or empty directories.
   Explain every added adapter and review large non-root packages too.
4. Every original test has a destination. Fixed input/identity Store evidence and fault cases
   pass; randomly created databases need not have identical whole-file bytes.
5. Freeze final files and run `go run ./cmd/verify`: checker, all tests/race/vet, dependencies,
   protocol generation, and example all pass. `-checks-only` is not full verification.
   Distinguish unrun/failed/old-baseline evidence.
6. Documents reflect delivered scope. Acceptance, commit, and push require corresponding
   authority; they do not automatically restart P7.

**Checker enforcement (2026-09-13, included in `35f9e6f3`):**
Package size distinguishes production/tests/registered generated output.
Core/Facet `contract`/`internal/token` have exact dependency rules and actual external-consumer
compilation counterexamples.
Core revisions/service evidence use one token definition.
`internal/failure` preserves only original ErrInvalid/ErrUnrepresentable identities,
depending on standard library. Geometry/token may use it; it imports no values/rules/owner.
Other modules/transport/SDK cannot cross it. It is not a generic error framework or approval
of other failure packages.

Core `internal/store` registers byte tools/four-bucket write preparation.
Only standard library/shared/values are allowed; `go.etcd.io/bbolt` is allowed solely in
transaction.go/tests, not every byte tool.
Preparation receives original transaction context/durable identity header and returns fixed
byte write ports, not full facts/callbacks/business decisions.
Original owner retains db.Update, lock, final commit, recovery, and publication;
no prebuilt whole-byte mirror.
Unregistered store subdirectories and other modules/transport/SDK remain forbidden.

Facet instance/namespace/access/request leaves register exact dependencies.
Old root source/imports reject; external consumers cannot import namespace.
Transport production uses narrow ports, not instance.
Core instance/business/transport consumers likewise register exactly, not blanket internal imports.
`transport/grpc/federationconn` owns outbound connections/finite leases;
`transport/grpc/internal/wire` owns shared codecs.
Root/federationconn may use wire; wire cannot access SDK, connection owner, root, or business internal.
SDK/other modules cannot bypass this through Go visibility.
Other transport packages need independent §5 registration.
Shared communication values use actual contract. Old Core root imports reject, with no temporary waiver.

`internal/commit/binding` owns original retry input, normalized public requests, comparisons,
and format validation shared by live admission/storage/replay.
No writable facts, grants, locks, journal, or database.
Original `publicExpectedRevision`/closure/MAC admission remains commit, not parser;
unadmitted parsed values are not verified operations.
`internal/access/value` holds field shapes/representations only, no context/grants.
Element/Path/Facet-contact JSON projections live here once.
Only `record.go`/tests may use `token.PublicSigner`, passed by callers, not owner, key manager,
writable facts, or registry. Registration gives no blanket token dependency to representation files.
Exact path/direction/access checks forbid facts/value reverse-importing binding.
Historical slice validation is not final-workspace proof.
Declaration mapping, fixed-Store equivalence, size, and final gate stay in the plan,
not a second status map here.

`transport/grpc/mountconn` owns original addressed outbound adapters, client index,
and per-call approval copies, not Server sessions/admission.
Constructors use it directly without old-root aliases.
Server checks expected-source assembly without approvals.
Inbound/outbound share wire binding shape, not copied authorization/command commitments.

`internal/element/fact` has exact consumers for module facts/commit/recovery.
It uses contract, geometry, token, shared/values, not root, instance, Store, workflows, Realm,
or transport. External consumers/SDK/transport/other owners cannot cross it.
Tables stay private; query projections receive original bounded read sets, not control/history.

`internal/facet/fact` owns current relations, deterministic Plan/Apply, bounded projections.
Dependencies are standard library, shared/values, Core contract/token, not Element facts, commit,
Store, Realm, content, or transport.
Owner supplies existence at the original time; outcomes/events/cut issuance/persistence stay owner.
SDK/transport/other owners cannot access it.

`internal/mount/fact` owns current outgoing/incoming relations, bounded reads/candidate replacement.
Only standard library/shared/values/contract dependencies.
Operation/address outcomes, attempts, journal, cuts, coordination, grants, Store, recovery
stay owner. No reverse access, writable maps, or unverified revision issuance.
Other owners/SDK/transport cannot cross it.

Both relation states embed a unique read-only View like Element.
View exposes current value reads/iteration, not Plan/Apply/Replace/Clone/recovery.
It is not independent snapshot consistency; locking, immutable publication, evidence, and release
stay owner.
Snapshot read sets hold live Views/bounded current mount evidence, not `coreState`.
Initial construction validates claims/tombstones, then releases historical input.
Every later read still checks live spatial facts; immutability does not justify removing checks.
Complete-state/live-View validation shares one deterministic rule.

Public Mount/Facet control values and trusted service results live in contract directly.
Moving types moves no business validation and adds no public Valid methods.
Service-cut parsing is not source authentication.

Core/Facet roots have zero production/test files/lines. Old consumers/helpers are removed;
checker rejects root source/imports, without aliases, RPC proxies, or test entry.
One valid implementation remains per moved area.
Implementation, transport/SDK, and tests require actual boundary acceptance,
not only root cleanup/checker success.

## 9. References and limits

[Go project layout](https://go.dev/doc/modules/layout) permits multiple packages/private
implementation, without requiring all public entries at roots or universal file-count limits.
[Method declarations](https://go.dev/ref/spec#Method_declarations) require receiver base types
in the same package; distributing a giant object's methods across directories requires design change.
[Internal directories](https://pkg.go.dev/cmd/go#hdr-Internal_Directories) restrict imports,
not network identity, permission, or runtime security.
[Interface review guidance](https://go.dev/wiki/CodeReviewComments#interfaces) favors actual use
and consumer-side interfaces. These rules do not wrap every implementation in an interface.

UCUPA uses its accepted responsibilities/placement, not Kubo/etcd/Pebble file counts as proof.
Judge actual state/dependencies, call cost, navigability, and complete verification,
not resemblance to another directory tree.
