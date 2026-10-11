# UCUPA Facet

Facet stores Element content keyed by an opaque type reference. It stores no spatial facts
and does not decide Element ownership. Stage 1 P4's local cycle was implemented, fully verified,
and accepted with user authorization. The [plan](../docs/architecture/development-plan.md)
records delivery status; the architecture manifest defines current scope.

The implementation follows the [Core/Facet local contract](../docs/architecture/facet-local-truth-and-core-lifecycle.md)
and repository [engineering rules](../ENGINEERING.md).

The lifecycle entry is [`instance`](instance/instance.go): `instance.New`,
`CreateDurable`, and `OpenDurable` return a handle; call content/contact operations
directly through `handle.Namespace` and close with `handle.Close()`.
The instance assembles dependencies and manages lifecycle, not per-method business forwarding.
Shared Go values, results, errors, and necessary injected ports are in
[`contract`](contract/namespace.go). [`internal/token`](internal/token/token.go)
uniquely defines private token representation. Authoritative content, acceptance, commits,
and recovery belong to [`internal/namespace`](internal/namespace/owner.go).
Bounded context/observation resources belong to [`internal/access`](internal/access/resources.go).
The root contains no old entry, alias, or tests. Relocation preserved P6 comparisons,
preconditions, type representation, and storage semantics; the plan records frozen validation,
not a second refactoring journal here.

## Access chain and public operations

Core proposes attachment → Facet accepts → Core activates. Acceptance alone does not enable
content access. Core's ACTIVE attachment returns Facet identity, specified address, and exact
relationship key. The caller connects by address. Facet uses its stored reverse contact to
check the original Core is still PRESENT with matching ACTIVE.

- `AcceptAttachment`: atomically retain immutable acceptance and initial Core contact.
- `ReadAcceptance`: return original acceptance, current reverse contact, and Facet cut;
  exact absence has independent evidence.
- `UpdateCoreNextHop`: local authorization/contact CAS only; Core need not be online.
- `Get`/`ListTypes`: one F0 validation; return content/all current types and actual
  per-owner evidence.
- `Put`/`Delete`: conditional key mutation, independent commit, exact operation replay.

Get/Put/Delete use `FacetKeyScope(self, Element, TypeRef)`. Acceptance, acceptance reads,
contact updates, and ListTypes use `FacetElementScope(self, Element)`.
Namespace permission is not a wildcard for key permission; a service caller does not
automatically inherit an end user's permissions.

## Types and consistency

Since P7.3a, `FacetTypeRef` is any 32-byte value, not an address, name, or version.
Facet retrieves/parses no type artifact and performs no schema validation. Put stores bounded
opaque Payload bytes unchanged at `(Element, TypeRef)`. The payload limit is 1 MiB;
each Element has at most 256 current types. ListTypes returns the complete raw-byte-ordered set.

Every fresh content operation checks Core. Get/ListTypes capture Facet F0, acceptance, and
contact, read Core outside the lock, then verify Facet cut is unchanged; otherwise
`ErrSnapshotChanged` prevents mixed results. Once Put/Delete obtain valid Core evidence,
they may commit interleaved with later detach/delete. The result reports the actual Core cut;
subsequent fresh reads hide invalidated content. No cross-Store atomicity is claimed.

Missing acceptance, unreachable contact, and wrong identity are not Element absence.
Element absence, NONE, DETACHED, non-current owner, changed relationship, missing key,
changed snapshot, and denial remain distinct. No physical orphan Reconcile, automatic
reattachment, bare-Ref lookup, or fallback address exists. Explicit supported P7 lifecycle
operations are separate from this original content-access chain; they do not add automatic
content migration.

## State, commits, and recovery

New creates a stable Facet identity; CreateDurable exclusively creates, and OpenDurable restores.
All three constructors require explicit Realm connection assembly, CoreSource, and trusted
initialization RealmID plus explicit local/network address. Creation verifies the assembly
reaches that Realm before binding identity; reopen restores and checks the contact from the
original Store. There is no bare Authorizer injection, and configuration cannot override or
invent contacts. Omitted PublicAccessPolicy disables the public profile, not ordinary local
operations; no TTL default is supplied.

