package contract

import "ucupa/shared/values"

// NamespaceManifestEntry is one ordered entry of a complete element namespace
// manifest at the source final cut. Live entries carry the exact payload facts
// and the record revision; deleted entries carry the tombstone revision.
// Payloads never travel inside the manifest.
type NamespaceManifestEntry struct {
	Type     values.FacetTypeRef
	Live     bool
	Length   uint64
	Digest   [32]byte
	Revision Revision
	Deleted  Revision
}

// NamespaceManifest is the complete ordered TypeRef key set of one element.
// Entries may be empty: a present manifest with a final cut represents a
// complete empty namespace, distinct from an absent manifest or unknown source
// state. The digest covers the ordered entries (including the empty sequence),
// entry kinds, types, lengths, payload digests and tombstones, so it proves the
// exact namespace without carrying the bytes.
type NamespaceManifest struct {
	Element  values.ElementID
	Source   values.FactAuthorityRef
	FinalCut uint64
	Entries  []NamespaceManifestEntry
	Digest   [32]byte
}

// NamespaceFenced is the durable source fence result of one replacement.
type NamespaceFenced struct {
	Key      values.FacetAcceptanceKey
	Manifest NamespaceManifest
}

// NamespaceSourceState discloses the source-side replacement evidence of one
// element to the controlling Core for inspection and bounded recovery.
type NamespaceSourceState struct {
	Found    bool
	Key      values.FacetAcceptanceKey
	Fenced   bool
	Refused  bool
	Settled  bool
	Manifest *NamespaceManifest
}

// NamespaceAccepted is the durable target acceptance result of one replacement.
type NamespaceAccepted struct {
	Key values.FacetAcceptanceKey
}

// NamespaceSourceEntry is one exact live payload of a fenced namespace,
// disclosed entry by entry for the controlling Core to stage downstream.
type NamespaceSourceEntry struct {
	Type     values.FacetTypeRef
	Payload  []byte
	Revision Revision
}

// NamespaceLiveEntry is one live manifest key disclosed to the controlling
// Core for its entry-by-entry relay. It names a record to move, never the
// payload bytes themselves.
type NamespaceLiveEntry struct {
	Type   values.FacetTypeRef
	Length uint64
}

// NamespaceImportState discloses the target-side replacement evidence of one
// element to the controlling Core for inspection and bounded recovery. Live
// is the current staged key set the Core must relay before acceptance; it is
// empty once the namespace has been durably accepted.
type NamespaceImportState struct {
	Accepted  bool
	Installed bool
	Key       values.FacetAcceptanceKey
	Digest    [32]byte
	FinalCut  uint64
	Live      []NamespaceLiveEntry
}
