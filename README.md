# UCUPA

**Spatial infrastructure for the united cyber universe**

UCUPA stands for **The system of United Cyber Universe Position and Anchor**.

UCUPA organizes a world as a tree of local coordinate frames. It is designed for worlds that
grow in extent and local detail, while different operators store and maintain their own parts.
The tree gives those parts a common spatial structure without requiring one central world database.

## Spatial model

- Every Element is an **Anchor** or an **Entity**. Anchors contain other anchors and entities;
  entities are leaves. Each non-root element has one parent anchor and a parent-relative pose.
- Position and orientation compose along the parent chain. Moving an anchor changes its
  descendants’ derived poses without rewriting their local records; element identities stay stable.
- Non-root anchors have finite spherical ranges covering their descendants. Queries use these
  ranges to discover intersecting elements within a selected part of the tree.
- A mount distributes the same tree across Core owners. The parent Core owns the mount anchor’s
  record; the accepting Core owns the elements below that boundary.

Spatial inheritance does not grant access. Ownership, permissions, and service addresses are
separate from parent–child relationships. Separate roots do not automatically share coordinates.

## Core, Facet, and Realm

| Module | Responsibility |
|---|---|
| **Core** | Element identities, existence, parents, local poses, ranges, and spatial relationships |
| **Facet** | Optional opaque content keyed by Element + TypeRef; applications define and interpret its format |
| **Realm** | Caller identity and explicit permissions for actions on targets |

Applications handle rendering, simulation, and user interfaces. UCUPA provides the spatial facts,
content access, and authorization they consume.

## Use UCUPA

The public interface supports five behaviors. These are categories of operations, not five
mandatory calls or a fixed application pipeline.

| Behavior | What an application does |
|---|---|
| **Enter** | Use a space reference and authenticated identity to establish a temporary, bounded handle |
| **Discover** | Follow paths, list children, or query a region; inspect result scope and completeness |
| **Read** | Read spatial records from Core and optional content from the referenced Facet |
| **Observe** | Follow committed changes from a reading point; refresh when a continuity gap is reported |
| **Propose** | Request a change with preconditions and an operation ID; handle its owner’s committed result or rejection |

The Go SDKs use typed gRPC services over TLS. Connections use specified addresses and verify the
expected service identity. A bare element ID does not trigger a global address lookup.

## Run UCUPA locally

From a source checkout, open the repository root containing `go.mod`. Use Go 1.25 or later,
as required by that file:

```text
go version
go run ./cmd/ucupa
```

On the first run, Go downloads the dependencies pinned by `go.mod` and `go.sum`, unless they are
already cached. The example needs no separate database or service. It uses temporary local
stores to demonstrate cross-branch discovery, two-Core reads, snapshot pagination, Facet content,
and public observation.

The [demo guide](docs/prototype-demo.md) contains the complete expected output and explains what
each scenario proves. Success requires all five output lines and exit code 0.

## Project status

UCUPA is currently an **engineering prototype**. Its Baseline has been completed and accepted
within the declared scope. The [snapshot status](docs/architecture/development-plan.md#1-当前在哪里)
summarizes its supported profiles and validation limits. The
[public source repository](https://github.com/EthanSi2021/UCUPA) uses independent Git history;
the private development history is not included.

Local frames use finite numeric profiles; they do not imply unlimited capacity or precision.
Network and transfer support covers defined profiles, not arbitrary topologies. The Baseline
does not establish automatic high availability, production-scale performance, or production readiness.
The local example is not a network deployment or a benchmark.

## Documentation

- [Demo guide](docs/prototype-demo.md): first run, expected results, and recording guidance.
- [Run guide](RUN.md): product hosts, tool requirements, and verification commands.
- [Platform overview](PLATFORM.md): a non-normative projection of the model and ownership boundaries.
- [Architecture index](docs/architecture/README.md): authoritative specifications and current contracts.
- [Development plan](docs/architecture/development-plan.md): current work and completion gates.
- [Engineering rules](ENGINEERING.md): directory organization, dependencies, and file boundaries.
- [Agent instructions](AGENTS.md): repository-wide working and delivery rules.

This README is an introduction, not a specification. The architecture index identifies the sources
that define UCUPA semantics. This page does not establish a separate contract or implementation plan.

## Verification

```text
go run ./cmd/verify
```

The full verifier covers engineering rules, fixed generated code, tests, race checks, static checks,
dependencies, the local example, and whitespace. See the [tool requirements](RUN.md#verify).
Running the example alone is not equivalent to passing the full verifier.

## License

UCUPA is licensed under the [Apache License, Version 2.0](LICENSE).
Third-party components remain subject to their respective licenses and notices. See
[third-party notices](NOTICE) for the root module's declared dependencies and their license texts.

## Security

GitHub private vulnerability reporting is enabled for this repository. See the
[security policy](SECURITY.md) for the reporting entry and what to include in a report. Do not
put vulnerability details, credentials, private keys, or real deployment configuration in public
issues, screenshots, or recordings.
