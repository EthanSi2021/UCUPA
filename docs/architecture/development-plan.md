# UCUPA status and development plan

This is the sole current status and gate map. It does not define domain semantics, authorize
new features, or replace the [architecture authority manifest](README.md).

The implementation Baseline comprises Stage 1 and Stage 2 and was accepted within its declared
scope. This source snapshot was published on 2026-10-10 in an independent Git repository on
`main` at [EthanSi2021/UCUPA](https://github.com/EthanSi2021/UCUPA). The repository is public,
and GitHub private vulnerability reporting is enabled and verified. No history from the
development repository was imported. This is source publication, not a production-readiness
claim or a versioned binary release.

<a id="1-当前在哪里"></a>

## 1. Current snapshot

UCUPA is an engineering prototype. Its positioning is:

**Spatial infrastructure for the united cyber universe**

UCUPA stands for **The system of United Cyber Universe Position and Anchor**.

This snapshot includes the current implementation, tests, public protocol sources and generated
bindings, development tools, runtime instructions, engineering rules, and effective specifications
and contracts. It excludes the private development journal, superseded designs, frozen development
rationale, original Git metadata, ignored local configuration, and runtime or build artifacts.

The source export does not change implementation bytes, public interfaces, Store formats,
authorization, transfer profiles, or recovery rules. Publication preparation is not a new
implementation phase. This document replaces the private journal only in the exported snapshot;
it does not delete, rewrite, or claim to reproduce that journal.

## 2. Implemented Baseline scope

| Area | Implemented scope | Important boundary |
|---|---|---|
| Core | Stable Element identities, local frames and ranges, parent relationships, mutations, discovery, pagination, exact reads, and owner-local durable outcomes | Finite numerical profiles and capacities; no global UUID locator or global atomic snapshot |
| Facet | Optional opaque content, explicit Core relations, exact key operations, observation, persistence, and controlled lifecycle changes | No schema interpretation, implicit cross-owner transaction, or automatic content cleanup |
| Realm | Caller credentials, exact permission bindings, online authorization, and controlled Core/Facet authorization contacts | No automatic trust, offline Allow, global authorization database, or replica takeover |
| Network and mounts | Specified-address gRPC/TLS, expected service identity, explicit mount relationships, bounded sessions and cross-Core reads | Only contract-defined profiles; not arbitrary topology or production readiness |
| Ownership continuity | Same-Store writer handoff; supported Core transfer and Facet replacement, reacceptance, recovery, retirement, read, and observation paths | No nested carrier transfer, uncontinued outgoing external edges, implicit multi-owner atomic migration, or automatic HA |
| Executable products | Local demonstration and explicit Core, Facet, and Realm hosts with finite test fixtures | Rendering, simulation, applications, and visual administration remain external |

The exact accepted scope is defined by the current contracts indexed in [README.md](README.md),
not by this summary or by a package's existence.

<a id="phase-5-实现交付记录"></a>

## 3. Public access and observation

The accepted public surface covers canonical entry, caller-bound discovery and reads,
conditional changes with retained original outcomes, and finite prepared observation. Later
contracts extend specified-address network access and supported lifecycle profiles without
turning local cuts into a global revision. Facet TypeRef is an opaque 32-byte key; payload bytes
are bounded and preserved, not parsed or schema-validated by Facet.

This is a scope summary; verification evidence for this exported directory is recorded below.

## 4. Validation boundaries

The accepted development Baseline has local, durable-store, process, gRPC/TLS, and selected
two-host test evidence. Those records belong to the private development repository and are not
republished here as a full implementation journal.

The local demo uses local connections in one process; it is not a network deployment or a
benchmark. Realm network combinations have independent-product loopback evidence, not verified
production or real multi-host Realm deployment. Selected two-host Core/Facet checks do not prove
all public-network fault conditions or deployment topologies.

Finite precision, resource limits, and explicit unavailable or partial outcomes remain part of
the supported profiles. Baseline completion does not imply infinite capacity, production-scale
performance, automatic high availability, or completion of every longer-term direction.

## 5. Publication gates

1. Completed: review the public file set, document links, dependency notices, and sensitive-content scan.
2. Completed: initialize this snapshot's own empty Git repository on `main`, with explicit authorization.
3. Completed: freeze the exact source file set and pass the full verifier from the new repository root.
4. Completed: confirm the GitHub destination, private visibility, empty remote, and account write permission.
5. Completed: create the independent initial commit and push `main` to the private destination;
   verify that local and remote `main` agree and that the repository remains private.
6. Completed: commit and push the [security policy](../../SECURITY.md) and README guidance;
   select GitHub private vulnerability reporting without publishing a personal contact address.
7. Completed with explicit authorization: make the repository public and enable GitHub private
   vulnerability reporting; verify anonymous repository access and the reporting status.

No release tag or binary distribution was created. Source publication does not authorize a new
feature phase, website deployment, or additional hosted service.
The website remains a separate project; its diagrams are illustrative, not live service evidence.

### Local preparation checks

On 2026-10-09, the exported implementation, tests, protocol/schema inputs, generated bindings,
and dependency files were compared byte-for-byte with the source workspace. No implementation
files were changed or omitted. Document links and copied upstream license/notice texts were
checked, and local sensitive-content scans were run without uploading the source.

Gitleaks 8.30.1 reported one generic-key candidate in `cmd/ucupa-realm/config_test.go`, in the
explicit `validBootstrap` test fixture used by `TestReadBootstrapParsesExplicitState`. It was
reviewed as test data, not a discovered deployment credential; the scanner finding was not
hidden or suppressed. A scan cannot prove the absence of all secrets.

With Go 1.26.3 on Windows and network dependency downloads disabled, `go run ./cmd/ucupa`
completed from this snapshot, printed all five expected lines, and exited with code 0. This is
local demonstration evidence only.

On 2026-10-10, references to unrelated internal application projects were replaced with generic
consumer descriptions in four documents. Domain rules, links, implementation files, and the
original development workspace were unchanged. With explicit authorization, this snapshot's
own empty Git repository was initialized on `main`; no commit, staging, remote configuration,
push, or publication was performed.

On the same date, `go run ./cmd/verify` completed from this independent repository on Windows
with Go 1.26.3, protoc 34.1, the pinned plugins, WinLibs GCC 16.1.0, and the required Python
interop environment. Network dependency downloads were disabled. Engineering checks reported
0 errors and 0 reviews; fixed protocol generation, all ordinary tests, race tests, vet,
`go mod tidy -diff`, the local demo, and diff whitespace all passed, with exit code 0.
All 1,307 source files remained byte-identical throughout verification, and the original
development repository's HEAD and working-tree status were unchanged. This is local Windows
verification, not new real multi-host, production, or scale evidence.

After the full gate, only this plan and the root README were updated to record verification
and publication preparation; documentation checks were repeated without changing implementation
files. Read-only GitHub checks confirmed the specified account's write permission and an empty
private destination. The user then authorized the initial commit and private push. Commit
metadata uses the specified GitHub account alias and its no-reply address rather than a personal
email address. This does not add a personal copyright attribution. The actual commit and remote
synchronization state are recorded by Git. Subsequent checks confirmed the initial commit's
private upload, matching local and remote `main`, and an unchanged original development workspace.

The security-policy preparation adds only `SECURITY.md` and updates the root README and this
plan. Content, local links, whitespace, and the unchanged implementation bytes were checked;
this documentation-only step does not claim a new full runtime verification. With explicit user
authorization, the policy was committed and pushed, the repository was made public, and GitHub
private vulnerability reporting was enabled. The enable request returned HTTP 204; authenticated
and anonymous checks both returned `enabled: true`, and anonymous repository metadata confirmed
public visibility. The README, security policy, and this plan were then synchronized with those
actual results. No personal email address or response-time commitment was introduced, and no
test vulnerability report or public security advisory was submitted.

<a id="6-stage-2realm-授权访问链"></a>

## 6. Stage 2: Realm authorization access

Stage 2's accepted scope is S2P1 design, S2P2 owner-local persistent Realm contacts, and S2P3a–d
typed Realm services, network Core and Facet consumers, and bounded product combinations.
The [RealmAttachment contract](realm-attachment-contract.md) defines this scope.

Each Core or Facet retains its own single persistent contact. Local and network profiles are
explicit and do not fall back to each other. Online waits happen outside fact locks, followed
by final request, evidence, contact-generation, and local-fact checks at the original boundary.
Remote administration, changing RealmID, automatic trust, and malicious private-key clones are
not silently enabled or claimed to be solved.

<a id="7-所有代码交付共同遵守的验证与变更规则"></a>

## 7. Verification and change rules

Code delivery requires the full repository command on the frozen final file set:

```text
go run ./cmd/verify
```

Engineering-only checks or the local demo do not replace its protocol-generation, test, race,
vet, dependency, demo, and whitespace gates. Missing tools or failing gates remain failures.
The verifier requires this project's own Git checkout; see [RUN.md](../../RUN.md#verify).

Documentation-only preparation uses content, reference, byte-preservation, and safety checks;
it is not reported as a fresh full-runtime verification. Implementation, verification, acceptance,
commit, push, and publication are distinct states.

## 8. Deferred directions

Operator, Archive, deployment automation, performance/capacity work, HA, multi-region operation,
dynamic partitioning, and scale work require a concrete scenario, a bounded contract, and
separate authorization. There is no automatically scheduled next phase or revived Console.

## 9. English documentation: first batch

On 2026-10-10, the first documentation-only batch translates the Core and Facet module/API
READMEs, all three product-host READMEs, PLATFORM, ENGINEERING, and AGENTS into English.
The Realm module README was already English and is unchanged. The ten translated documents
and this progress record are the only changed files above the published baseline.

Configuration fields and values, executable commands, permission names, Store markers,
resource limits, dependency boundaries, and engineering ceilings are preserved. Stale progress
wording is aligned with accepted delivery and actual tests, while local, loopback, and bounded
network evidence remain explicitly qualified. Historical engineering increments are retained
as history, not new capacity allowances.

Documentation checks passed: 287 local links and anchors, 22 configuration/command blocks,
diff whitespace, and byte preservation of every non-documentation file. The original private
development workspace is unchanged. On Windows, `go run ./cmd/verify -checks-only` reported
0 errors and 0 responsibility reviews; runtime gates were not rerun for these prose-only edits.

At the end of the first batch, the Chinese axioms and domain contracts remained unchanged
and authoritative. Their English translation is a separate second batch, not an implied
change of semantics or authority.
The user has authorized a local commit of this first batch, followed by the second batch.
Git records the actual commit state. No push is authorized in this step; the second batch
remains documentation-only and must preserve the accepted semantics and implementation scope.

## 10. English documentation: second batch reviewed

The first batch is committed locally as `42740d7`; it has not been pushed. The second batch
starts from that clean commit and does not authorize implementation or a remote update.

The first completed unit is the
[foundational axioms](ucupa-foundational-axioms.md) and
[concept and mainline baseline](ucupa-concept-mainline-baseline.md). Their English text replaces
the two language-suffixed files, with incoming paths and heading anchors updated together.
The authority manifest retains its original precedence and one current definition; this is
a language replacement, not a new set of axioms or target semantics.

All A1–A10 and D1–D12 decisions, falsifiability gates, examples, formulas, and boundaries are
retained. The Realm contact supplement's former pending-implementation statement is explicitly
historical; current delivery scope remains in this plan and the existing contracts.

The second completed unit is the [Core local spatial-truth contract](core-local-spatial-truth.md)
and [Core local spatial-query contract](core-local-query.md), translated in place. The original
G1 slice's limits remain explicitly distinguished from the entire current Core; its successors
retain their existing responsibilities. Numeric rules, formulas, API examples, permission and
error identifiers, decision order, snapshot lifecycle, and acceptance conditions are preserved.
The historical slice marker and Go declarations are not rewritten as today's implementation
format or constructor signatures.

The third completed unit is the [Core mount contract](core-mount-contract.md) and
[Core–Facet fact and lifecycle contract](facet-local-truth-and-core-lifecycle.md), translated
in place. Stable identity, directional contacts, independent owner commits, control and
attachment state machines, F0 read checks, original outcomes, dual query modes, and failure
boundaries are preserved. P2/P3/P4 limits are distinguished from explicitly named successors;
P4's former source-file mapping is now expressly historical, not a claim about today's layout.

The fourth completed unit is the [public-access and fact-observation contract](public-access-and-observation.md)
and [ownership-continuity and lifecycle contract](owner-transition-contract.md), translated in
place. The five public behavior categories, explicit entry and closure authorization, independent
Facet permissions, prepared reads, bounded observation, GAP/UNAVAILABLE distinctions, opaque
content, and original-outcome recovery are preserved. Ownership transitions retain exact
operation binding, irreversible fences, acceptance and installation obligations, persistent
continuity evidence, and first-supported versus conditional external-edge profiles. Historical
design commits are identified as original development history, not commits in this public
snapshot. Obsolete P5 delivery-record section numbers now refer to the development plan without
an incorrect section anchor; the P7.3a opaque-content revision remains explicit.

The fifth completed unit is the [network-access contract](network-access-contract.md) and
[RealmAttachment contract](realm-attachment-contract.md), translated in place. Target identity,
online caller and view checks, bounded network evidence, query composition, persistent owner
contacts, same-Realm repair, contact-generation checks, and lock-free authorization waits with
final fact-boundary guards are preserved. Original P6 checkpoints are explicitly historical
where the same contract already records later delivery. The former JSON-payload wording now
matches the accepted opaque-content rule: exact Payload bytes, not type-schema validation.

All ten specifications and contracts in this second batch now have English bodies. The other
Realm and Store contract bodies were already English; their reference-only edits do not claim
new translations or new capabilities. This corrects the earlier, overbroad statement that all
network, Realm, and Store contracts remained untranslated. Continue from the existing authority
manifest and plan, not a parallel definition or new implementation phase.

Checks passed for the first unit: 289 local links/anchors, unchanged heading levels and numbered
clause sequences, preserved technical identifiers and non-diagram example bodies (apart from
translated comments), and clean diff whitespace. The two specifications retain respectively
17/39 headings, 12/132 numbered clauses, and 2/19 fenced examples. All non-documentation bytes
remain unchanged; the original private workspace was not modified.

Checks passed for the second unit: local links/anchors, heading levels, numbered clause sequences,
technical identifiers, numerical formulas, and fenced examples, including the unchanged Go API
block. The spatial/query contracts retain respectively 32/11 headings, 16/30 numbered clauses,
and 17/1 fenced examples. At that unit's completion, 291 local links/anchors were checked.
Only these two contracts and this plan changed during the second unit; the earlier unit's work
was preserved. All non-documentation bytes and the original private workspace remain unchanged.

Checks passed for the third unit: 293 local links/anchors, heading levels, numbered clause
sequences, technical identifiers, API/state-machine examples, diff whitespace, and byte
preservation. The mount/Core–Facet contracts retain respectively 32/25 headings, 19/18 numbered
clauses, 9/17 fenced examples, 100/108 list items, and 129/33 table rows. Human-readable diagrams,
labels, and comments are translated; executable signatures and state-machine transitions are
preserved. Five other documents have only incoming heading-reference repairs. Earlier work,
all non-documentation bytes, and the original private workspace remain unchanged.

Checks passed for the fourth unit: 295 local links/anchors, unchanged heading levels and numbered
clause sequences, retained technical identifiers, diff whitespace, and byte preservation.
The public-access/ownership contracts retain respectively 27/25 headings, 17/14 numbered
clauses, 1/0 fenced examples, 17/13 list items, and 111/74 table rows. The human-readable public
behavior diagram is translated; machine identifiers and the transition sequence are preserved.
Four other documents have only incoming heading-reference repairs. Earlier second-batch work,
all non-documentation bytes, and the original private workspace remain unchanged.

Checks passed for the fifth unit and whole-batch review on 2026-10-11: 297 local links/anchors,
heading levels, numbered clause sequences, technical identifiers, numeric constraints, table
structure, and diff whitespace. The network/RealmAttachment contracts retain respectively
23/14 headings, 17/7 numbered clauses, 0/1 fenced examples, 34/9 list items, and 67/50 table rows.
Two other documents have only incoming network-heading repairs. The final review also translated
one remaining human-readable comment in the existing mount example; its identifiers are unchanged.
Earlier second-batch work is otherwise preserved.

The whole-batch review checked terminology, authority, original slice versus successor scope,
and historical versus current implementation claims. All ten translated specifications/contracts
retain their section structure, technical identifiers, and executable examples; human-readable
diagrams and comments are English. Remaining Chinese characters in Markdown occur only in
preserved navigation fragment IDs and links to them, not reader-facing prose. No parallel Chinese
specification remains. All non-documentation bytes, license files, and the original private
development workspace are unchanged.

On Windows, `go run ./cmd/verify -checks-only` reported 0 errors and 0 responsibility reviews.
Runtime gates were not rerun for these documentation-only edits; this is not fresh code or runtime
acceptance. The second batch and its technical documentation review are complete. On 2026-10-11,
the user approved the reviewed batch and authorized its local commit. This progress record is
included in that documentation commit; Git records the actual commit state. The translation and
review baseline is `42740d7`. No push, new branch, or implementation phase is authorized by this
step.
