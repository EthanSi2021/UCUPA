# UCUPA local demo

**Spatial infrastructure for the united cyber universe**

UCUPA stands for **The system of United Cyber Universe Position and Anchor**.
It organizes a world through nested local frames, with independent operators maintaining their
own parts of the same spatial structure.

This guide demonstrates the current engineering prototype through a reproducible local program.
Start with the [project introduction](../README.md); the [architecture index](architecture/README.md)
identifies the contracts that define its behavior.

## Run the example

From a source checkout, open the repository root containing `go.mod`. Use Go 1.25 or later,
as required by that file:

```text
go version
go run ./cmd/ucupa
```

Go downloads the pinned dependencies on the first run unless they are already cached. This
example needs no separate database or service, C compiler, Protobuf compiler, or Python runtime.
Additional tools are required for the [full verifier](../RUN.md#verify), not for this example.

The program uses local connections and temporary stores. Its two-Core directories are created
for the run and removed on normal completion. It does not connect to a production deployment.

Successful execution exits with code 0 and prints all five lines:

```text
UCUPA: PASS
cluster: cluster-p/realm-p owner-local
element: 00000000-0000-4000-8000-000000003004 root-relative=(3,0,0) cross-branch query+same-cut read; identity+facet preserved; deleted subject hidden
connected: two durable Core stores; upward discovery, snapshot pagination+exact read across writes; latest state recovered
public: canonical entry+create+discovery+read; direct Facet and gapless prepared observation
```

The first PASS line alone is insufficient: the final scenario must finish and the process must
exit successfully. The example’s regression test can be run separately:

```text
go test ./cmd/ucupa -count=1
```

## What the results prove

| Output | Demonstrated behavior | Evidence boundary |
|---|---|---|
| `element` | Cross-branch discovery and a read at the same cut; a spatial change preserves identity and Facet content; deletion stops that content being served as current | Does not cover every transfer shape or fault combination |
| `connected` | Two independent durable Core stores in one process; upward discovery through a mount; retained-snapshot pagination and exact reads across a write; reopening verifies the latest durable value | Uses `local:` connections, not network RPC or two machines |
| `public` | Enter, create, discover, read, Facet content access, and prepared observation through local public envelopes | Does not demonstrate a browser client, TLS deployment, or global atomic snapshot |

The program’s scenarios are implemented in `cmd/ucupa/main.go`, `connected.go`, and `public.go`.
Network products and their separate validation evidence are described in the [run guide](../RUN.md)
and [Baseline scope summary](architecture/development-plan.md#1-当前在哪里).
Success here is not a benchmark, a public-network demonstration, or a full-verifier result.

## Two-minute introduction video

This is a recording script, not an existing video. Use a continuous terminal recording for the
run and clearly label explanatory diagrams as illustrative.

| Time | Visual | Suggested narration |
|---|---|---|
| 0–20 s | Project headline and anchor tree; show the engineering-prototype status separately | “How do you support a world that keeps growing, while preserving dense local detail? UCUPA provides spatial infrastructure for the united cyber universe.” |
| 20–40 s | Tree and 3D organization diagram, followed by the three module roles | “Objects are positioned relative to parent anchors. Those frames compose through the tree, and different Core owners can maintain its parts. Facet stores optional content; Realm checks identity and permissions.” |
| 40–75 s | Record `go version`, `go run ./cmd/ucupa`, all five output lines, and the successful exit without cuts | “This local program exercises discovery, content access, observation, and reads across two independent Core stores.” |
| 75–105 s | Highlight the `connected` and `public` lines alongside the evidence table | “Writes persist normally. Retained snapshots let this query continue reading its earlier state. The public example enters a space, finds an entity, reads its content, and follows a committed change.” |
| 105–120 s | Demo guide and project status; add the confirmed repository link only after publication | “The engineering Baseline is complete within its declared scope. Run the example to inspect the behavior; larger deployments still need their own validation.” |

Use sample data and temporary identities. Hide credentials, private repository addresses, local
usernames, and unrelated windows. If the run fails, fix it and record another complete run;
do not splice different runs into apparent success.

Network faults, permission revocation, and a live 3D client can be separate demonstrations only
when backed by their own actual execution paths. They are not demonstrated by this local run.

## Website and presentation

Use the same project headline and full name as the README. Keep the engineering-prototype label
separate from the project’s purpose.

- Explain growth in spatial extent and local detail through an anchor tree, parent-relative
  frames, and the distribution of one tree across Core owners.
- Show Core, Facet, Realm, and the five public behaviors: Enter, Discover, Read, Observe, Propose.
- Provide the reproducible command and complete expected results. Add a video only once recorded;
  link the public repository and private security-reporting channel only when they are available.
- Distinguish implemented behavior from future directions and validation limits. Operator and
  Archive remain candidates; there is no built-in Console or publicly writable demo service.

The website’s 3D diagrams illustrate organization; they are not live service connections or
runtime evidence. A future live visualization client must read facts through public interfaces
without becoming another authority or an internal UCUPA UI. Rendering frame rate is not a
UCUPA throughput or latency measurement.
