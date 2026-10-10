package composition

import (
	"context"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
)

// ForReplacement fixes the Core service identity of the local namespace
// replacement peers. Client requests can never replace the caller: both peers
// authorize only the actor established here, at the exact bound addresses.
func (c *LocalFacetConnections) ForReplacement(caller values.CallerContext) (replacement.FacetPeers, error) {
	if c == nil || !caller.Subject.Valid() || caller.Proof == ([32]byte{}) {
		return replacement.FacetPeers{}, corecontract.ErrInvalid
	}
	return replacement.FacetPeers{
		Source: facetSourcePeer{connections: c, caller: caller},
		Target: facetTargetPeer{connections: c, caller: caller},
	}, nil
}

type facetSourcePeer struct {
	connections *LocalFacetConnections
	caller      values.CallerContext
}

type facetTargetPeer struct {
	connections *LocalFacetConnections
	caller      values.CallerContext
}

// replacementPeer connects the exact bound Facet identity the acceptance key
// names; no Ref lookup or fallback exists. Any failure to reach that identity
// at the supplied address is reported as temporary Core-side unavailability.
func replacementPeer(c *LocalFacetConnections, ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (*facet.Instance, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, corecontract.ErrUnavailable
	}
	peer, err := c.Connect(address, key.Facet)
	if err != nil {
		return nil, corecontract.ErrUnavailable
	}
	return peer, nil
}

// replacementManifestBytes is the exact journal form of one fenced manifest;
// an unfencodable manifest is a durable contradiction, never a retry.
func replacementManifestBytes(m facetcontract.NamespaceManifest) ([]byte, error) {
	manifest, err := facetcontract.EncodeNamespaceManifest(m)
	if err != nil {
		return nil, corecontract.ErrConflict
	}
	return manifest, nil
}

func (s facetSourcePeer) CheckSourceExport(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID) (replacement.FacetReplacementSourceState, error) {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementSourceState{}, err
	}
	state, err := peer.Namespace.CheckNamespaceExport(ctx, s.caller, key, facetcontract.OperationID(op))
	if err != nil || ctx.Err() != nil {
		return replacement.FacetReplacementSourceState{}, corecontract.ErrUnavailable
	}
	out := replacement.FacetReplacementSourceState{Found: state.Found, Fenced: state.Fenced, Refused: state.Refused, Settled: state.Settled}
	if state.Manifest != nil {
		manifest, err := replacementManifestBytes(*state.Manifest)
		if err != nil {
			return replacement.FacetReplacementSourceState{}, err
		}
		out.FinalCut = state.Manifest.FinalCut
		out.ManifestDigest = state.Manifest.Digest
		out.Manifest = manifest
	}
	return out, nil
}

func (s facetSourcePeer) FenceSource(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, target values.FactAuthorityRef) (replacement.FacetReplacementFenced, error) {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementFenced{}, err
	}
	fenced, err := peer.Namespace.FenceNamespace(ctx, s.caller, key, facetcontract.OperationID(op), target)
	if err != nil || ctx.Err() != nil {
		return replacement.FacetReplacementFenced{}, corecontract.ErrUnavailable
	}
	manifest, err := replacementManifestBytes(fenced.Manifest)
	if err != nil {
		return replacement.FacetReplacementFenced{}, err
	}
	return replacement.FacetReplacementFenced{Key: fenced.Key, FinalCut: fenced.Manifest.FinalCut, ManifestDigest: fenced.Manifest.Digest, Manifest: manifest}, nil
}

func (s facetSourcePeer) ReadSourceEntry(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, typ values.FacetTypeRef) (replacement.FacetReplacementEntry, error) {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementEntry{}, err
	}
	entry, err := peer.Namespace.ReadNamespaceSource(ctx, s.caller, key, facetcontract.OperationID(op), typ)
	if err != nil || ctx.Err() != nil {
		return replacement.FacetReplacementEntry{}, corecontract.ErrUnavailable
	}
	return replacement.FacetReplacementEntry{Type: entry.Type, Payload: entry.Payload}, nil
}

func (s facetSourcePeer) SettleSource(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, target values.FactAuthorityRef) error {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return err
	}
	if err = peer.Namespace.SettleNamespace(ctx, s.caller, key, facetcontract.OperationID(op), target); err != nil || ctx.Err() != nil {
		return corecontract.ErrUnavailable
	}
	return nil
}

// replacementImportState projects the target's import evidence; it never
// carries payload bytes.
func replacementImportState(state facetcontract.NamespaceImportState) replacement.FacetReplacementImportState {
	live := make([]replacement.FacetReplacementLiveEntry, len(state.Live))
	for i, e := range state.Live {
		live[i] = replacement.FacetReplacementLiveEntry{Type: e.Type, Length: e.Length}
	}
	return replacement.FacetReplacementImportState{Accepted: state.Accepted, Installed: state.Installed, Digest: state.Digest, FinalCut: state.FinalCut, Live: live}
}

func (s facetTargetPeer) CheckTargetImport(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, manifest []byte, coreAddress values.NextHop) (replacement.FacetReplacementImportState, error) {
	decoded, err := facetcontract.DecodeNamespaceManifest(manifest)
	if err != nil {
		return replacement.FacetReplacementImportState{}, corecontract.ErrConflict
	}
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementImportState{}, err
	}
	state, err := peer.Namespace.CheckNamespaceImport(ctx, s.caller, key, facetcontract.OperationID(op), decoded, coreAddress)
	if err != nil || ctx.Err() != nil {
		return replacement.FacetReplacementImportState{}, corecontract.ErrUnavailable
	}
	return replacementImportState(state), nil
}

func (s facetTargetPeer) StageTargetEntry(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, typ values.FacetTypeRef, payload []byte) error {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return err
	}
	if err = peer.Namespace.StageNamespaceEntry(ctx, s.caller, key, facetcontract.OperationID(op), typ, payload); err != nil || ctx.Err() != nil {
		return corecontract.ErrUnavailable
	}
	return nil
}

func (s facetTargetPeer) AcceptTarget(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, coreAddress values.NextHop) (replacement.FacetReplacementAccepted, error) {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementAccepted{}, err
	}
	accepted, err := peer.Namespace.AcceptNamespace(ctx, s.caller, key, facetcontract.OperationID(op), coreAddress)
	if err != nil || ctx.Err() != nil {
		return replacement.FacetReplacementAccepted{}, corecontract.ErrUnavailable
	}
	return replacement.FacetReplacementAccepted{Key: accepted.Key}, nil
}

func (s facetTargetPeer) InstallTarget(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, source values.FactAuthorityRef, finalCut uint64) error {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return err
	}
	if err = peer.Namespace.InstallNamespace(ctx, s.caller, key, facetcontract.OperationID(op), source, finalCut); err != nil || ctx.Err() != nil {
		return corecontract.ErrUnavailable
	}
	return nil
}

func (s facetTargetPeer) ReadTargetImport(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID) (replacement.FacetReplacementImportState, error) {
	peer, err := replacementPeer(s.connections, ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementImportState{}, err
	}
	state, err := peer.Namespace.ReadNamespaceImport(ctx, s.caller, key, facetcontract.OperationID(op))
	if err != nil || ctx.Err() != nil {
		return replacement.FacetReplacementImportState{}, corecontract.ErrUnavailable
	}
	return replacementImportState(state), nil
}
