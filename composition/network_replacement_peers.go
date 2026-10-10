package composition

import (
	"context"
	"encoding/binary"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/contract/replacement"
	pb "ucupa/ucupa-facet/bindings/go/protobuf"
	service "ucupa/ucupa-facet/sdk/go"
)

// ForReplacement fixes the Core service identity of the network namespace
// replacement peers. Both contacts authorize only this Core's service
// principal, established at the exact bound address; content callers never
// reach these ports, and the peers never adopt another principal.
func (s *NetworkCoreFacetSource) ForReplacement() replacement.FacetPeers {
	return replacement.FacetPeers{Source: networkFacetSourcePeer{s}, Target: networkFacetTargetPeer{s}}
}

type networkFacetSourcePeer struct{ source *NetworkCoreFacetSource }
type networkFacetTargetPeer struct{ source *NetworkCoreFacetSource }

// replacementClient resolves the exact bound Facet identity the acceptance key
// names and maintains the fixed service session. No Ref lookup or fallback
// exists; any failure to reach that identity is temporary Core-side
// unavailability.
func (s *NetworkCoreFacetSource) replacementClient(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey) (*service.Client, error) {
	if s == nil || ctx == nil || ctx.Err() != nil || !key.Valid() || key.Core != s.core {
		return nil, corecontract.ErrUnavailable
	}
	e := s.peers[address]
	if e.Client == nil || e.Facet != key.Facet {
		return nil, corecontract.ErrUnavailable
	}
	if e.Credentials != nil {
		audience, id, secret, err := e.Credentials.Evidence()
		if err != nil {
			return nil, corecontract.ErrUnavailable
		}
		err = e.Client.EnsureSession(ctx, audience, id, secret)
		clear(secret[:])
		if err != nil {
			return nil, corecontract.ErrUnavailable
		}
	}
	return e.Client, nil
}

func networkFacetServiceKey(key values.FacetAcceptanceKey) *pb.ServiceAttachmentKey {
	b := key.Proposal.Bytes()
	return &pb.ServiceAttachmentKey{Core: proto.String(string(key.Core)), Facet: proto.String(string(key.Facet)), Element: proto.String(string(key.Element)), Proposal: &pb.ServiceRevision{Incarnation: b[:32], Sequence: proto.Uint64(binary.BigEndian.Uint64(b[68:]))}}
}

// replacementPeerDone folds one step-done echo. A missing done marker or any
// transport failure is temporary Core-side unavailability.
func replacementPeerDone(ctx context.Context, done *pb.ReplacementStepDone, err error) error {
	if err != nil || ctx.Err() != nil || done == nil || !done.GetDone() {
		return corecontract.ErrUnavailable
	}
	return nil
}

func replacementPeerFailure(ctx context.Context, failure int32, err error) error {
	if err != nil || ctx.Err() != nil || failure != 0 {
		return corecontract.ErrUnavailable
	}
	return nil
}

func (p networkFacetSourcePeer) CheckSourceExport(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID) (replacement.FacetReplacementSourceState, error) {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementSourceState{}, err
	}
	r, err := client.Replacement().CheckSourceExport(ctx, &pb.ReplacementStepRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op)})
	if err2 := replacementPeerFailure(ctx, int32(r.GetFailure()), err); err2 != nil {
		return replacement.FacetReplacementSourceState{}, err2
	}
	w := r.GetResult()
	if w == nil {
		return replacement.FacetReplacementSourceState{}, corecontract.ErrUnavailable
	}
	out := replacement.FacetReplacementSourceState{Found: w.GetFound(), Fenced: w.GetFenced(), Refused: w.GetRefused(), Settled: w.GetSettled()}
	if len(w.GetManifest()) > 0 {
		if w.GetFinalCut() == 0 || len(w.GetManifestDigest()) != 32 {
			return replacement.FacetReplacementSourceState{}, corecontract.ErrUnavailable
		}
		out.FinalCut = w.GetFinalCut()
		copy(out.ManifestDigest[:], w.GetManifestDigest())
		out.Manifest = append([]byte(nil), w.GetManifest()...)
	}
	return out, nil
}

func (p networkFacetSourcePeer) FenceSource(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, target values.FactAuthorityRef) (replacement.FacetReplacementFenced, error) {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementFenced{}, err
	}
	r, err := client.Replacement().FenceSource(ctx, &pb.FenceSourceRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op), Target: proto.String(string(target))})
	if err2 := replacementPeerFailure(ctx, int32(r.GetFailure()), err); err2 != nil {
		return replacement.FacetReplacementFenced{}, err2
	}
	w := r.GetResult()
	if w == nil || !proto.Equal(w.Key, networkFacetServiceKey(key)) || w.GetFinalCut() == 0 || len(w.GetManifestDigest()) != 32 || len(w.GetManifest()) == 0 {
		return replacement.FacetReplacementFenced{}, corecontract.ErrUnavailable
	}
	out := replacement.FacetReplacementFenced{Key: key, FinalCut: w.GetFinalCut()}
	copy(out.ManifestDigest[:], w.GetManifestDigest())
	out.Manifest = append([]byte(nil), w.GetManifest()...)
	return out, nil
}

