package encoding

import (
	"encoding/json"
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	storebytes "ucupa/ucupa-core/internal/store"
	storerecord "ucupa/ucupa-core/internal/store/record"
	"ucupa/ucupa-core/internal/token"
	transferfact "ucupa/ucupa-core/internal/transfer/fact"
)

// Private storage encoding only. Reuse the original tuple/Revision codec, and
// preserve arbitrary valid Principal/Operation bytes instead of JSON replacing
// non-UTF8 strings. No source key, approval secret or runtime view is persisted.
type StoredTransferIntent struct {
	Profile        transferfact.Profile       `json:"profile"`
	Operation      []byte                     `json:"operation"`
	ActorRealm     []byte                     `json:"actorRealm"`
	ActorPrincipal []byte                     `json:"actorPrincipal"`
	Source         values.FactAuthorityRef    `json:"source"`
	Target         values.FactAuthorityRef    `json:"target"`
	Boundary       values.ElementID           `json:"boundary"`
	Expected       storerecord.StoredRevision `json:"expected"`
	Final          *storerecord.StoredTuple   `json:"final,omitempty"`
	SourceAddress  values.NextHop             `json:"sourceAddress"`
	TargetAddress  values.NextHop             `json:"targetAddress"`
}

type StoredTransferEvent struct {
	TargetParent        []byte                         `json:"targetParent,omitempty"`
	Parent              *storedParentIntent            `json:"parent,omitempty"`
	ParentRelease       *storedParentRelease           `json:"parentRelease,omitempty"`
	EdgeResults         []byte                         `json:"edgeResults,omitempty"`
	Completion          *storedCoreCompletion          `json:"completion,omitempty"`
	EdgeInstallation    *storedEdgeInstallation        `json:"edgeInstallation,omitempty"`
	EdgeFence           *storedEdgeFence               `json:"edgeFence,omitempty"`
	CarrierInstallation *storedCarrierInstallation     `json:"carrierInstallation,omitempty"`
	CarrierBinding      *storedCarrierBinding          `json:"carrierBinding,omitempty"`
	CarrierAcceptance   *storedCarrierAcceptance       `json:"carrierAcceptance,omitempty"`
	CarrierFence        *StoredCarrierFence            `json:"carrierFence,omitempty"`
	CarrierPermit       *storedCarrierPermit           `json:"carrierPermit,omitempty"`
	CarrierRefusal      *storedCarrierRefusal          `json:"carrierRefusal,omitempty"`
	Carrier             *storedCarrierIntent           `json:"carrier,omitempty"`
	CarrierExpected     *storerecord.StoredMountCut    `json:"carrierExpected,omitempty"`
	Facet               *storedFacetReplacementIntent  `json:"facet,omitempty"`
	FacetFence          *storedFacetReplacementFence   `json:"facetFence,omitempty"`
	FacetRefusal        *storedFacetReplacementRefusal `json:"facetRefusal,omitempty"`
	Kind                TransferEventKind              `json:"kind"`
	Cut                 uint64                         `json:"cut"`
	Input               *StoredTransferIntent          `json:"input,omitempty"`
	Operation           []byte                         `json:"operation,omitempty"`
	ActorRealm          []byte                         `json:"actorRealm,omitempty"`
	ActorPrincipal      []byte                         `json:"actorPrincipal,omitempty"`
	Expected            uint64                         `json:"expected,omitempty"`
	Manifest            []byte                         `json:"manifest,omitempty"`
	Closures            []byte                         `json:"closures,omitempty"`
	Fence               *storedTransferFence           `json:"fence,omitempty"`
	Acceptance          *storedTransferAcceptance      `json:"acceptance,omitempty"`
	Decision            *storedTransferDecision        `json:"decision,omitempty"`
	Installation        *StoredTransferInstallation    `json:"installation,omitempty"`
}

type StoredTransferInstallation struct {
	Decision storedTransferDecision `json:"decision"`
	Scope    [32]byte               `json:"scope"`
	Cut      uint64                 `json:"cut"`
	Revision uint64                 `json:"revision"`
}

func StoreTransferDecision(d transferfact.SourceDecision) storedTransferDecision {
	return storedTransferDecision{Acceptance: storeTransferAcceptance(d.Acceptance), Scope: token.Scope(d.At), Cut: token.Serial(d.At), Revision: d.Revision}
}

func (s storedTransferDecision) decision() (transferfact.SourceDecision, error) {
	a, err := s.Acceptance.acceptance()
	return transferfact.SourceDecision{Acceptance: a, At: token.FromParts(s.Scope, s.Cut), Revision: s.Revision}, err
}

