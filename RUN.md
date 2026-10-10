# Running UCUPA

This page explains how to build, verify, and run the current checkout. It does
not classify architecture documents or track project gates. See the
[architecture authority manifest](docs/architecture/README.md) for current
contracts and the [development plan](docs/architecture/development-plan.md) for
the current action.

## Verify

From the root of a Git checkout containing this project's `go.mod` and `.git`:

```powershell
go run ./cmd/verify
```

The verifier finds the repository root by looking for `.git`. A source snapshot without its
own Git repository is not a valid verifier checkout: do not run it from a directory nested
inside an unrelated Git repository. The local example below can run without `.git`.

This is the single delivery verification entry. It checks source organization,
production dependency boundaries, physical file size, and formatting without
rewriting files. The scan includes new, untracked source files. It then runs
the full test suite, the required race suite, vet, `go mod tidy -diff`, the
local demonstration, and `git diff --check`. Failed or unavailable gates make
the command fail; responsibility-review warnings still require engineering
review as described in the [engineering rules](ENGINEERING.md).

For a quick engineering-only check during editing:

```text
go run ./cmd/verify -checks-only
```

The quick mode does not replace delivery verification. The full command sets
`CGO_ENABLED=1` for the race gate. On Windows, that gate requires a working C
compiler on `PATH`. That is a Go
race-detector test-toolchain requirement, not a UCUPA production runtime
dependency.

The pinned Go dependencies are in `go.mod`/`go.sum`: bbolt, JSON Schema, and the
network protocol/runtime dependencies. No Node.js, container runtime or external
database service is required. Production services do not need a Protobuf compiler.
Full delivery verification now also regenerates the network bindings into a
temporary directory and compares them. Install the pinned development tools:

- `protoc` 34.1 (the executable must report `libprotoc 34.1` and be on `PATH`);
- `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12`;
- `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2`.

`go run ./cmd/protogen` generates the registered binding files from their
`.proto` sources. `go run ./cmd/protogen -check` only compares; it does not rewrite
the checkout. The full verifier includes this check and fails if tools are absent
or different. Dependency versions are tool/runtime pins, not UCUPA API versions.

