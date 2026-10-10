package wire

import (
	"testing"

	"google.golang.org/protobuf/proto"

	realmpb "ucupa/ucupa-realm/bindings/go/protobuf"
)

func requireShapeAccepted(t *testing.T, message proto.Message) {
	t.Helper()
	if _, err := Representation.ToValue(message.ProtoReflect(), 0); err != nil {
		t.Fatalf("valid shape rejected: %v", err)
	}
}

func requireShapeRejected(t *testing.T, message proto.Message) {
	t.Helper()
	if _, err := Representation.ToValue(message.ProtoReflect(), 0); err == nil {
		t.Fatal("invalid shape accepted")
	}
}

func TestRepresentationAcceptsExactEvidenceShapes(t *testing.T) {
	requireShapeAccepted(t, &realmpb.EstablishCallerRequest{Audience: proto.String("realm-a"), CredentialId: make([]byte, 16), Secret: make([]byte, 32)})
	requireShapeAccepted(t, &realmpb.EstablishForeignCallerRequest{Actor: proto.String(string(requirementsTestOwner)), Payload: make([]byte, 8), Signature: make([]byte, 64), Challenge: make([]byte, 32)})
	requireShapeAccepted(t, &realmpb.ForkCallerRequest{Caller: make([]byte, 32)})
	requireShapeAccepted(t, &realmpb.ReleaseHandleRequest{Handle: make([]byte, 32)})
	requireShapeAccepted(t, &realmpb.ValidateViewRequest{Caller: make([]byte, 32), View: make([]byte, 32), Requirements: []*realmpb.Requirement{coreElementRequirement("p")}})
	requireShapeAccepted(t, &realmpb.ValidateDenialViewRequest{Caller: make([]byte, 32), Denial: make([]byte, 32), Requirement: coreElementRequirement("p")})
}

func TestRepresentationRejectsWrongEvidenceSizes(t *testing.T) {
	requireShapeRejected(t, &realmpb.EstablishCallerRequest{Audience: proto.String("realm-a"), CredentialId: make([]byte, 15), Secret: make([]byte, 32)})
	requireShapeRejected(t, &realmpb.EstablishCallerRequest{Audience: proto.String("realm-a"), CredentialId: make([]byte, 16), Secret: make([]byte, 31)})
	requireShapeRejected(t, &realmpb.EstablishForeignCallerRequest{Actor: proto.String(string(requirementsTestOwner)), Payload: make([]byte, 8), Signature: make([]byte, 63), Challenge: make([]byte, 32)})
	requireShapeRejected(t, &realmpb.EstablishForeignCallerRequest{Actor: proto.String(string(requirementsTestOwner)), Payload: make([]byte, 8), Signature: make([]byte, 64), Challenge: make([]byte, 31)})
	requireShapeRejected(t, &realmpb.EstablishForeignCallerRequest{Actor: proto.String(string(requirementsTestOwner)), Payload: make([]byte, 4097), Signature: make([]byte, 64), Challenge: make([]byte, 32)})
	requireShapeRejected(t, &realmpb.ForkCallerRequest{Caller: make([]byte, 31)})
	requireShapeRejected(t, &realmpb.ReleaseHandleRequest{Handle: make([]byte, 33)})
	requireShapeRejected(t, &realmpb.ValidateViewRequest{Caller: make([]byte, 32), View: make([]byte, 16), Requirements: []*realmpb.Requirement{coreElementRequirement("p")}})
	requireShapeRejected(t, &realmpb.ValidateDenialViewRequest{Caller: make([]byte, 32), Denial: make([]byte, 16), Requirement: coreElementRequirement("p")})
}

func TestRepresentationRejectsMissingRequiredValues(t *testing.T) {
	requireShapeRejected(t, &realmpb.EstablishCallerRequest{CredentialId: make([]byte, 16), Secret: make([]byte, 32)})
	requireShapeRejected(t, &realmpb.ForkCallerRequest{})
	requireShapeRejected(t, &realmpb.ReleaseHandleRequest{})
	// An empty requirement list is not a wire-shape fault: repeated fields have
	// no presence, and the handler's requirement-set validation rejects it.
}

func TestRepresentationBoundsRequirementList(t *testing.T) {
	requirements := make([]*realmpb.Requirement, 0, MaxRequirements+1)
	for i := 0; i <= MaxRequirements; i++ {
		requirements = append(requirements, coreElementRequirement("p"))
	}
	requireShapeRejected(t, &realmpb.AuthorizeViewRequest{Caller: make([]byte, 32), Requirements: requirements})
}
