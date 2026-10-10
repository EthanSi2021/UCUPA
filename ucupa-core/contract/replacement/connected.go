// Package replacement defines the designated Facet contacts of one element
// namespace replacement. The controlling Core is the sole orchestrator: these
// local ports carry only peer evidence, never caller authorization, and the
// adapters establish the fixed Facet service identity themselves. Manifest
// bytes travel opaquely in the exact journal-retained form; the Core never
// re-encodes or parses them.
package replacement

import (
	"context"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

// FacetReplacementSourceState discloses the source Facet's durable replacement
// evidence. When Fenced holds, FinalCut, ManifestDigest and the exact
// manifest journal bytes are the source's immutable fence facts.
type FacetReplacementSourceState struct {
	Found          bool
	Fenced         bool
	Refused        bool
	Settled        bool
	FinalCut       uint64
	ManifestDigest [32]byte
	Manifest       []byte
}

// FacetReplacementFenced is the source Facet's durable fence result. Key binds
// the fenced acceptance; Manifest is the exact journal byte form.
type FacetReplacementFenced struct {
	Key            values.FacetAcceptanceKey
	FinalCut       uint64
	ManifestDigest [32]byte
	Manifest       []byte
}

// FacetReplacementEntry is one exact live payload of a fenced namespace,
// relayed entry by entry. Payload bytes stay opaque to the Core.
type FacetReplacementEntry struct {
	Type    values.FacetTypeRef
	Payload []byte
}

// FacetReplacementLiveEntry is one live manifest key the Core must relay
// before the target's acceptance; it never carries payload bytes.
type FacetReplacementLiveEntry struct {
	Type   values.FacetTypeRef
	Length uint64
}

// FacetReplacementImportState discloses the target Facet's durable import
// evidence. Live is the current staged key set for the relay loop.
type FacetReplacementImportState struct {
	Accepted  bool
	Installed bool
	Digest    [32]byte
	FinalCut  uint64
	Live      []FacetReplacementLiveEntry
}

// FacetReplacementAccepted is the target Facet's durable acceptance result.
type FacetReplacementAccepted struct {
	Key values.FacetAcceptanceKey
}

// FacetSourcePeer is the controlling Core's port to the source Facet. The
// source's own durable refusal is discovered through Check, never commanded.
type FacetSourcePeer interface {
	CheckSourceExport(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID) (FacetReplacementSourceState, error)
	FenceSource(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID, values.FactAuthorityRef) (FacetReplacementFenced, error)
	ReadSourceEntry(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID, values.FacetTypeRef) (FacetReplacementEntry, error)
	SettleSource(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID, values.FactAuthorityRef) error
}

// FacetTargetPeer is the controlling Core's port to the accepting target
// Facet. CoreAddress is the controlling Core's retained service address the
// target records with its acceptance for later authoritative reads.
type FacetTargetPeer interface {
	CheckTargetImport(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID, []byte, values.NextHop) (FacetReplacementImportState, error)
	StageTargetEntry(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID, values.FacetTypeRef, []byte) error
	AcceptTarget(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID, values.NextHop) (FacetReplacementAccepted, error)
	InstallTarget(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID, values.FactAuthorityRef, uint64) error
	ReadTargetImport(context.Context, values.NextHop, values.FacetAcceptanceKey, contract.OperationID) (FacetReplacementImportState, error)
}

// FacetPeers carries the two designated contacts of one recovery. Adapters
// must verify the expected Facet identity at the supplied address and keep a
// fixed service identity independent of the requesting caller.
type FacetPeers struct {
	Source FacetSourcePeer
	Target FacetTargetPeer
}
