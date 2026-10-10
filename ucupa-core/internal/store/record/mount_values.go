package record

import (
	"encoding/hex"
	"errors"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	mountfact "ucupa/ucupa-core/internal/mount/fact"
	"ucupa/ucupa-core/internal/token"
)

type StoredMountCut struct {
	Scope  string `json:"scope"`
	Serial uint64 `json:"serial"`
}

type StoredMountKey struct {
	Parent   string         `json:"parent"`
	Anchor   string         `json:"anchor"`
	Proposal StoredRevision `json:"proposal"`
	Carrier  string         `json:"carrier"`
}

type StoredMountAdmission struct {
	Key       StoredMountKey `json:"key"`
	Operation []byte         `json:"operation"`
}

type StoredMountContactRevision struct {
	Scope    string         `json:"scope"`
	Key      StoredMountKey `json:"key"`
	Incoming bool           `json:"incoming"`
	Serial   uint64         `json:"serial"`
}

type StoredMountPrincipal struct {
	Realm     []byte `json:"realm"`
	Principal []byte `json:"principal"`
}

func StoreMountPrincipal(actor values.PrincipalRef) StoredMountPrincipal {
	return StoredMountPrincipal{Realm: []byte(actor.Realm), Principal: []byte(actor.Principal)}
}

func (stored StoredMountPrincipal) DecodePrincipal() values.PrincipalRef {
	return values.PrincipalRef{Realm: values.RealmID(string(stored.Realm)), Principal: values.PrincipalID(string(stored.Principal))}
}

func StoreMountCut(cut contract.CoreReadCut) *StoredMountCut {
	if cut == (contract.CoreReadCut{}) {
		return nil
	}
	scope := token.Scope(cut)
	return &StoredMountCut{Scope: hex.EncodeToString(scope[:]), Serial: token.Serial(cut)}
}

func loadMountScope(encoded string) ([32]byte, error) {
	var scope [32]byte
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != len(scope) || hex.EncodeToString(decoded) != encoded {
		return scope, errors.New("mount token scope is not canonical")
	}
	copy(scope[:], decoded)
	if scope == ([32]byte{}) {
		return scope, errors.New("mount token scope is zero")
	}
	return scope, nil
}

func (stored *StoredMountCut) Cut() (contract.CoreReadCut, error) {
	if stored == nil {
		return contract.CoreReadCut{}, nil
	}
	scope, err := loadMountScope(stored.Scope)
	if err != nil {
		return contract.CoreReadCut{}, err
	}
	cut := token.FromParts(scope, stored.Serial)
	if !token.Valid(cut) {
		return contract.CoreReadCut{}, errors.New("mount cut is invalid")
	}
	return cut, nil
}

func StoreMountRevision(revision contract.MountRevision) *StoredRevision {
	if revision == (contract.MountRevision{}) {
		return nil
	}
	stored := StoreRevision(token.RevisionFromParts(token.MountRevisionScope(revision), token.MountRevisionElement(revision), token.MountRevisionSerial(revision)))
	return &stored
}

func LoadMountRevision(stored *StoredRevision) (contract.MountRevision, error) {
	if stored == nil {
		return contract.MountRevision{}, nil
	}
	revision, err := stored.Revision()
	return token.MountRevisionFromParts(token.RevisionScope(revision), token.RevisionElement(revision), token.RevisionSerial(revision)), err
}

func StoreMountKey(key contract.MountKey) *StoredMountKey {
	if key == (contract.MountKey{}) {
		return nil
	}
	stored := storeMountKeyValue(key)
	return &stored
}

func storeMountKeyValue(key contract.MountKey) StoredMountKey {
	return StoredMountKey{
		Parent: string(key.ParentAuthority), Anchor: string(key.MountAnchorUUID),
		Proposal: StoreRevision(token.RevisionFromParts(token.MountRevisionScope(key.ProposalRevision), token.MountRevisionElement(key.ProposalRevision), token.MountRevisionSerial(key.ProposalRevision))), Carrier: string(key.CarrierAuthority),
	}
}

func (stored *StoredMountKey) Key() (contract.MountKey, error) {
	if stored == nil {
		return contract.MountKey{}, nil
	}
	revision, err := LoadMountRevision(&stored.Proposal)
	if err != nil {
		return contract.MountKey{}, err
	}
	key := contract.MountKey{ParentAuthority: values.FactAuthorityRef(stored.Parent), MountAnchorUUID: values.ElementID(stored.Anchor), ProposalRevision: revision, CarrierAuthority: values.FactAuthorityRef(stored.Carrier)}
	if !key.Valid() {
		return contract.MountKey{}, errors.New("stored MountKey is invalid")
	}
	return key, nil
}

func StoreMountAdmission(key contract.MountAdmissionKey) *StoredMountAdmission {
	if key == (contract.MountAdmissionKey{}) {
		return nil
	}
	return &StoredMountAdmission{Key: storeMountKeyValue(key.MountKey), Operation: []byte(key.OperationID)}
}

func (stored *StoredMountAdmission) Admission() (contract.MountAdmissionKey, error) {
	if stored == nil {
		return contract.MountAdmissionKey{}, nil
	}
	key, err := stored.Key.Key()
	if err != nil {
		return contract.MountAdmissionKey{}, err
	}
	admission := contract.MountAdmissionKey{MountKey: key, OperationID: contract.OperationID(string(stored.Operation))}
	if !mountfact.ValidMountAdmissionKey(admission) {
		return contract.MountAdmissionKey{}, errors.New("stored AdmissionKey is invalid")
	}
	return admission, nil
}

func StoreMountContactRevision(revision contract.NextHopRevision) *StoredMountContactRevision {
	if revision == (contract.NextHopRevision{}) {
		return nil
	}
	return &StoredMountContactRevision{Scope: hex.EncodeToString(token.NextHopScopeBytes(revision)), Key: storeMountKeyValue(token.NextHopKey(revision)), Incoming: token.NextHopIncoming(revision), Serial: token.NextHopSerial(revision)}
}

func (stored *StoredMountContactRevision) Revision() (contract.NextHopRevision, error) {
	if stored == nil {
		return contract.NextHopRevision{}, nil
	}
	scope, err := loadMountScope(stored.Scope)
	if err != nil {
		return contract.NextHopRevision{}, err
	}
	key, err := stored.Key.Key()
	if err != nil {
		return contract.NextHopRevision{}, err
	}
	revision := token.NextHopRevisionFromParts(scope, key, stored.Incoming, stored.Serial)
	if !token.ValidNextHopRevision(revision) {
		return contract.NextHopRevision{}, errors.New("stored NextHopRevision is invalid")
	}
	return revision, nil
}

func optionalStoreTuple(tuple contract.SpatialTuple) *StoredTuple {
	if tuple == (contract.SpatialTuple{}) {
		return nil
	}
	stored := StoreTuple(tuple)
	return &stored
}

func optionalLoadTuple(stored *StoredTuple) (contract.SpatialTuple, error) {
	if stored == nil {
		return contract.SpatialTuple{}, nil
	}
	return stored.Tuple()
}

func optionalStoreRevision(revision contract.Revision) *StoredRevision {
	if revision == (contract.Revision{}) {
		return nil
	}
	stored := StoreRevision(revision)
	return &stored
}

func optionalLoadRevision(stored *StoredRevision) (contract.Revision, error) {
	if stored == nil {
		return contract.Revision{}, nil
	}
	return stored.Revision()
}
