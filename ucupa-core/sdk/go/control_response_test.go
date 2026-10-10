package coresdk

import (
	"testing"

	"google.golang.org/protobuf/proto"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

func TestControlResponseRejectsMalformedEvidence(t *testing.T) {
	good := &corepb.ControlRead{Core: proto.String("00000000-0000-4000-8000-000000000001"), Element: proto.String("00000000-0000-4000-8000-000000000002"), Presence: corepb.Presence_NOT_FOUND.Enum(), Cut: &corepb.ServiceCoreCut{Incarnation: append([]byte{1}, make([]byte, 31)...), Sequence: proto.Uint64(1)}}
	if err := validateResponse(good.ProtoReflect(), 0); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*corepb.ControlRead){
		"zero cut":          func(r *corepb.ControlRead) { r.Cut.Sequence = proto.Uint64(0) },
		"empty incarnation": func(r *corepb.ControlRead) { r.Cut.Incarnation = make([]byte, 32) },
		"missing cut":       func(r *corepb.ControlRead) { r.Cut = nil },
		"absent with state": func(r *corepb.ControlRead) { r.State = corepb.AttachmentState_NONE.Enum() },
		"present without revision": func(r *corepb.ControlRead) {
			r.Presence = corepb.Presence_PRESENT.Enum()
			r.State = corepb.AttachmentState_NONE.Enum()
		},
		"unknown nested field": func(r *corepb.ControlRead) { r.Cut.ProtoReflect().SetUnknown([]byte{0x18, 1}) },
	} {
		t.Run(name, func(t *testing.T) {
			r := proto.Clone(good).(*corepb.ControlRead)
			change(r)
			if err := validateResponse(r.ProtoReflect(), 0); err == nil {
				t.Fatal("malformed service evidence accepted")
			}
		})
	}
}
