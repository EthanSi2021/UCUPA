package commit_test

import (
	"errors"
	"path/filepath"
	"testing"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/commit"
)

func TestDurableOpenRejectsNonCurrentOrCorruptState(t *testing.T) {
	for _, name := range []string{
		"unrecognized marker",
		"superseded P7.3c marker",
		"superseded P7.3d marker",
		"superseded S2P1 marker",
		"missing marker",
		"empty marker",
		"unexpected bucket",
		"invalid Element UUID key",
		"unknown Element kind",
		"root carries Pose",
		"noncanonical Rotation",
		"noncanonical Element JSON",
		"unknown Operation field",
		"Anchor cycle",
		"out of parent Range",
		"duplicate Element Revision source",
		"forged same-scope expected Revision",
		"canonical impossible retained outcome",
		"legal current fact disagrees with committed history",
		"missing committed history",
		"cut Revision mismatch",
		"invalid bounded Range",
		"missing parent",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "corrupt.db")
			seedRangeStore(t, path)
			commit.CorruptCurrentStoreForTest(t, path, name)
			opened, err := commit.OpenDurable(path, nil)
			if !errors.Is(err, contract.ErrStore) || opened != nil {
				if opened != nil {
					_ = opened.CloseStore()
				}
				t.Fatalf("OpenDurable accepted corruption: owner=%#v err=%v", opened, err)
			}
		})
	}
}
