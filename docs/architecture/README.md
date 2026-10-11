# UCUPA architecture authority manifest

This is the sole index of normative authority and current implementation contracts. It does not
define domain semantics, start development, or turn the whole target design into implemented scope.
The accepted Baseline is an engineering prototype; current status and publication gates are in the
[development plan](development-plan.md#1-当前在哪里).

## 1. Authority order

| Priority or role | Source | Responsibility |
|---|---|---|
| 1 | [Foundational axioms](ucupa-foundational-axioms.md) | System-level purpose, boundaries, and invariants |
| 2 | [Concept and mainline baseline](ucupa-concept-mainline-baseline.md) | The detailed D1–D12 semantic specification |
| 3 | Accepted current implementation contracts below | Observable behavior only within their expressly implemented scopes |
| Non-normative projection | [Platform](../../PLATFORM.md) and product READMEs | Reader-facing explanations, not competing specifications |
| Dynamic plan | [Development plan](development-plan.md) | Current scope and next gate, not domain semantics |
| Engineering constraints | [Engineering rules](../../ENGINEERING.md) | Source organization, dependencies, and review boundaries |

A difference between the target specification and a bounded implementation contract is an
implementation gap, not permission to reinterpret either silently. Contract acceptance is not
authorization to implement additional behavior. The foundational axioms and concept/mainline
baseline now use their reviewed English text as the single current specification, preserving
the accepted semantics rather than creating parallel definitions. The domain contracts below
also use English as their single current text, retaining their accepted scopes and semantics.
Language changes do not promote target semantics into implemented capabilities or authorize
additional behavior.

## 2. Core contracts

| Contract | Current scope |
|---|---|
| [Local spatial truth](core-local-spatial-truth.md) | Owner-local D1–D4, direct listing, geometry oracle, durable mutation and recovery |
| [Local query](core-local-query.md) | Explicit upward scope, independent Basis, full downward search, pagination, dual snapshot modes, and same-basis exact reads |
| [Mount base contract](core-mount-contract.md) | Accepted local mount control and combined reads; network extensions are separately defined below |
| [Core–Facet lifecycle](facet-local-truth-and-core-lifecycle.md) | Explicit attachment/contact control, exact-read envelopes, and independent owner boundaries |
| [Public access and observation](public-access-and-observation.md) | Caller-bound entry, closure access, canonical messages, retained outcomes, and finite prepared observation |
| [Ownership continuity and lifecycle](owner-transition-contract.md) | Same-Store writer handoff and supported Element/coverage transfers, relation changes, reads, and terminal observation |
| [RealmAttachment](realm-attachment-contract.md) | One persistent owner-level Realm contact, explicit local/network use, same-Realm address maintenance, online and final-boundary checks |

Supported Core transfer profiles are a single Entity, an owner-local Anchor closure without
uncontinued outgoing external edges, root successor, and simple carrier replacement. Nested
carrier transfer, positive continuation of outgoing external edges, and implicit multi-owner
atomic migration remain outside current scope. The conditional future clauses do not enable them.

## 3. Facet contracts

| Contract | Current scope |
|---|---|
| [Core–Facet lifecycle](facet-local-truth-and-core-lifecycle.md) | Stable Facet identity, proposal-bound acceptance, explicit contacts, exact content, and durable owner-local outcomes |
| [Public access and observation](public-access-and-observation.md) | Direct public content access and observation; TypeRef is an opaque 32-byte key, with bounded unchanged payload bytes and no schema validation |
| [Ownership continuity and lifecycle](owner-transition-contract.md) | Same-Store writer handoff, bounded namespace replacement, reacceptance after Core transfer, same-Core relation recovery, explicit retirement and release, and read/observation invalidation |
| [RealmAttachment](realm-attachment-contract.md) | Persistent authorization contact in the existing namespace commit domain and final generation/evidence checks |
| [Store mechanism decision](phase-9-database-decision.md) | Bounded bbolt mechanism, not domain semantics |
| [Private Store recognition](private-store-format-id-correction.md) | Owner-private recognition principles, not a compatibility lineage |

Lifecycle changes do not revive old approvals or introduce automatic cleanup, activation,
cross-owner atomic transactions, or alternate Store readers.

## 4. Realm contracts

| Contract | Current scope |
|---|---|
| [Minimum model](realm-minimum-model-correction.md) | Exact PrincipalRef, PermissionID, and OperationScope |
| [Permission-binding administration](realm-owned-permission-binding-administration.md) | Binding Put/Delete, aggregate CAS, original outcomes, and durable owner |
| [Online caller authentication](realm-owned-online-caller-authentication.md) | Caller establishment, credential lifecycle, convergence, and Store recognition |
| [Public access and observation](public-access-and-observation.md) | Caller/view and exact closure-scope increments without replacing the Realm-owned kernel |
| [Ownership command authorization](owner-transition-contract.md#4-command-evidence-and-permissions) | Original caller and independent exact owner permissions for supported transitions |
| [RealmAttachment](realm-attachment-contract.md) | Independent typed Realm product, finite caller/view/denial handles, online verification, and bounded Core/Facet network combinations |

These contracts do not add global authorization state, offline Allow, copied permissions,
implicit trust, remote identity/grant administration, or global atomic revocation. Sharing one
Realm means using one actual Realm owner, not copying its Store or permissions.

## 5. Network, composition, and other modules

The [network access contract](network-access-contract.md), sections 1–7 and 8.1–8.6, owns the
accepted specified-address gRPC/Protobuf/TLS, bounded session/delegation, contact, and public
federation increment. It extends the local contracts rather than retroactively renaming them
as network contracts. See the [validation boundaries](development-plan.md#4-validation-boundaries).

Composition only assembles explicit owner connections. It does not own spatial facts,
authorization, transfer state, or a global locator. Realm network waits remain outside fact
locks and are followed by bounded checks at the original local boundary. Local and network
profiles do not fall back to each other. Fixed-cell executable evidence does not create a new owner.

Operator and Archive are deferred packaging directions without new current implementation
contracts. Console is not a UCUPA product. Rendering and visual administration belong to external
applications; a directory, test, diagram, or README cannot promote a candidate into current scope.

## 6. Publication scope and predecessor references

This source snapshot retains current specifications, contracts, and engineering constraints.
Private implementation journals, superseded designs, frozen development rationale, and original
Git history are not included. Remaining predecessor mentions explain context only; they do not
restore an old authority, old protocol, Store reader, fallback, or migration obligation.

Publication edits to navigation and status do not accept new domain rules or enlarge the
Baseline's implementation or validation scope.

## 7. Maintenance rules

1. Keep one active semantic mainline and one current status/gate map.
2. Axioms constrain the detailed baseline; current contracts describe their exact implemented scopes.
3. Keep one definition of each fact. Cross-owner contracts refer to, rather than duplicate, other owners' rules.
4. A successor must update this manifest and affected predecessor notices together; it does not add compatibility branches.
5. Product introductions and diagrams remain non-normative projections.
6. Engineering constraints do not define domain semantics or implementation progress.
7. Historical labels never become default requirements; unimplemented profiles remain unimplemented.
8. Preserve explicit verification limits and distinguish design, implementation, verification, acceptance, commit, push, and publication.
