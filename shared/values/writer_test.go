package values

import (
	"strings"
	"testing"
)

func TestWriterInstallationIsBoundedAndPrivate(t *testing.T) {
	a, err := NewWriterInstallation("handoff")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewWriterInstallation("handoff")
	if err != nil {
		t.Fatal(err)
	}
	if a.Secret == b.Secret || a.Secret == [32]byte{} || a.Commitment() == b.Commitment() {
		t.Fatal("capabilities not independent")
	}
	for _, op := range []string{"", strings.Repeat("x", 129)} {
		if _, err := NewWriterInstallation(op); err == nil {
			t.Fatal("invalid operation accepted")
		}
		if (WriterHandoffRequest{Operation: op, ExpectedEpoch: 1, Target: a.Commitment()}).Valid() {
			t.Fatal("invalid operation accepted for handoff")
		}
	}
	r := WriterHandoffRequest{Operation: a.Operation, ExpectedEpoch: 1, Target: a.Commitment()}
	if !r.Valid() {
		t.Fatal("valid handoff rejected")
	}
	r.ExpectedEpoch = 0
	if r.Valid() {
		t.Fatal("zero epoch accepted")
	}
	r.ExpectedEpoch = 1
	r.Target = WriterInstallation{}.Commitment()
	if r.Valid() {
		t.Fatal("known zero secret accepted")
	}
	cfg := &WriterStartup{Handoff: true, Install: &a}
	if !cfg.Valid() {
		t.Fatal("valid target startup rejected")
	}
	cfg.Install = &WriterInstallation{Operation: "handoff"}
	if cfg.Valid() {
		t.Fatal("zero installation secret accepted")
	}
}
