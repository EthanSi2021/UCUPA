package token

import (
	"reflect"
	"testing"

	"ucupa/shared/values"
)

var _ interface {
	ElementRevision(Revision) values.PublicToken
	MountRevision(MountRevision) values.PublicToken
	MountContact(NextHopRevision) values.PublicToken
	AttachmentRevision(values.AttachmentRevision) values.PublicToken
	FacetContact(FacetContactRevision) values.PublicToken
} = PublicSigner{}

func TestPublicSignerFixedMethodsMatchExistingAlgorithms(t *testing.T) {
	revision, mountRevision, incoming, outgoing, facetContact, attachment := publicRevisionFixture(t)
	signer := NewPublicSigner(publicRevisionKey, publicRevisionOwner)
	cases := []struct {
		name string
		got  values.PublicToken
		want values.PublicToken
	}{
		{name: "element", got: signer.ElementRevision(revision), want: PublicElementRevision(publicRevisionKey, publicRevisionOwner, revision)},
		{name: "mount", got: signer.MountRevision(mountRevision), want: PublicMountRevision(publicRevisionKey, publicRevisionOwner, mountRevision)},
		{name: "mount-contact-incoming", got: signer.MountContact(incoming), want: PublicMountContact(publicRevisionKey, publicRevisionOwner, incoming)},
		{name: "mount-contact-outgoing", got: signer.MountContact(outgoing), want: PublicMountContact(publicRevisionKey, publicRevisionOwner, outgoing)},
		{name: "attachment", got: signer.AttachmentRevision(attachment), want: PublicAttachmentRevision(publicRevisionKey, publicRevisionOwner, attachment)},
		{name: "facet-contact", got: signer.FacetContact(facetContact), want: PublicFacetContact(publicRevisionKey, publicRevisionOwner, facetContact)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Fatalf("signer projection changed: got %#v want %#v", testCase.got, testCase.want)
			}
		})
	}
}

func TestPublicSignerPreservesInvalidAndZeroAttachmentSemantics(t *testing.T) {
	signer := NewPublicSigner(publicRevisionKey, publicRevisionOwner)
	if got := signer.ElementRevision(Revision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero Element revision was signed: %#v", got)
	}
	if got := signer.MountRevision(MountRevision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero mount revision was signed: %#v", got)
	}
	if got := signer.MountContact(NextHopRevision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero mount contact was signed: %#v", got)
	}
	if got := signer.FacetContact(FacetContactRevision{}); got != (values.PublicToken{}) {
		t.Fatalf("zero Facet contact was signed: %#v", got)
	}
	if got, want := signer.AttachmentRevision(values.AttachmentRevision{}), PublicAttachmentRevision(publicRevisionKey, publicRevisionOwner, values.AttachmentRevision{}); got != want || got == (values.PublicToken{}) {
		t.Fatalf("zero attachment behavior changed: got %#v want %#v", got, want)
	}
}

func TestPublicSignerDoesNotExposeCredentialsOrGenericSigning(t *testing.T) {
	typ := reflect.TypeOf(PublicSigner{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).PkgPath == "" {
			t.Fatalf("PublicSigner exposes field %q", typ.Field(i).Name)
		}
	}
	for _, name := range []string{"Key", "Owner", "Sign", "Valid"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Fatalf("PublicSigner exposes forbidden method %q", name)
		}
	}
}
