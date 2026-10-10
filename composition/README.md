# UCUPA Composition

Composition is the non-authoritative in-memory harness that connects the Core
and Facet truth layers to Realm authorization for black-box scenarios.
It is not a product module and owns no Core/Facet semantic truth, Realm
identity/permission state, delegation, or durable route fact.

`Cluster` is only a shallow assembly root. It accepts an explicitly constructed
Realm and passes it directly to the consumer-owned Core and Facet authorization ports.
P4 requires explicit Core and Facet NextHop addresses, a fixed service actor for each
direction, and a type resolver in Seed. LocalFacetCoreConnections projects the separately
authorized current proposal/attachment read; it discloses no spatial record.
LocalFacetConnections connects at the exact supplied address and checks the stable Facet
identity. Its ForCore adapter reads acceptance using the configured Core service actor.
Composition does not select owners, mint grants or copy contacts. The caller grants exact
permissions after reading each newly created owner's identity, then explicitly proposes,
accepts and activates attachment. Core-only assembly remains possible without Facet.

Composition exposes the resulting owner modules without mirroring their
operations or interpreting their domain facts.

The ordinary local deterministic fixtures seed registered Principals and exact
`PermissionBinding` rows, then inject already established `PrincipalRef`
values. Composition proves wiring and exact owner authorization; it does not
authenticate callers or establish issuer, key, rotation, or revocation trust.

P5 public fixtures and the public demo explicitly configure both owners with
`PublicAccessPolicy` and obtain sealed `CallerContext` values from the trusted
Realm bootstrap. `PublicCall` adapters pass canonical messages to exactly the
specified address and verify the expected owner. Client identity is independent
of message fields. `ForFacetCaller` binds the fixed Facet service caller; its
minimal Core control evidence/preparation stays inside Facet and the adapter,
never in client responses. Facet owns its F0/Core-C0 combination and recovery;
Composition neither owns streams nor joins cuts. Remote authentication remains
outside this trusted-local profile.

The Phase 1 local connection adapter binds specified `NextHop` addresses to
restricted authorized Core read ports. It checks the expected Core identity,
never searches by Ref or exposes a raw Store, and calls peers outside its map
lock. These trusted in-process bindings are not remote authentication.

After the local Go responsibility split, bind a `CoreEndpoint` explicitly:
`CoreEndpoint{Read: c.Elements(), Public: c}` supplies the existing local read
and public-envelope entries. A read-only binding leaves `Public` nil; a public
call then remains unavailable rather than gaining a synthesized capability.
The two provided entries must name the same owner. One map record captures and
replaces the pair atomically; callbacks run outside its lock and identity is
rechecked before disclosing results. This is assembly, not a Core forwarding API.

Core-side Facet relationships use `LocalFacetCoreEndpoint` in the same way:
`LocalFacetCoreEndpoint{Read: c.Facets(), Control: c}` explicitly supplies the
proposal/attachment read entry and optional current-Core observation entry.
Ordinary relationship reads need only Read. A missing Control remains unavailable
to service observation; it is not synthesized from the read port. The pair has
one expected identity, is captured/rebound together, and is checked after each
control call. A reopened Core must supply its new live entries, not old handles.

P2 adds `LocalMountConnections`: explicitly bind specified addresses to
`MountPeerPort`, obtain a source-bound `MountTransport` with `TransportFor`,
and configure that transport on the source Core's actual `Mounts()` entry.
Bindings use `c.Mounts().MountPeer()`; the composition layer does not proxy Mount
operations or calculate command commitments. Each Prepare, Finish or
ReadDecision captures only the specified port, checks its expected identity
before and after calling it, and supplies the source identity from trusted
assembly rather than a caller-controlled message field. Rebinding affects
subsequent calls, not the captured port of an in-flight call. There is no
fallback address scan or Ref directory. Durable contact facts and all mount
decisions remain owned by Core; a reopened Core must be explicitly rewired.

P3 adds `LocalConnectedConnections` for the same specified-address rule on
connected reads. Bind `c.Connected().ConnectedPeer()` and configure the transport
on `c.Connected()`. `TransportFor` captures the source Core identity; the only
peer surface is `contract/connected.PeerPort`, with opaque Request/Reply values.
Each call captures one address binding,
checks the expected target before and after invoking that port, and carries
the trusted source identity separately from the caller's request. Core owns
query authorization, scope, cuts, snapshots, and completeness. The adapter
does not decode or retain those facts, and it does not retry through other
addresses. A failed or cancelled exchange must still allow the Core-owned
temporary read resources to be released.

Composition has no Cluster directory, copied delegation/context records, CRUD
facade, Store, network transport, host, server, background work, or domain-state map.
The adapters are restricted local connections, not remote routing or a
second implementation of the combined-query or multi-owner-cut rules. P3 uses
the connected read adapter for its bounded dual-Core scenarios; network
transport and remote authentication remain outside that local implementation.
This does not retain a predecessor compatibility layer or postpone an
independent Ref route system.


P4 persists neither address map: the owners persist contacts, and the host only rebinds
local transport after restart. One Core can attach different Elements to different Facets;
one Facet can accept different Core-owned Elements. No bare Ref lookup or fallback exists.
The local demo obtains its Facet connection from the same-cut ACTIVE attachment returned
by Core, then Facet performs a fresh authorized reverse read with independent cut evidence.
