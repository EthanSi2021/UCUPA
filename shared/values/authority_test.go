package values

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
	"testing/iotest"
)

func TestFactAuthorityRefProfile(t *testing.T) {
	for _, variant := range []string{"8", "9", "a", "b"} {
		value := FactAuthorityRef("01234567-89ab-4cde-" + variant + "f01-23456789abcd")
		if !value.Valid() {
			t.Fatalf("canonical UUIDv4 rejected: %q", value)
		}
	}
	invalid := []FactAuthorityRef{
		"",
		"00000000-0000-0000-0000-000000000000",
		"01234567-89ab-4cde-8f01-23456789abc",
		"01234567-89AB-4cde-8f01-23456789abcd",
		"0123456-789ab-4cde-8f01-23456789abcd",
		"01234567-89ab-4cde-8f01-23456789abcg",
		"01234567-89ab-1cde-8f01-23456789abcd",
		"01234567-89ab-7cde-8f01-23456789abcd",
		"01234567-89ab-4cde-7f01-23456789abcd",
		"01234567-89ab-4cde-cf01-23456789abcd",
		"urn:uuid:01234567-89ab-4cde-8f01-23456789abcd",
		"{01234567-89ab-4cde-8f01-23456789abcd}",
		"0123456789ab4cde8f0123456789abcd",
		" 01234567-89ab-4cde-8f01-23456789abcd",
		"01234567-89ab-4cde-8f01-23456789abcd\n",
		"01234567-89ab-4cde-8f01-23456789abc\x00",
		"01234567-89ab-4cde-8f01-23456789abcｄ",
	}
	for _, value := range invalid {
		t.Run(string(value), func(t *testing.T) {
			if value.Valid() {
				t.Fatalf("invalid FactAuthorityRef accepted: %q", value)
			}
		})
	}
}

func TestFactAuthorityRefAndNextHopRemainDistinctNamedValues(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(FactAuthorityRef("")),
		reflect.TypeOf(NextHop("")),
		reflect.TypeOf(ElementID("")),
		reflect.TypeOf(""),
	}
	for i, actual := range types {
		if actual.Kind() != reflect.String {
			t.Fatalf("%v does not have underlying string kind", actual)
		}
		for _, other := range types[i+1:] {
			if actual == other {
				t.Fatalf("distinct value types collapsed: %v", actual)
			}
		}
	}
}

func TestNewFactAuthorityRef(t *testing.T) {
	seen := make(map[FactAuthorityRef]struct{})
	for range 64 {
		value, err := NewFactAuthorityRef()
		if err != nil {
			t.Fatalf("NewFactAuthorityRef(): %v", err)
		}
		if !value.Valid() {
			t.Fatalf("generated invalid FactAuthorityRef: %q", value)
		}
		if _, exists := seen[value]; exists {
			t.Fatalf("generated duplicate FactAuthorityRef: %q", value)
		}
		seen[value] = struct{}{}
	}
}

func TestNewFactAuthorityRefSetsVersionAndVariant(t *testing.T) {
	tests := []struct {
		name string
		fill byte
		want FactAuthorityRef
	}{
		{name: "zero entropy bytes", fill: 0x00, want: "00000000-0000-4000-8000-000000000000"},
		{name: "set entropy bytes", fill: 0xff, want: "ffffffff-ffff-4fff-bfff-ffffffffffff"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := iotest.OneByteReader(bytes.NewReader(bytes.Repeat([]byte{test.fill}, 16)))
			value, err := newFactAuthorityRef(reader)
			if err != nil || value != test.want {
				t.Fatalf("newFactAuthorityRef() = (%q, %v), want (%q, nil)", value, err, test.want)
			}
		})
	}
}

func TestNewFactAuthorityRefRejectsEntropyFailure(t *testing.T) {
	wantErr := errors.New("entropy unavailable")
	tests := []struct {
		name   string
		reader io.Reader
		want   error
	}{
		{name: "failed source", reader: iotest.ErrReader(wantErr), want: wantErr},
		{name: "empty source", reader: bytes.NewReader(nil), want: io.EOF},
		{name: "short source", reader: bytes.NewReader(make([]byte, 15)), want: io.ErrUnexpectedEOF},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := newFactAuthorityRef(test.reader)
			if value != "" || !errors.Is(err, test.want) {
				t.Fatalf("newFactAuthorityRef() = (%q, %v), want (zero, %v)", value, err, test.want)
			}
		})
	}
}
