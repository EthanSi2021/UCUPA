package wire

import (
	"errors"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"

	"ucupa/shared/values"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
)

func TestMountServiceWireRoundTrip(t *testing.T) {
	const element values.ElementID = "00000000-0000-4000-8000-000000000001"
	const parent values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"
	const carrier values.FactAuthorityRef = "00000000-0000-4000-8000-000000000003"
	e := corecontract.ServiceRevision{Incarnation: [32]byte{1}, Element: element, Sequence: math.MaxUint64}
	revision, _ := e.ElementRevision()
	mount, _ := e.MountRevision()
	rangeValue, _ := corecontract.NewBoundedSphere(42)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	tuple := corecontract.SpatialTuple{Parent: element, LocalPose: corecontract.Pose{Translation: corecontract.Vec3{X: 2}, Rotation: rotation}, Range: rangeValue}
	base := corecontract.MountPrepareRequest{Key: corecontract.MountKey{ParentAuthority: parent, CarrierAuthority: carrier, MountAnchorUUID: element, ProposalRevision: mount}, OperationID: corecontract.OperationID(string([]byte{0, 255, 3})), ParentActor: values.PrincipalRef{Realm: "parent", Principal: "user"}, CarrierActor: values.PrincipalRef{Realm: "carrier", Principal: "user"}, ReservedRevision: mount, Tuple: tuple}
	for _, kind := range []corecontract.MountCommandKind{corecontract.MountEstablish, corecontract.MountUpdate, corecontract.MountRevoke} {
		r := base
		r.Command = corecontract.MountCommand{Kind: kind, MountAnchorUUID: element}
		switch kind {
		case corecontract.MountEstablish:
			r.Command.ExpectedElementRevision = revision
			r.Command.CarrierAuthority = carrier
			r.Command.CarrierNextHop = "https://carrier.example:443"
			r.Command.ParentNextHop = "https://parent.example:443"
		case corecontract.MountUpdate:
			r.Command.ExpectedElementRevision = revision
			r.Command.ExpectedMountRevision = mount
			r.Command.Final = tuple
		case corecontract.MountRevoke:
			r.Command.ExpectedMountRevision = mount
			r.CarrierActor = values.PrincipalRef{}
		}
		if kind != corecontract.MountEstablish {
			r.PreviousAdmission = corecontract.MountAdmissionKey{MountKey: r.Key, OperationID: "established"}
		}
		wire := MountRequestWire(r)
		raw, err := proto.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		decoded := new(pb.ServiceMountPrepare)
		if err = proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		got, err := MountRequest(decoded)
		if err != nil || got != r {
			t.Fatalf("kind %d roundtrip: %#v %v", kind, got, err)
		}
		if decoded.Reserved.GetSequence() != math.MaxUint64 {
			t.Fatal("sequence rounded")
		}
	}
}

func TestMountServiceWireRejectsMalformedEvidence(t *testing.T) {
	const element = "00000000-0000-4000-8000-000000000001"
	for _, r := range []*pb.ServiceRevision{nil, {}, {Incarnation: make([]byte, 32), Sequence: proto.Uint64(1)}, {Incarnation: []byte{1}, Sequence: proto.Uint64(1)}} {
		if _, err := MountRevision(r, element); err == nil {
			t.Fatal("malformed revision accepted")
		}
	}
	if _, err := MountCut(&pb.ServiceCoreCut{Incarnation: make([]byte, 32), Sequence: proto.Uint64(1)}); err == nil {
		t.Fatal("zero incarnation")
	}
	rangeValue, _ := corecontract.NewBoundedSphere(1)
	rotation, _ := corecontract.NewRotation(1, 0, 0, 0)
	w := MountTupleWire(corecontract.SpatialTuple{Parent: values.ElementID(element), LocalPose: corecontract.Pose{Rotation: rotation}, Range: rangeValue})
	for _, mutate := range []func(*pb.SpatialTuple){func(w *pb.SpatialTuple) { w.Pose.Translation.Items = nil }, func(w *pb.SpatialTuple) { w.Pose.Rotation.Ratios.Items = []float64{1} }, func(w *pb.SpatialTuple) { w.Range.Radius = proto.Float64(math.NaN()) }, func(w *pb.SpatialTuple) { w.Pose.Rotation.Pivot = proto.Uint32(99) }} {
		bad := proto.Clone(w).(*pb.SpatialTuple)
		mutate(bad)
		if _, err := MountTuple(bad); err == nil {
			t.Fatal("malformed tuple accepted")
		}
	}
}

func TestFederationProjectionRequestRejectsAbsentOrUnknownKind(t *testing.T) {
	// Generated protobuf getters accept nil receivers; conversion must preserve
	// the original explicit invalid-input result, not panic or infer a kind.
	for _, input := range []*pb.FederationProjectionRequest{nil, {}, {Kind: proto.String("unknown")}} {
		if _, err := FederationProjectionRequest(input); !errors.Is(err, corecontract.ErrInvalid) {
			t.Fatalf("projection %#v: got %v, want ErrInvalid", input, err)
		}
	}
}