func (p networkFacetSourcePeer) ReadSourceEntry(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, typ values.FacetTypeRef) (replacement.FacetReplacementEntry, error) {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementEntry{}, err
	}
	r, err := client.Replacement().ReadSourceEntry(ctx, &pb.ReadSourceEntryRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op), TypeRef: typ.Bytes()})
	if err2 := replacementPeerFailure(ctx, int32(r.GetFailure()), err); err2 != nil {
		return replacement.FacetReplacementEntry{}, err2
	}
	w := r.GetResult()
	if w == nil || string(w.GetTypeRef()) != string(typ.Bytes()) {
		return replacement.FacetReplacementEntry{}, corecontract.ErrUnavailable
	}
	return replacement.FacetReplacementEntry{Type: typ, Payload: append([]byte(nil), w.GetPayload()...)}, nil
}

func (p networkFacetSourcePeer) SettleSource(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, target values.FactAuthorityRef) error {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return err
	}
	r, err := client.Replacement().SettleSource(ctx, &pb.SettleSourceRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op), Target: proto.String(string(target))})
	return replacementPeerDone(ctx, r.GetResult(), replacementPeerFailure(ctx, int32(r.GetFailure()), err))
}

// replacementLive mirrors the target's disclosed live key set; it never carries
// payload bytes.
func replacementLive(live []*pb.ReplacementLiveEntry) ([]replacement.FacetReplacementLiveEntry, bool) {
	out := make([]replacement.FacetReplacementLiveEntry, len(live))
	for i, e := range live {
		typ, err := values.NewFacetTypeRef(e.GetTypeRef())
		if err != nil || !typ.Valid() || e.GetLength() == 0 {
			return nil, false
		}
		out[i] = replacement.FacetReplacementLiveEntry{Type: typ, Length: e.GetLength()}
	}
	return out, true
}

// networkReplacementImportState mirrors the local peer projection: an import
// check that names the manifest always carries its digest, final cut and live
// key set, accepted or not; only the target's empty pre-pending answer omits
// them.
func networkReplacementImportState(w *pb.TargetImportState) (replacement.FacetReplacementImportState, error) {
	out := replacement.FacetReplacementImportState{Accepted: w.GetAccepted(), Installed: w.GetInstalled()}
	if w.GetFinalCut() == 0 || len(w.GetDigest()) != 32 {
		if !out.Accepted && !out.Installed {
			return out, nil
		}
		return replacement.FacetReplacementImportState{}, corecontract.ErrUnavailable
	}
	live, ok := replacementLive(w.GetLive())
	if !ok {
		return replacement.FacetReplacementImportState{}, corecontract.ErrUnavailable
	}
	out.FinalCut = w.GetFinalCut()
	copy(out.Digest[:], w.GetDigest())
	out.Live = live
	return out, nil
}

func (p networkFacetTargetPeer) CheckTargetImport(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, manifest []byte, coreAddress values.NextHop) (replacement.FacetReplacementImportState, error) {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementImportState{}, err
	}
	r, err := client.Replacement().CheckTargetImport(ctx, &pb.CheckTargetImportRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op), Manifest: manifest, CoreNextHop: proto.String(string(coreAddress))})
	if err2 := replacementPeerFailure(ctx, int32(r.GetFailure()), err); err2 != nil {
		return replacement.FacetReplacementImportState{}, err2
	}
	w := r.GetResult()
	if w == nil {
		return replacement.FacetReplacementImportState{}, corecontract.ErrUnavailable
	}
	return networkReplacementImportState(w)
}

func (p networkFacetTargetPeer) StageTargetEntry(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, typ values.FacetTypeRef, payload []byte) error {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return err
	}
	r, err := client.Replacement().StageTargetEntry(ctx, &pb.StageTargetEntryRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op), TypeRef: typ.Bytes(), Payload: payload})
	return replacementPeerDone(ctx, r.GetResult(), replacementPeerFailure(ctx, int32(r.GetFailure()), err))
}

func (p networkFacetTargetPeer) AcceptTarget(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, coreAddress values.NextHop) (replacement.FacetReplacementAccepted, error) {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return replacement.FacetReplacementAccepted{}, err
	}
	r, err := client.Replacement().AcceptTarget(ctx, &pb.AcceptTargetRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op), CoreNextHop: proto.String(string(coreAddress))})
	if err2 := replacementPeerFailure(ctx, int32(r.GetFailure()), err); err2 != nil {
		return replacement.FacetReplacementAccepted{}, err2
	}
	w := r.GetResult()
	if w == nil || !proto.Equal(w.Key, networkFacetServiceKey(key)) {
		return replacement.FacetReplacementAccepted{}, corecontract.ErrUnavailable
	}
	return replacement.FacetReplacementAccepted{Key: key}, nil
}

func (p networkFacetTargetPeer) InstallTarget(ctx context.Context, address values.NextHop, key values.FacetAcceptanceKey, op corecontract.OperationID, source values.FactAuthorityRef, finalCut uint64) error {
	client, err := p.source.replacementClient(ctx, address, key)
	if err != nil {
		return err
	}
	r, err := client.Replacement().InstallTarget(ctx, &pb.InstallTargetRequest{Key: networkFacetServiceKey(key), OperationId: []byte(op), Source: proto.String(string(source)), FinalCut: proto.Uint64(finalCut)})
	return replacementPeerDone(ctx, r.GetResult(), replacementPeerFailure(ctx, int32(r.GetFailure()), err))
}

// ReadTargetImport stays unsupported on the fixed network peer interface: the
// wire carries no target-side import read, and the single-writer recovery loop
// advances only through CheckTargetImport's disclosed live key set.
func (networkFacetTargetPeer) ReadTargetImport(context.Context, values.NextHop, values.FacetAcceptanceKey, corecontract.OperationID) (replacement.FacetReplacementImportState, error) {
	return replacement.FacetReplacementImportState{}, corecontract.ErrUnavailable
}