The independent implementation conformance gate additionally uses an isolated
Python 3.12 environment and pinned packages. Follow the
[interop tool setup](acceptance/testdata/interop/README.md#tool-setup).
It is test-only, not a product dependency. Missing tools fail the full test
suite; tests never install packages or access a package registry.

## Observable demonstration

The checkout provides:

- independent local Core, Facet, and Realm persistence boundaries;
- canonical `Structure + LocalPose + Range`, owner-local read cuts, and exact
  ancestor projection;
- revision/CAS and exact OperationID replay for owner-local create, update,
  reparent, and non-cascading delete;
- mixed-kind `ListDirect` pagination with explicit stale-continuation failure;
- local `QueryRegion` with upward depth selecting the full search subtree,
  a separate coordinate Basis, exact sphere intersections, and explicit
  numeric incompleteness;
- optional, explicitly configured retained read snapshots, with current-cut
  verification when retention is disabled;
- trusted in-process dual-Core path reads, direct listings, and spatial
  queries through mount-specified addresses and independently authorized
  owner views;
- exact Realm binding administration and caller-credential lifecycle;
- deterministic local Core–Facet behavior through an explicitly configured
  service actor; and
- black-box, restart, process-exit, convergence, corruption, authorization,
  geometry, and recovery tests over the implementation candidate.

`go run ./cmd/ucupa` executes the deterministic public demonstration once and
prints the five-line success summary shown in the
[prototype demonstration](docs/prototype-demo.md) when all scenarios succeed.
Success requires exit code zero and the complete output; the first PASS line alone
does not prove that the later public scenario completed. The local
probe finds a tangent Entity in a sibling Anchor and reads it at the same cut,
using real Realm grants. The connected scenario uses two temporary durable
Core stores: a probe from the carrier side ascends through the mount, pages
across a subsequent write using retained snapshots, reads a match at the same
connected cut, and reopens the carrier store to verify the latest durable
value. Temporary stores are removed when the demonstration finishes.

Retention is a startup choice, not a delay before writes reach storage.
Omitting `Config.SnapshotRetention` disables retention; supplying a positive
duration enables it. There is no default retention duration. Read handles are
local to the current run and do not survive closing or reopening a Core.
The demonstration is a bounded local program, not a service process.

## P6 real-host Core probe (opt-in)

This two-machine check runs a real Core product on host A and the public Go SDK
probe on host B over the LAN, without a proxy or shared Store. It is separate
from the ordinary verifier. Use disposable test Stores only, two machines with
mutually reachable private IP addresses, and authenticated SSH for the temporary
manifest. That manifest contains a test credential: keep its directory private,
do not print its contents or commit it, and delete the remote copy afterward.

On host A (PowerShell, from this repository root):

```powershell
$env:UCUPA_P6_HOST_IP = '<host A private IPv4>'
$env:UCUPA_P6_MANIFEST_OUT = Join-Path $env:TEMP 'ucupa-p6-core-manifest.json'
$env:UCUPA_P6_HOST_GATE = Join-Path $env:TEMP 'ucupa-p6-core-gate'
go test ./acceptance -run '^TestP6RealHostCoreProduct$' -count=1 -v -timeout 20m
```

Leave that command running after it reports ready. Transfer the manifest to a
private directory on host B using an SSH host key you have independently
verified, then run from host B's repository root:

```sh
chmod 700 /path/to/private-test-dir
chmod 600 /path/to/private-test-dir/manifest.json
UCUPA_P6_REMOTE_MANIFEST=/path/to/private-test-dir/manifest.json \
UCUPA_P6_REMOTE_STATE=/path/to/private-test-dir/receipt.sha256 \
UCUPA_P6_REMOTE_PHASE=write \
go test ./acceptance -run '^TestP6RealHostCoreProbe$' -count=1 -v
```

After the `write` probe passes, create `<host gate>.restart` on host A and wait
for its `Core product restarted` log and `<host gate>.restarted` marker. Run
the same probe on host B with `UCUPA_P6_REMOTE_PHASE=recover`; it checks the
original OperationID receipt against the first phase. Then create
`<host gate>.done` on host A and wait for the host test to pass. Remove host B's
manifest and receipt files. The host test removes its own manifest and gate
files. Neither test starts unless its explicit environment is present.

The probe rejects a wrong TLS key, wrong expected Core, and wrong caller
credential before accepting the correct caller; it then writes and reads an
Entity and verifies its original receipt after Core reopens the same Store.
The probe checks that the target IP is not assigned to its own host, so an
all-on-one-machine run cannot satisfy it. This is one P6 real-host evidence
slice, not proof of cross-host Core–Facet service calls, nested federation,
host power loss, arbitrary networks, or production readiness.

## P6 real-host Core–Facet chain (opt-in)

This two-machine check puts the Core product on host A and the Facet product
on host B over the LAN, each opening its own temporary Core/Facet/Realm Stores
behind pinned TLS. It is separate from the ordinary verifier. Both manifests
contain disposable test credentials: transfer them only over independently
verified authenticated SSH, keep their directories private, do not print or
commit them, and delete the copies afterward. Start both sides roughly
together; each exports its manifest and then waits for the peer's manifest,
so stale peer manifests must not exist when a side starts.

On host A (PowerShell, from this repository root):

```powershell
$env:UCUPA_P6_CHAIN_CORE_IP = '<host A private IPv4>'
$env:UCUPA_P6_CHAIN_CORE_MANIFEST_OUT = Join-Path $env:TEMP 'ucupa-p6-chain-core.json'
$env:UCUPA_P6_CHAIN_FACET_MANIFEST = Join-Path $env:TEMP 'ucupa-p6-chain-facet.json'
go test ./acceptance -run '^TestP6RealHostFacetChainCoreSide$' -count=1 -v -timeout 40m
```

On host B (from its repository root, private test directory):

```sh
chmod 700 /path/to/private-test-dir
UCUPA_P6_CHAIN_FACET_IP=<host B private IPv4> \
UCUPA_P6_CHAIN_FACET_MANIFEST_OUT=/path/to/private-test-dir/facet.json \
UCUPA_P6_CHAIN_CORE_MANIFEST=/path/to/private-test-dir/core.json \
UCUPA_P6_CHAIN_FACET_GATE=/path/to/private-test-dir/gate \
go test ./acceptance -run '^TestP6RealHostFacetChainFacetSide$' -count=1 -v -timeout 40m
```

Copy host A's exported core manifest to host B's `UCUPA_P6_CHAIN_CORE_MANIFEST`
path and host B's exported facet manifest to host A's
`UCUPA_P6_CHAIN_FACET_MANIFEST` path. Both sides then start their products and
verify the peer's entry with wrong-TLS-target, wrong-expected-authority and
wrong-credential probes before host A drives the whole public chain: propose,
cross-host accept, activate, Payload write/read, per-owner denial of an inert
unauthorized caller on each side, and idempotent reverse-contact repair. When
host A logs to stop the Facet product, create `<gate>.stop` on host B; after
host A confirms the explicit unavailable failure and its offline-side contact
repair, create `<gate>.start` on host B to restart the same Facet product on
its original Stores. Once host A verifies the recovered content, original
OperationID receipt and acceptance replay, create `<gate>.done` on host B and
wait for both tests to pass. Each side removes its own manifest and gate files;
also remove the transferred manifest copies. Neither test starts unless its
explicit environment is present, and each side rejects a peer endpoint that is
loopback or assigned to its own host, so one machine cannot satisfy the pair.

This slice proves the two-machine Core–Facet public access chain only: it is
not the whole P6 multi-machine gate, and it covers no cross-Core federation,
Stage 2 routing, host power loss, arbitrary networks or production readiness.

## P6 real-host cross-Core federation (opt-in)

This H3 pair runs Core A and Core C as separate product processes and Stores
on host A, and Core B with its own Realm and Store on host B. Both A→B and
B→C are real private-LAN mount edges. All mount controls and Read, ListDirect
and QueryRegion assertions use public gRPC/TLS. Disposable descendant content
is seeded through the formal owner APIs while each product is stopped, after
its incoming public mount commits; content creation is not an H3 public-RPC
assertion. The products then reopen the original Stores, and subsequent
federation checks use only public gRPC/TLS. Run both tests from the same
frozen source revision; this is not part of the ordinary verifier. The three
manifests carry disposable test credentials. Transfer them only through SSH
whose host key has been independently verified; do not print, commit or leave
them in shared directories. Test directories and transferred manifests must
be private and removed afterward. Start both sides with no stale manifest or
gate files; each exports its own manifest before waiting for the peer copies.

On host A (PowerShell, from this repository root):

```powershell
$env:UCUPA_P6_H3_A_IP = '<host A private IPv4>'
$env:UCUPA_P6_H3_A_MANIFEST_OUT = Join-Path $env:TEMP 'ucupa-p6-h3-a.json'
$env:UCUPA_P6_H3_C_MANIFEST_OUT = Join-Path $env:TEMP 'ucupa-p6-h3-c.json'
$env:UCUPA_P6_H3_B_MANIFEST = Join-Path $env:TEMP 'ucupa-p6-h3-b.json'
go test ./acceptance -run '^TestP6RealHostFederationASide$' -count=1 -v -timeout 60m
```

On host B (from the same source revision in a private test directory):

```sh
chmod 700 /path/to/private-test-dir
UCUPA_P6_H3_B_IP=<host B private IPv4> \
UCUPA_P6_H3_B_MANIFEST_OUT=/path/to/private-test-dir/b.json \
UCUPA_P6_H3_A_MANIFEST=/path/to/private-test-dir/a.json \
UCUPA_P6_H3_C_MANIFEST=/path/to/private-test-dir/c.json \
UCUPA_P6_H3_B_GATE=/path/to/private-test-dir/gate \
go test ./acceptance -run '^TestP6RealHostFederationBSide$' -count=1 -v -timeout 60m
```

Copy A's two exported manifests to B's matching input paths and B's manifest
to A's input path. B's manifest also names its gate prefix. After A reports
that the nested public query has passed, create `<B gate>.stop` on host B;
after A confirms explicit incomplete coverage, create `<B gate>.start`.
After A reports original-Store restart and fresh results, create
`<B gate>.done`; wait for both tests to finish. Do not create these markers
early. Both tests remove their own exported manifests and gates; also remove
transferred copies, logs containing private paths, and disposable directories.

The pair rejects a peer endpoint assigned to the local host, rejects wrong
TLS pin, expected authority and credential, verifies the original caller is
independently denied by B when B has no grant, and checks nested source graph,
closure, pagination, depth, offline PARTIAL coverage, and original-Store
restart without silently renewing old continuation. It is H3 evidence only:
network partition, sudden host power loss, clock skew, independent second
implementations and the overall P6 environment gate remain separate.

## P6 real-host scoped faults (opt-in)

H4 reuses the H3 A→B→C public chain with two additional, separately gated
faults. On B, a test-only TCP byte relay advertises B's address without
terminating TLS; cutting it closes only that product's test connections while
the actual B product listener remains reachable on B. Later the B product is
terminated abruptly (not by its orderly shutdown path) and reopened from its
original Core/Realm Stores. Neither fault changes host routes, firewall or
clock. This is not evidence of a system-wide partition or host power loss.

Use distinct private directories and manifest paths, not stale H3 files.
On host A run:

```powershell
$env:UCUPA_P6_H4_A_IP = '<host A private IPv4>'
$env:UCUPA_P6_H4_A_MANIFEST_OUT = Join-Path $env:TEMP 'ucupa-p6-h4-a.json'
$env:UCUPA_P6_H4_C_MANIFEST_OUT = Join-Path $env:TEMP 'ucupa-p6-h4-c.json'
$env:UCUPA_P6_H4_B_MANIFEST = Join-Path $env:TEMP 'ucupa-p6-h4-b.json'
go test ./acceptance -run '^TestP6RealHostFaultASide$' -count=1 -v -timeout 60m
```

On host B, from the same frozen source in a private directory, run:

```sh
UCUPA_P6_H4_B_IP='<host B private IPv4>' \
UCUPA_P6_H4_B_MANIFEST_OUT=/path/to/private-test-dir/b.json \
UCUPA_P6_H4_A_MANIFEST=/path/to/private-test-dir/a.json \
UCUPA_P6_H4_C_MANIFEST=/path/to/private-test-dir/c.json \
UCUPA_P6_H4_B_GATE=/path/to/private-test-dir/gate \
go test ./acceptance -run '^TestP6RealHostFaultBSide$' -count=1 -v -timeout 60m
```

Exchange manifests only through independently verified SSH, as in H3. After
A reports that the initial nested query passed, create `<B gate>.bridge_cut`;
after A reports explicit incomplete coverage, create `<B gate>.bridge_resume`.
Wait until A reports the restored full query before creating `<B gate>.stop`;
after A reports incomplete coverage from abrupt B product death, create
`<B gate>.start`. Once A finishes original-Store recovery checks, create
`<B gate>.done` and wait for both tests to pass. Do not advance any gate early.
The bridge cut must return within the configured public deadline, not as a
complete empty result. Test-only finite public-state and federation-lease
limits allow the several long-lived pages in this script; production defaults
and product contracts remain unchanged. Remove transferred manifests, logs
with private paths and disposable directories after recording the results.

## P8 fixed-cell executable smoke

The P8 profile is one Core product process and one Facet product process on
loopback TLS, each opening its own durable fact Store and local Realm Store.
The following repeatable test provisions those four Stores through owner APIs
in a fresh temporary directory, starts the real product binaries, and uses
the public SDKs to create/read a Core Entity, establish one Facet relation,
write/read opaque Facet content, repair both contacts while a peer is offline,
archive and hash the four stopped Stores, reject a missing original Store,
cold-revoke a credential, and reopen both owners without substituting backups:

```powershell
go test ./acceptance -run '^TestFacetRelationshipIndependentProducts$' -count=1 -v
```

Its temporary certificates, credential evidence, and Stores are test-only and
are removed when the test exits. This is a runnable baseline that also sends
an orderly console interrupt to each product and reopens its original Store.
It is not a retained deployment or evidence of archive takeover, real
multi-machine networking, or P8 fault-matrix completion. Product configuration
and manual startup requirements are in the [Core host guide](cmd/ucupa-core/README.md)
and [Facet host guide](cmd/ucupa-facet/README.md). The complete P8.2 operator
flow and P8.3 measured fault exercises remain separate delivery gates in the
[development plan](docs/architecture/development-plan.md).

For a retained instance, send each foreground host `Ctrl+C`, wait for both
processes to exit, and confirm their original Store locks are released before
touching private Stores. A force kill is not an orderly-stop step. Restart
only from the same four original paths and the matching writer configuration;
do not point either host at an archive. Provision or change Realm grants and
credentials only while its host is stopped, through the Realm owner API, then reopen the original Store
and establish a new session. A timeout or lost response to a write is not
proof of non-commit: keep its original OperationID and resolve it against the
original Store. These rules also apply before making a cold archive.

For an offline integrity archive, set the four source paths and a **new**
archive directory explicitly. This command copies but never replaces an
original Store or starts an archived owner:

```powershell
$stores = @{
  'core.db' = 'C:/private/ucupa/core.db'
  'core-realm.db' = 'C:/private/ucupa/core-realm.db'
  'facet.db' = 'C:/private/ucupa/facet.db'
  'facet-realm.db' = 'C:/private/ucupa/facet-realm.db'
}
$archive = 'C:/private/ucupa-archives/cell-001' # choose a new path
foreach ($name in $stores.Keys) {
  if (-not (Test-Path -LiteralPath $stores[$name] -PathType Leaf)) { throw "Missing original: $name" }
}
if (Test-Path -LiteralPath $archive) { throw 'Archive path already exists' }
New-Item -ItemType Directory -Path $archive -ErrorAction Stop | Out-Null
foreach ($name in $stores.Keys) {
  $source = $stores[$name]
  $destination = Join-Path $archive $name
  Copy-Item -LiteralPath $source -Destination $destination -ErrorAction Stop
  $before = (Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash
  $after = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash
  if ($before -ne $after) { throw "Archive hash mismatch: $name" }
  "$name $after"
}
```

Record all four printed digests together with the stopped process identities
and time; protect the archive as sensitive Store data. An archive proves only
that those bytes were copied intact. It does not prove that the copy contains
every later confirmed write or that it is safe to serve the original
Authority/Realm from it. If any original
Store is missing or corrupt, keep that owner unavailable; there is no
automatic restore-from-archive path in this profile. The public
handoff tests can be run with
`go test ./acceptance -run '^TestWriter(CoreIndependentProduct|FacetIndependentProducts)$' -count=1 -v`;
the Facet test performs Core then Facet handoff in one two-product cell,
while the Core test independently verifies exact Core result recovery. They
exercise sequential replacement on the original Store, not clone takeover.

For diagnosis, retain each host's startup output and sanitized error line.
`cannot open Core Store`, `cannot open Facet Store`, and Realm identity errors
are stop conditions, not prompts to initialize or restore a different Store.
With a fresh authenticated SDK session, use `Writer().InspectWriter` to inspect
the installed epoch and original handoff decision, Core
`FacetControl().InspectFacet` and Facet `Relationships().InspectAcceptance`
for relationship evidence, and `RecoverOutcome` with the **original**
OperationID and exact request to resolve an uncertain write. An offline peer
must remain explicitly unavailable; the two contact updates in the smoke test
show how to repair each local address without recreating the relationship.

To run the optional fixed-cell load and latency exercise (999 preloaded Core
Entities plus one public creation, exactly one Facet relationship), use:

```powershell
$env:UCUPA_P8_MEASURE = '1'
go test ./acceptance -run '^TestFacetRelationshipIndependentProducts$' -count=1 -v -timeout 45m
Remove-Item Env:UCUPA_P8_MEASURE
```

The test logs 24 sequential samples for each named RPC and the four Store
sizes, plus three orderly-stop and three forced-stop reopen times to the first successful session
establishment for each product and one original OperationID recovery round trip
for each owner after the final reopen. This long-running measurement explicitly uses
10-minute sessions and connections and a larger transient context quota;
these are measurement settings, not the normal profile defaults. It also
checks exact 1 MiB Payload and 256-TypeRef boundaries. This is a loopback observation, not a service-level
objective. The normal verifier runs the same correctness flow without the
expensive load.

P8's two storage faults are separate, opt-in Linux acceptance runs of that
same public two-product flow. Never point either run at an existing Store.
For real ENOSPC, first mount a disposable filesystem of at most 512 MiB at a
path distinct from the system filesystem; it must permit execution because
the fixture builds its product binaries inside `TMPDIR`. Confirm the mount
with `findmnt` and its capacity with `df` before running:

```sh
export UCUPA_P8_FAULT_MOUNT=/path/to/disposable-mount
export TMPDIR="$UCUPA_P8_FAULT_MOUNT"
export GOTMPDIR=/tmp
export UCUPA_P8_DISK_FULL=1
go test ./acceptance -run '^TestFacetRelationshipIndependentProducts$' -count=1 -v -timeout 15m
unset UCUPA_P8_DISK_FULL UCUPA_P8_FAULT_MOUNT TMPDIR GOTMPDIR
```

The test refuses an unmounted/system-disk path or a filesystem over 512 MiB.
It fills only its disposable fixture volume to actual ENOSPC, releases the
filler, reopens the **original fixture Store**, resolves the original
OperationID, and checks exact content and retry behavior. A failed RPC alone
is not proof of an uncommitted write.

For the in-flight crash, set `UCUPA_P8_CRASH_GATE` to a fresh disposable
filename prefix and `UCUPA_P8_CRASH_TRACE` to a fresh trace filename. Start
the test in one terminal, without the disk-full variables:

```sh
export UCUPA_P8_CRASH_GATE=/path/to/disposable-gate-prefix
export UCUPA_P8_CRASH_TRACE=/path/to/disposable-trace-file
go test ./acceptance -run '^TestFacetRelationshipIndependentProducts$' -count=1 -v -timeout 15m
```

Use those same two path values in
the other terminals. The test prints the Facet product PID and waits for
`<gate>.go`. In a second terminal, read `<gate>.ready`, verify that PID is
the disposable Facet test product, and attach `strace`:

```sh
pid=$(cat "$UCUPA_P8_CRASH_GATE.ready")
sudo strace -f -yy -e trace=pwrite64 -e inject=pwrite64:signal=SIGKILL:when=1 \
  -p "$pid" -o "$UCUPA_P8_CRASH_TRACE"
```

After `strace` reports attachment, create `<gate>.go` from a third terminal
with `touch "$UCUPA_P8_CRASH_GATE.go"`.
The test accepts the run only when the trace shows `pwrite64` on the
disposable `facet.db` followed by SIGKILL of that product PID. It then
reopens the original Store, recovers or retries the **same** request and
OperationID, and checks that no partial Payload or duplicate result appears.
Ordinary repository verification does not set either fault-mode variable.
Unset `UCUPA_P8_CRASH_GATE` and `UCUPA_P8_CRASH_TRACE` after the run.

## Current runtime limits

`cmd/ucupa-core` provides Core gRPC/TLS access, mount control and explicitly
configured nested public federation. See its [startup and client instructions](cmd/ucupa-core/README.md).
It opens existing independent durable Core and Realm Stores, authenticates
callers, and adapts the P5 public operations. Optional target-issued read
delegation binds a scope and short-lived token to a configured actor TLS key;
delegation alone does not enable federation: the host's explicit federation
connections and original-requester identity exchange provide that chain.
The local demonstration remains a
separate supported in-process consumer, not a network fallback.

Run the actual product-process/reopen test independently with:

```powershell
go test ./acceptance -run 'TestNetworkIndependentProcessReopen|TestNetworkReadDelegationProductProcess' -count=1 -v
```

This builds and launches the product host and communicates only over loopback
TCP/TLS. No real-host, nested-federation, Facet network, independent-server
interoperability or production readiness claim follows from that test.
The development plan records implementation and validation status separately.

`cmd/ucupa-facet` now provides an independent Facet gRPC host with a separately
authenticated Core reverse-control connection. See its [configuration](cmd/ucupa-facet/README.md).
`go test ./acceptance -run TestFacetAndCoreIndependentProducts -count=1 -v` exercises
both product programs with separate durable owners and Realms. Attachment setup
is provisioned before launch; network attachment-control writes and automatic
federation are not implied by this test.

`cmd/ucupa-realm` serves one existing durable Realm Store as a pinned network
service: typed target check, identity establishment and view authorization over
gRPC/TLS with SPKI-to-owner service admission and bounded run-local handles. The
SDK sends user credentials and protected authorization bodies only after
`CheckTarget` has verified the actual Realm identity on the current connection.
The verification is bound atomically to the TLS transport that answered the
RPC, not to the newest connection at response time. A reply from a replaced
transport cannot qualify the replacement; a fresh check is required.
An optional `foreign` section explicitly configures trusted identity issuers
(pinned signing key, pinned issuer Core connection and online liveness over the
existing P6 proof check); without it foreign establishment fails closed as
unavailable. See its
[bootstrap and configuration](cmd/ucupa-realm/README.md). The Realm kernel
remains the only identity and authorization fact owner; handles are revalidated
online on every operation and never persist a reusable Allow.

```powershell
go test ./cmd/ucupa-realm -count=1 -v
```

builds the real product binary and verifies bootstrap, reopen rejection, the
SDK chain over real TCP/TLS, forced-kill restart with Realm identity preserved
and prior-run handles invalid, a fresh caller working afterwards, and the
foreign port staying fail-closed without configuration. This
loopback test is not a real multiple-host or production deployment claim.
Core consumers now authorize through this service via the Core product's
network Realm profile (S2P3b, see below); Facet consumers use their own
persisted contact under the same explicit network profile (S2P3c).

```powershell
go test ./acceptance -run TestRealmProductForeignIdentityAcrossProcesses -count=1 -v
```

builds one ucupa-core issuer product and one ucupa-realm receiver product and
verifies the cross-process foreign identity chain: a proof bound to the admitted
caller's owner establishes exactly the issuer's subject with only the seeded
authorization, the online liveness check runs over the receiver's own pinned
issuer connection, a misbound owner is rejected, an offline issuer is
unavailable, and a restarted issuer rejects the old proof while a fresh proof
works again.

`cmd/ucupa-core` can assemble its Realm contact through the pinned network
service instead of the local Realm kernel: set `realm_profile` to `network`
and provide `realm_endpoints`, each pinning the Realm's `next_hop`, `ca_file`,
`spki_sha256` and this owner's mTLS service certificate/private key. The
profile is mutually exclusive with `realm_store`/`realm_next_hop`, endpoints
reuse the host's RPC deadline, handshake timeout and message limits, and
opening the Store still proves the persisted contact online before serving.
See the [Core configuration](cmd/ucupa-core/README.md).

```powershell
go test ./acceptance -run 'TestNetworkRealmContactProduct(Chain|Misbinding|IdentityChain|Maintenance|LockProof)$' -count=1 -v
```

builds one ucupa-realm product and one ucupa-core product and drives the Core
consumer chain over real gRPC/TLS: authorization and session derivation
through the Core's mTLS service identity, Core restart recovery through the
persisted contact, stale run-local handles refusing across a Realm restart,
offline fail-closure, online revocation and restoration, a client deadline
without commit plus same-id retry, wrong SPKI/Realm pins refusing startup, an
unadmitted service identity serving nothing, the P6 source-side identity
chain online, the original contact-maintenance matrix over the same products,
and a byte-level hold barrier proving that a write waiting on its online
authorization never holds the fact lock while a lock-needing Close succeeds.
These loopback tests do not establish production or real multiple-host claims.

`go test ./acceptance -run TestFacetRelationshipIndependentProducts -count=1 -v`
starts with no attachment. Both real product processes perform proposal,
acceptance, activation, content read/write, offline contact repair in both
directions, restart/replay, and detach entirely through authenticated gRPC.
This verifies the relationship network chain, not automatic public federation.

`go test ./acceptance -run '^TestNetworkFederationNestedProductsRestart$' -count=1 -v`
starts three independent Core product processes and exercises nested public
query, pagination, ListDirect, same-cut Read, prepared observation, source
interruption and durable restart. Old source leases are not renewed silently;
explicit release returns resources for a fresh query. Address selection follows
the stored relation and configured peer trust, never a separate Ref directory.

`go test ./acceptance -run '^TestNetworkIndependentImplementationCorpus$' -count=1 -v`
runs the same 46-case public corpus against Go products and an independent
Python reference implementation. Its [declared scope and pinned setup](acceptance/testdata/interop/README.md)
are part of the evidence: it is not a second production federation/storage implementation.

The complete verification command remains `go run ./cmd/verify`. Its ordinary
and race suites have an explicit finite 20-minute per-package timeout: the
product acceptance package builds and starts many real executables sequentially.
This runner limit does not change product RPC deadlines, session lifetimes,
individual fixture bounds or test assertions, and neither suite is skipped.
Real multiple-host
networking, certificate deployment and host failures remain unverified; these
loopback tests do not establish production readiness or complete the real-host gate.

### S2P3d finite Realm product combinations

```powershell
go test ./acceptance -run '^(TestRealmCombination.*|TestNetworkRealmCommittedResponseLossRecovery|TestNetworkFederationRealmProductsRestart)$' -count=1 -v
```

These four tests reuse existing public product scenarios. Core and Facet use
either two independent Realm products/Stores or one explicitly shared Realm
product/Store with two distinct admitted owner certificates. They verify
relationship/content operations, contact repair, outage/restart, independent
permission denial and original-result recovery after permission restoration.
Wrong Realm identity and pin refuse Facet startup. Grant changes are explicit
cold maintenance of the original stopped Realm Store, not new remote admin RPCs.

A byte-only return-traffic barrier proves a Core mutation committed before its
response is lost, then recovers the exact original receipt after original-Store
restart. Current authorization still controls disclosure of that receipt.
The two-Core mounted query/pagination/read/observation scenario additionally
runs both authorization sources as independent Realm products. P6 signed relay
Actor and receiving service owner remain distinct; actor substitution, missing
actor, wrong-owner handle reuse and challenge reuse are rejected. This fixture
explicitly supplies issuer trust and uses the receiving Core certificate for
its Realm's issuer-liveness client; this is not automatic certificate discovery.

The evidence is Windows loopback with separate real product processes and TLS.
It does not claim real multi-host Realm networking, production readiness,
global atomic authorization or exhaustive topology/fault combinations.
