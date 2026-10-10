package policy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"

	contract "ucupa/ucupa-realm/contract"
	token "ucupa/ucupa-realm/internal/token"
)

func appendRealmField(encoded []byte, value string) ([]byte, error) {
	return appendRealmBytes(encoded, []byte(value))
}

func appendRealmBytes(encoded, value []byte) ([]byte, error) {
	if uint64(len(value)) > uint64(math.MaxUint32) {
		return nil, errors.New("Realm Store field exceeds u32 length")
	}
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(value)))
	return append(encoded, value...), nil
}

type realmDecoder struct {
	encoded []byte
	offset  int
}

func (decoder *realmDecoder) readByte() (byte, error) {
	value, err := decoder.read(1)
	if err != nil {
		return 0, err
	}
	return value[0], nil
}

func (decoder *realmDecoder) readField() ([]byte, error) {
	lengthBytes, err := decoder.read(4)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBytes)
	remaining := len(decoder.encoded) - decoder.offset
	if uint64(length) > uint64(remaining) {
		return nil, errors.New("Realm Store field is truncated")
	}
	return decoder.read(int(length))
}

func (decoder *realmDecoder) readRevision() (contract.BindingSetRevision, error) {
	encoded, err := decoder.read(40)
	if err != nil {
		return contract.BindingSetRevision{}, err
	}
	revision := token.Binding([32]byte(encoded[:32]), binary.BigEndian.Uint64(encoded[32:]))
	if !token.Valid(revision) {
		return contract.BindingSetRevision{}, errors.New("BindingSetRevision is invalid")
	}
	return revision, nil
}

func (decoder *realmDecoder) readCredentialID() (contract.CredentialID, error) {
	encoded, err := decoder.read(len(contract.CredentialID{}))
	if err != nil {
		return contract.CredentialID{}, err
	}
	return decodeCredentialID(encoded)
}

func (decoder *realmDecoder) readCredentialRevision() (contract.CredentialSetRevision, error) {
	encoded, err := decoder.read(40)
	if err != nil {
		return contract.CredentialSetRevision{}, err
	}
	revision := token.Credential([32]byte(encoded[:32]), binary.BigEndian.Uint64(encoded[32:]))
	if !token.Valid(revision) {
		return contract.CredentialSetRevision{}, errors.New("CredentialSetRevision is invalid")
	}
	return revision, nil
}

func (decoder *realmDecoder) read(size int) ([]byte, error) {
	if size < 0 || size > len(decoder.encoded)-decoder.offset {
		return nil, errors.New("Realm Store record is truncated")
	}
	end := decoder.offset + size
	value := decoder.encoded[decoder.offset:end]
	decoder.offset = end
	return value, nil
}

func (decoder *realmDecoder) done() bool {
	return decoder.offset == len(decoder.encoded)
}

func copyString(value []byte) string {
	return string(bytes.Clone(value))
}
