# UCUPA agent instructions

This is the long-term agent entry point for the entire `ucupa` repository: design,
implementation, testing, review, and delivery. It defines working practices, not domain
semantics, engineering rules, current phase, progress, or task authorization.
UCUPA is independent infrastructure. A downstream application's business requirements must
not become public UCUPA responsibilities without a confirmed design.

## 1. Sources and reading order

- [Architecture authority manifest](docs/architecture/README.md): effective specifications,
  contracts, evidence, and historical material.
- [Development plan](docs/architecture/development-plan.md): current work, sequence,
  completion gates, and handoff information.
- [Engineering rules](ENGINEERING.md): directories, dependencies, public boundaries,
  file organization, and engineering checks.
- [Run guide](RUN.md): builds, tools, verification, and actual operation.

Read the documents and sections relevant to the task. When resuming development, first
confirm the plan's current status and slice, then read the applicable specifications,
contracts, and source. A local documentation, comment, or wording edit does not require
reading the entire plan and engineering specification, but do not skip affected constraints
or implement from a chat summary alone.

Only the authority manifest defines document precedence; this file creates no second order.
Distinguish accepted design, current implementation, candidates, and historical evidence.
An accepted design is not proof of implementation. When sources conflict, check document
status, code, and tests to distinguish an implementation gap, stale record, or defect.
Do not silently choose the convenient interpretation. If resolving the conflict changes
accepted behavior or expands scope, report the evidence and request a decision.

## 2. Starting and resuming work

- Work and run repository-wide commands from the `ucupa` root containing this file, not
  the parent workspace. If handed off from the parent, locate this repository and explicitly
  read this file; do not assume nested instructions were loaded automatically.
- Confirm repository, branch, HEAD, staging, uncommitted changes, and untracked files before
  choosing the working baseline.
- Derive the objective, authorization, completion conditions, and exclusions from the
  current user request and plan.
- Check completed and remaining work and the exact file state covered by recent validation.
  Progress before an interruption is not completion evidence.
- Existing changes belong to the working baseline. Do not replace them with HEAD or delete,
  revert, stage, or clean unrelated work.
- If another task is editing overlapping files, coordinate ownership first. Do not overwrite
  its changes or accept work while competing writes continue.
- When changing checkout or handing work to another agent, preserve uncommitted and untracked
  results in full; a commit hash alone is insufficient.

Within authorization, follow the confirmed next step without repeatedly reopening accepted
design decisions. Report evidence and impact when a substantive blocker appears.

## 3. Engineering responsibility and authorization

The agent owns routine engineering research, recommendations, naming, decomposition,
implementation order, testing, and technical review. Do not ask the user to choose ordinary
technical details or perform the engineer's correctness checks.

The user decides product objectives, scope, and consequential tradeoffs affecting behavior,
calling conventions, security, availability, or cost. When a user decision is needed, lead
with a recommendation, evidence, and impact, not an unranked list of options.

- Explanations, reviews, and status requests do not authorize edits; design discussion does
  not automatically authorize implementation.
- Authorization for one slice does not authorize later phases, extra modules, or another
  refactoring round.
- Commits, pushes, and destructive operations require corresponding authorization. Do not
  request it again when it is already explicit.
- Complete necessary implementation, validation, and cleanup within scope. If additional
  authority or scope is required, stop at that boundary and explain why.

## 4. Implementation discipline

Follow the engineering rules for directories, dependencies, and size limits; do not duplicate
their directory trees or numeric thresholds here.

- Before adding a file or package, identify responsibility, fact ownership, dependency
  direction, and real consumers. Do not decompose mechanically by operation step.
- Keep one authoritative state, commit path, and recovery mechanism per logical owner.
  Do not duplicate state, rules, or whole models to make directories look tidy.
- Greenfield: use the accepted single interface and implementation. Do not add old import
  compatibility, parallel entry points, protocol fallbacks, or speculative placeholder abstractions.
- Greenfield does not permit silent behavior changes. Explain and validate structural,
  public-interface, and domain-semantic changes separately.
- Do not expose private implementation merely to ease relocation. Separate source from
  generated output and follow the run guide's generation rules.
- Update real consumers, tests, and necessary documentation. Remove code made obsolete by
  this change without expanding into unrelated cleanup.
- Do not hide defects by deleting counterexamples, skipping failing tests, relaxing
  dependency rules, or raising size limits. If an engineering rule genuinely needs revision,
  provide separate responsibility and dependency evidence and follow its review procedure.
- Add no unauthorized product dependency, external service, or persistence-semantic change.
  Logs and delivery material must not expose credentials or secrets.

## 5. Validation and delivery

During development, run targeted tests and engineering checks proportional to risk.
When fixing a defect, add a counterexample that demonstrates the original problem.
Before code delivery, run the full command at the repository root on the final file set:

```text
go run ./cmd/verify
```

`go run ./cmd/verify -checks-only` checks engineering constraints, not the full runtime gate.
Tool and environment requirements are defined in the run guide. Report missing tools or
unrun validation explicitly; neither counts as a pass.

Freeze the delivery files during full verification. If code, protocol, or result-affecting
configuration changes afterward, rerun the full gate on the final set before delivery.
Record command, outcome, environment, and baseline. A green run on an older commit or another
workspace does not validate current changes. For refactoring, verify behavior, public surface,
and storage/recovery rules, not just compilation or file counts.

For documentation-only edits, check content consistency, references, links, and diff as
appropriate; do not claim a fresh code or runtime acceptance. Briefly state changes,
verification, uncovered scope, and blockers. Record design confirmation, implementation,
verification, user acceptance, commit, and push separately.

Claim only capabilities demonstrated in the actual validation environment. Do not promote
local simulation into real multi-host, production, or scale validation.

## 6. Collaboration and handoff

Delegate only with collaboration authorization, not merely because tools exist.
Every subtask needs an explicit objective, permitted files or responsibilities, immutable
boundaries, and verifiable completion conditions. Prefer independent bounded tasks.
The main task coordinates shared state, transactions, and interface changes to avoid
overlapping writes, and owns integration, review, and final verification. A subtask's completion
message alone is not delivery evidence.

Before interruption or handoff, update the current progress section in the existing plan with:

- Objective, completed and remaining work, and the next contract, source, and test entry points.
- Branch/commit, uncommitted baseline, untracked results, ownership, and active work.
- Latest verification command, result, scope, unverified items, and concrete blockers.
- Authorization boundaries, immutable invariants, and explicit completion conditions.

Keep one current entry for dynamic task information. Do not copy it into parallel plans or
this file. A receiving agent must recheck the workspace and evidence rather than accept
the handoff summary as unverified completion proof.

## 7. Maintaining this file

Edit this file only when long-term working practices change; update the development plan
for phase and progress changes. Do not add phase-specific tasks, commit hashes, temporary
failures, file counts, validation logs, fixed models, or subtask lists.

Keep one root-level instruction file. Do not create repeated module instructions for directory
symmetry; add narrowly scoped instructions only for a demonstrated local difference.
