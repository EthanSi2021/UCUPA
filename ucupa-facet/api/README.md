# Facet public representations

## Local public profile

The entry is `facet.Namespace.PublicCall(ctx, caller, canonicalJSON)`, where
`ucupa-facet/instance` creates or opens `facet`. Establish the caller separately;
request fields cannot assert identity.

Obtain the key and NextHop from Core's ACTIVE attachment, then connect to that address and
verify the expected Facet. Public `access` is exactly
`{key:{core,facet,element,proposal},nextHop}`; proposal is Core's opaque token.
The owner's private AcceptanceKey byte encoding is not sent directly to clients.
Bare Ref does not resolve an address.

- [Request](request.schema.json), [response](response.schema.json), [types](types.schema.json).
- [Conformance cases](conformance.json).
- The [public contract](../../docs/architecture/public-access-and-observation.md) defines
  timing, encoding, and failure semantics.

`get`/`list-types` may set `prepareObserve:true`. Facet prepares its local queue at F0,
then obtains C0 control preparation from Core with the fixed service identity.
It rechecks F0/contact/authorization before delivery. Server control tokens never reach clients.
`begin-observe`/`poll` check both sides' continuity. Core detach/delete/contact changes
end the old combination even without a Facet write.
`release-observe` best-effort releases peer preparation; when disconnected, the peer's
independent deadline reclaims it. No network wait holds the Facet lock.

`put` expected is `{absent:true}` or `{revision:token}`; `delete` supplies the expected token
directly. `recover-outcome` supplies access, OperationID, and the original canonical request.
It reads only the caller's receipt, reexecutes no write, and adds no Core-online or external
type prerequisite.

## Typed network representation

`facet.proto` defines eight content RPCs: Get, ListTypes, Put, Delete, RecoverOutcome,
BeginObserve, PollChanges, and ReleaseObserve. Public AttachmentKey uses the same message
as Core's attachment reference in `shared/api/network.proto`, without local
AttachmentRevision/CoreCutEvidence encoding. `go run ./cmd/protogen` uses pinned repository
tools to generate Go bindings; the full gate regenerates and compares them.

`transport/grpc` adapts typed messages to the original PublicCall and defensively checks
responses. The host's authenticated session supplies caller identity; the original kernel
owns authorization, acceptance, exact retries, F0, and Core rechecks.
`transport/grpc.New` provides bounded TLS hosting and short-lived sessions.
`sdk/go` provides specified targets, certificate binding, sessions, and strict result checks.
Real TCP/TLS tests, not handlers alone, support the network evidence.

The independent `cmd/ucupa-facet` host, fixed-service Core checks, and network relationship
control writes are implemented. `acceptance/network_facet_relationship_product_test.go`
covers establishment from an empty relationship, bidirectional contact repair, restart,
and detach. See the [host guide](../../cmd/ucupa-facet/README.md) for configuration and limits.

Protocol round trips and actual-owner adaptation are tested in
`transport/grpc/representation_test.go` and `acceptance/network_facet_adapter_test.go`;
the latter is not TLS/independent-process evidence.
`acceptance/network_facet_test.go` covers actual TLS, session capacity, credential revocation,
wrong targets, and transport rebuilding. Its Core checks use a local fixed-service adapter,
not a cross-process reverse connection.
`acceptance/network_facet_reverse_test.go` covers actual network reverse checks/observation;
`acceptance/network_facet_process_test.go` covers independent products, separate Realms,
durable recovery, and service-session reestablishment.

## Types and opaque content

TypeRef is any 32-byte value, publicly encoded in 43 characters, not an address, name, or version.
Since P7.3a, Facet retrieves, parses, and validates no type artifacts.
Put stores bounded unchanged opaque Payload at `(Element, TypeRef)`, without schema validation.
Payload is at most 1 MiB; each Element has at most 256 current types. Content and receipt share
one transaction. Get/ListTypes return current-key content/current types only.

## Configuration and evidence

New/CreateDurable/OpenDurable explicitly receive Config. Omitted PublicAccessPolicy disables
the public surface. Both TTLs require explicit durations; other finite defaults are in the
contract. Each context costs 2 KiB, each notification group 512 logical bytes.
Execution additionally reserves `8×request bytes + 4 MiB`,
including decoding and maximum returned content batches, not payload alone.
Different Facets and Core-service dependencies are accounted and expired independently;
no global cut is assembled.

Run the real chain with `go run ./cmd/ucupa`.
[Public black-box tests](../../acceptance/public_facet_test.go) and
[concurrency/restart counterexamples](../../acceptance/public_facet_failure_test.go)
cover access, service/client isolation, and capacity. Schema conformance alone is not a
behavioral or security pass.
