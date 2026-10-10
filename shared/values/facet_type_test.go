package values

import (
	"bytes"
	"sort"
	"testing"
)

func TestFacetTypeRefOwnsExactBytes(t *testing.T) {
	input := append([]byte{0, 255, '/', 128}, make([]byte, 28)...)
	ref, err := NewFacetTypeRef(input)
	if err != nil || !ref.Valid() {
		t.Fatal(ref, err)
	}
	want := bytes.Clone(input)
	input[0] = 4
	output := ref.Bytes()
	output[1] = 0
	if !bytes.Equal(ref.Bytes(), want) {
		t.Fatal("aliased type bytes")
	}
	same, _ := NewFacetTypeRef(want)
	if !ref.Equal(same) || map[FacetTypeRef]bool{ref: true}[same] != true {
		t.Fatal("exact identity differs")
	}
}

func TestFacetTypeRefBoundariesAndOrdering(t *testing.T) {
	for _, size := range []int{0, 1, 31, 32, 33, 256, 257} {
		ref, err := NewFacetTypeRef(make([]byte, size))
		valid := size == 32
		if (err == nil) != valid || ref.Valid() != valid {
			t.Fatalf("size %d: %v %v", size, ref, err)
		}
		if !valid && ref != (FacetTypeRef{}) {
			t.Fatal("invalid construction returned value")
		}
	}
	if (FacetTypeRef{}).Valid() {
		t.Fatal("zero valid")
	}
	refs := []FacetTypeRef{testType("\xff"), testType("aa"), testType("A"), testType("a"), testType("\x00")}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Less(refs[j]) })
	for i := 1; i < len(refs); i++ {
		if bytes.Compare(refs[i-1].Bytes(), refs[i].Bytes()) >= 0 {
			t.Fatal("not raw byte order")
		}
	}
}