type storedTransferDecision struct {
	Acceptance storedTransferAcceptance `json:"acceptance"`
	Scope      [32]byte                 `json:"scope"`
	Cut        uint64                   `json:"cut"`
	Revision   uint64                   `json:"revision"`
}

type storedTransferAcceptance struct {
	Fence    storedTransferFence `json:"fence"`
	Scope    [32]byte            `json:"scope"`
	Cut      uint64              `json:"cut"`
	Revision uint64              `json:"revision"`
}

func storeTransferAcceptance(a transferfact.TargetAcceptance) storedTransferAcceptance {
	return storedTransferAcceptance{Fence: storeTransferFence(a.Fence), Scope: token.Scope(a.At), Cut: token.Serial(a.At), Revision: a.Revision}
}

func (s storedTransferAcceptance) acceptance() (transferfact.TargetAcceptance, error) {
	f, err := s.Fence.fence()
	return transferfact.TargetAcceptance{Fence: f, At: token.FromParts(s.Scope, s.Cut), Revision: s.Revision}, err
}

type storedTransferFence struct {
	Input  StoredTransferIntent `json:"input"`
	Scope  [32]byte             `json:"scope"`
	Cut    uint64               `json:"cut"`
	Digest [32]byte             `json:"digest"`
}

func storeTransferFence(f transferfact.SourceFence) storedTransferFence {
	return storedTransferFence{Input: storeTransferIntent(f.Input), Scope: token.Scope(f.FinalCut), Cut: token.Serial(f.FinalCut), Digest: f.ManifestDigest}
}

func (s storedTransferFence) fence() (transferfact.SourceFence, error) {
	i, err := s.Input.intent()
	return transferfact.SourceFence{Input: i, FinalCut: token.FromParts(s.Scope, s.Cut), ManifestDigest: s.Digest}, err
}

func storeTransferIntent(i transferfact.Intent) StoredTransferIntent {
	var final *storerecord.StoredTuple
	if i.Profile == transferfact.SliceTransfer {
		v := storerecord.StoreTuple(i.Final)
		final = &v
	}
	return StoredTransferIntent{Profile: i.Profile, Operation: []byte(i.Operation), ActorRealm: []byte(i.Actor.Realm), ActorPrincipal: []byte(i.Actor.Principal),
		Source: i.Source, Target: i.Target, Boundary: i.Boundary, Expected: storerecord.StoreRevision(i.Expected), Final: final,
		SourceAddress: i.SourceAddress, TargetAddress: i.TargetAddress}
}

func EncodeTransferIntent(i transferfact.Intent) ([]byte, error) {
	if !i.Valid() {
		return nil, contract.ErrInvalid
	}
	// Bound caller-controlled allocations before converting identities to bytes.
	if len(i.Actor.Realm) > MaxTransferIntentBytes || len(i.Actor.Principal) > MaxTransferIntentBytes {
		return nil, values.PublicResourceExhausted
	}
	data, err := json.Marshal(storeTransferIntent(i))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTransferIntentBytes {
		return nil, values.PublicResourceExhausted
	}
	return data, nil
}

func (s StoredTransferIntent) intent() (transferfact.Intent, error) {
	expected, err := s.Expected.Revision()
	if err != nil {
		return transferfact.Intent{}, err
	}
	var final contract.SpatialTuple
	switch s.Profile {
	case transferfact.RootSuccessor:
		if s.Final != nil {
			return transferfact.Intent{}, contract.ErrInvalid
		}
	case transferfact.SliceTransfer:
		if s.Final == nil {
			return transferfact.Intent{}, contract.ErrInvalid
		}
		final, err = s.Final.Tuple()
		if err != nil {
			return transferfact.Intent{}, err
		}
	default:
		return transferfact.Intent{}, contract.ErrInvalid
	}
	i := transferfact.Intent{Profile: s.Profile, Operation: contract.OperationID(string(s.Operation)),
		Actor:  values.PrincipalRef{Realm: values.RealmID(string(s.ActorRealm)), Principal: values.PrincipalID(string(s.ActorPrincipal))},
		Source: s.Source, Target: s.Target, Boundary: s.Boundary, Expected: expected, Final: final,
		SourceAddress: s.SourceAddress, TargetAddress: s.TargetAddress}
	_, err = EncodeTransferIntent(i)
	return i, err
}

