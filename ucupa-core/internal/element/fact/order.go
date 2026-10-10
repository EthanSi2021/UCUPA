package fact

import (
	"bytes"
	"encoding/hex"
	"sort"

	"ucupa/shared/values"
)

// UUIDBytes retains the original binary ordering representation for validated IDs.
func UUIDBytes(id values.ElementID) []byte {
	text := string(id)
	compact := make([]byte, 32)
	copy(compact[0:8], text[0:8])
	copy(compact[8:12], text[9:13])
	copy(compact[12:16], text[14:18])
	copy(compact[16:20], text[19:23])
	copy(compact[20:32], text[24:36])
	decoded := make([]byte, 16)
	if _, err := hex.Decode(decoded, compact); err != nil {
		panic("validated UUID failed to decode")
	}
	return decoded
}

// SortIDs orders existing fact IDs for reads and canonical traversal.
func SortIDs(ids []values.ElementID) {
	sort.Slice(ids, func(left, right int) bool {
		return bytes.Compare(UUIDBytes(ids[left]), UUIDBytes(ids[right])) < 0
	})
}