One owner has one state lock, state, commit history, and OperationID namespace.
Ordinary authorization and Core checks occur outside the lock; no type artifacts are fetched.
Authorization views come from this owner's current Realm contact. Network acquisition/revalidation
occurs outside the lock; final comparison, ctx, contact-generation checks, commit, and disclosure
share the same owner lock sequence. Do not wait for Realm under a fact lock.
Local verification remains bounded and must not call back into the owner.
One-shot views release after their last use; observation views follow the original finite
context lifetime, without another handle table or offline permit.
Principal-based local operations require trusted local assembly; network operations use
established CallerContext, not a principal name as identity.

Memory and bbolt use the same deterministic decisions and history checks; validation/acceptance
are recorded in the plan. Acceptance, Core contact, Realm contact, content, tombstones, cut,
public Revision signing key, and operation receipts persist together.
The current private format recognizes only `ucupa.facet.local@4d8e1a6f3c9b7250e6a2f8d5c1b7e3a9`,
not predecessors.

The Store holds one canonical private document: identity/scope, current fact projection,
and replayable events. Open first validates read-only, then opens without creation and rechecks
identity. Before/after commit and reopen share replay rules. No unknown-format conversion,
invented identity, second reader, or corruption repair exists. History is not compacted;
this verifiable local implementation is not large-scale storage or production optimization.

OperationID is 1..128 bytes, binding caller, owner, exact relation, operation, input, and
conditions. Exact retries reauthorize, return the original receipt without another Core call,
and never restore an old address. Definite precommit failure is unavailable; response loss
is indeterminate, recoverable by the retained result. Caller/operation/type/payload bytes
remain exact, without JSON text's UTF-8 replacement.

[Public save/recovery counterexamples](internal/namespace/public_store_recovery_test.go)
cover pre-save failure, response loss after publication, and actual load/recovery installation
when disk commit succeeded but its result is uncertain. The last uses only a package-private
test seam, not public configuration. A changed fact cut makes old observation GAP;
receipt recovery without cut change does not close observation, and exact retries do not
commit twice. Read the plan for full validation status.

## Single-writer handoff on the original Store

P7.1 implements only the same-domain profile in [P7 §6](../docs/architecture/owner-transition-contract.md#6-same-owner-writer-handoff).
Full verification and acceptance are recorded in the plan.
Call the actual namespace owner through `handle.Namespace.InspectWriter/HandoffWriter`;
SDK callers use `Writer()`. Enabled PublicAccessPolicy accounts for control, and a trusted
view independently authorizes `ucupa.facet.writer.inspect` or
`ucupa.facet.writer.handoff` on `AuthorityScope(FacetRef)`.
Content/Element permissions do not substitute.

`instance.Config.Writer` uses the same controlled startup values as Core, but this Facet
owns its journal, lock, and final commit. The source atomically fixes final cut and fences in
the original Store; in-flight Get cannot disclose old content after its Core check.
The designated instance exclusively opens the original Store and checks installation qualification.
Content, acceptance, contacts, original outcomes, and scope/keys are not rewritten for a new run.
Ordinary Open does not take over pending installation; ordinary restart does not advance epoch
or revive old run-local handles/sessions. Handoff creates no second namespace or Store copy
and does not itself change P7.3 payload/relationship semantics.
Startup fields are in the [Facet host](../cmd/ucupa-facet/README.md#single-writer-handoff).

## Validation scope

- Memory/durable lifecycle, CAS, unchanged results, independent types, input/output copies,
  replay, and authorization order.
- Type boundaries, raw-byte ordering, all 256 types, opaque-payload capacity, exact bytes.
- Acceptance, cancellation interleavings, reverse contacts, old receipts not restoring
  addresses, F0, and concurrent write/address-change barriers.
- Disconnection, wrong identity, missing acceptance, every Core state category, hidden
  invalidated content.
- Pretransaction faults, response loss, process exit, reopen, and read-only rejection of
  damaged format, facts, or history.
- Real Composition: one Core to multiple Facets, one Facet to multiple Cores, bidirectional
  address repair, and restart.

Predecessor existence/Reconcile tests were superseded under the current contract:
deleted-Element hiding remains tested; proactive physical cleanup is not product behavior.
Old free-form type/two-bucket examples were replaced by exact types and single-owner history
corruption tests. CAS, independent authorization, copies, capacity, concurrency, durability,
process exit, and Store isolation coverage remain. The sole full delivery command is
`go run ./cmd/verify`; targeted tests or checks-only are not a full pass.
