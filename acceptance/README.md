# UCUPA acceptance

This directory is the black-box regression witness for the accepted G1 Core
implementation and the accepted Facet and Realm public surfaces. Authority
classification is maintained only by the
[architecture index](../docs/architecture/README.md); tests provide evidence,
not authority. This is a test package, not a reusable product library, and does
not inspect owner-private state.

The G1 acceptance scenario exercises one Realm-authorized local Core spatial
truth set: root, Anchor, and Entity creation; bounded Range; projection to an
ancestor; stable Element identity; Facet attachment through an explicitly
configured service actor with exact Core read permission; deletion-driven
Facet invisibility; and explicit orphan reconciliation.

Separate gates prove Realm-owned permission-binding administration and online
caller authentication while consuming the same owner public APIs. Composition
only wires those owners together and adds no domain truth.

The historical Concept v0 remains predecessor evidence;
it is not the current contract. These tests do not claim mount, route, referral,
federation, discovery, transport, multi-owner cuts, or production operation.