func EncodeTransferJournal(events []TransferEvent) ([]byte, error) {
	if len(events) > MaxTransferEvents {
		return nil, values.PublicResourceExhausted
	}
	stored := make([]StoredTransferEvent, 0, len(events))
	for _, e := range events {
		if len(e.TargetParent) > MaxTransferParentProofBytes || e.Kind != TransferTargetAccepted && e.TargetParent != "" {
			return nil, contract.ErrInvalid
		}
		if len(e.EdgeResults) > maxTransferResultBytes || e.Kind != TransferTargetCompleted && e.EdgeResults != "" {
			return nil, contract.ErrInvalid
		}
		if len(e.Closures) > MaxTransferClosureBytes {
			return nil, values.PublicResourceExhausted
		}
		if e.Kind != TransferTargetAccepted && e.Kind != TransferCarrierAccepted && e.Closures != "" {
			return nil, contract.ErrInvalid
		}
		s := StoredTransferEvent{Kind: e.Kind, Cut: token.Serial(e.At), Operation: []byte(e.Operation),
			ActorRealm: []byte(e.Actor.Realm), ActorPrincipal: []byte(e.Actor.Principal), Expected: e.Expected, Closures: []byte(e.Closures), EdgeResults: []byte(e.EdgeResults)}
		s.TargetParent = []byte(e.TargetParent)
		if err := encodeTransferCompletion(e, &s); err != nil {
			return nil, err
		}
		if err := encodeTransferParentEvent(e, &s); err != nil {
			return nil, err
		}
		if e.Kind == TransferEdgeInstalled {
			if !e.EdgeParent.Valid() || !e.EdgeMount.Valid() {
				return nil, contract.ErrInvalid
			}
			s.EdgeInstallation = &storedEdgeInstallation{Parent: storeCoreInstallation(e.EdgeParent), Mount: e.EdgeMount}
		} else if e.EdgeParent != (transferfact.CoreInstallation{}) || e.EdgeMount != "" {
			return nil, contract.ErrInvalid
		}
		if e.Kind == TransferEdgeClosed {
			if !e.EdgeFence.Valid() {
				return nil, contract.ErrInvalid
			}
			s.EdgeFence = storeEdgeFence(e.EdgeFence)
		} else if e.EdgeFence != (transferfact.EdgeFence{}) {
			return nil, contract.ErrInvalid
		}
		if err := encodeCarrierEvent(e, &s); err != nil {
			return nil, err
		}
		if err := encodeCarrierAcceptanceEvent(e, &s); err != nil {
			return nil, err
		}
		if err := encodeFacetReplacementEvent(e, &s); err != nil {
			return nil, err
		}
		if e.Kind == TransferPending {
			if _, err := EncodeTransferIntent(e.Input); err != nil {
				return nil, err
			}
			i := storeTransferIntent(e.Input)
			s.Input = &i
		}
		if e.Kind == TransferTargetAccepted {
			if _, err := EncodeTransferIntent(e.Fence.Input); err != nil {
				return nil, err
			}
			f := storeTransferFence(e.Fence)
			s.Fence = &f
		}
		if e.Kind == TransferSuccessorDecided {
			if _, err := EncodeTransferIntent(e.Acceptance.Fence.Input); err != nil {
				return nil, err
			}
			a := storeTransferAcceptance(e.Acceptance)
			s.Acceptance = &a
		}
		if e.Kind == TransferTargetInstalled {
			if _, err := EncodeTransferIntent(e.Decision.Acceptance.Fence.Input); err != nil {
				return nil, err
			}
			d := StoreTransferDecision(e.Decision)
			s.Decision = &d
		}
		if e.Kind == TransferSourceSettled {
			if _, err := EncodeTransferIntent(e.Installation.Decision.Acceptance.Fence.Input); err != nil {
				return nil, err
			}
			s.Installation = &StoredTransferInstallation{Decision: StoreTransferDecision(e.Installation.Decision),
				Scope: token.Scope(e.Installation.At), Cut: token.Serial(e.Installation.At), Revision: e.Installation.Revision}
		}
		stored = append(stored, s)
		if len(e.Manifest) > MaxTransferManifestBytes {
			return nil, values.PublicResourceExhausted
		}
		stored[len(stored)-1].Manifest = []byte(e.Manifest)
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTransferJournalBytes {
		return nil, values.PublicResourceExhausted
	}
	return data, nil
}

func LoadTransferJournal(data []byte, scope [32]byte) ([]TransferEvent, error) {
	if len(data) == 0 || len(data) > MaxTransferJournalBytes {
		return nil, errors.New("transfer journal size is invalid")
	}
	var stored []StoredTransferEvent
	if err := storebytes.DecodeCanonicalJSON(data, &stored); err != nil {
		return nil, err
	}
	if stored == nil || len(stored) > MaxTransferEvents {
		return nil, errors.New("transfer journal count is invalid")
	}
	events := make([]TransferEvent, 0, len(stored))
	for _, s := range stored {
		if len(s.TargetParent) > MaxTransferParentProofBytes || s.Kind != TransferTargetAccepted && len(s.TargetParent) != 0 {
			return nil, contract.ErrInvalid
		}
		if len(s.EdgeResults) > maxTransferResultBytes || s.Kind != TransferTargetCompleted && len(s.EdgeResults) != 0 {
			return nil, contract.ErrInvalid
		}
		if len(s.Closures) > MaxTransferClosureBytes || (s.Kind != TransferTargetAccepted && s.Kind != TransferCarrierAccepted && len(s.Closures) != 0) {
			return nil, contract.ErrInvalid
		}
		e := TransferEvent{Kind: s.Kind, At: token.FromParts(scope, s.Cut), Operation: contract.OperationID(string(s.Operation)),
			Actor: values.PrincipalRef{Realm: values.RealmID(string(s.ActorRealm)), Principal: values.PrincipalID(string(s.ActorPrincipal))}, Expected: s.Expected, Closures: string(s.Closures), EdgeResults: string(s.EdgeResults)}
		e.TargetParent = string(s.TargetParent)
		if s.Kind == TransferEdgeInstalled {
			if s.EdgeInstallation == nil {
				return nil, contract.ErrInvalid
			}
			parent, err := s.EdgeInstallation.Parent.installation()
			if err != nil {
				return nil, err
			}
			e.EdgeParent, e.EdgeMount = parent, s.EdgeInstallation.Mount
		} else if s.EdgeInstallation != nil {
			return nil, contract.ErrInvalid
		}
		if s.Kind == TransferEdgeClosed {
			f, err := s.EdgeFence.fence()
			if err != nil {
				return nil, err
			}
			e.EdgeFence = f
		} else if s.EdgeFence != nil {
			return nil, contract.ErrInvalid
		}
		if err := decodeCarrierEvent(s, &e); err != nil {
			return nil, err
		}
		if err := decodeCarrierAcceptanceEvent(s, &e); err != nil {
			return nil, err
		}
		if err := decodeFacetReplacementEvent(s, &e); err != nil {
			return nil, err
		}
		if len(s.Manifest) > MaxTransferManifestBytes {
			return nil, errors.New("transfer manifest exceeds limit")
		}
		e.Manifest = string(s.Manifest)
		if s.Kind == TransferPending {
			if s.Input == nil {
				return nil, errors.New("transfer intent is missing")
			}
			i, err := s.Input.intent()
			if err != nil {
				return nil, err
			}
			e.Input = i
		} else if s.Input != nil {
			return nil, errors.New("non-initial transfer event contains another intent")
		}
		if s.Kind == TransferTargetAccepted {
			if s.Fence == nil {
				return nil, errors.New("target acceptance lacks source fence")
			}
			f, err := s.Fence.fence()
			if err != nil {
				return nil, err
			}
			e.Fence = f
		} else if s.Fence != nil {
			return nil, errors.New("source event carries target acceptance")
		}
		if s.Kind == TransferSuccessorDecided {
			if s.Acceptance == nil {
				return nil, errors.New("source successor lacks target acceptance")
			}
			a, err := s.Acceptance.acceptance()
			if err != nil {
				return nil, err
			}
			e.Acceptance = a
		} else if s.Acceptance != nil {
			return nil, errors.New("non-successor event carries target acceptance")
		}
		if s.Kind == TransferTargetInstalled {
			if s.Decision == nil {
				return nil, errors.New("target installation lacks source decision")
			}
			d, err := s.Decision.decision()
			if err != nil {
				return nil, err
			}
			e.Decision = d
		} else if s.Decision != nil {
			return nil, errors.New("non-installation event carries source decision")
		}
		if s.Kind == TransferSourceSettled {
			if s.Installation == nil {
				return nil, errors.New("source settlement lacks installed result")
			}
			d, err := s.Installation.Decision.decision()
			if err != nil {
				return nil, err
			}
			e.Installation = transferfact.TargetInstallation{Decision: d, At: token.FromParts(s.Installation.Scope, s.Installation.Cut), Revision: s.Installation.Revision}
		} else if s.Installation != nil {
			return nil, errors.New("non-settlement event carries installed result")
		}
		if err := decodeTransferCompletion(s, &e); err != nil {
			return nil, err
		}
		if err := decodeTransferParentEvent(s, &e); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}
