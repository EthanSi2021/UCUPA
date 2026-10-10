package facetgrpc

import (
	"context"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
	"ucupa/shared/values"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
)

type adapterOwner struct {
	raw   []byte
	err   error
	calls int
}

func (*adapterOwner) AuthorityRef() values.FactAuthorityRef {
	return "00000000-0000-4000-8000-000000000001"
}
func (o *adapterOwner) PublicCall(context.Context, values.CallerContext, []byte) ([]byte, error) {
	o.calls++
	return o.raw, o.err
}

type adapterCallers struct{ err error }

func (c adapterCallers) Caller(context.Context) (values.CallerContext, error) {
	return values.CallerContext{Proof: [32]byte{1}}, c.err
}

func TestAdapterDoesNotExposeInternalFailures(t *testing.T) {
	owner := &adapterOwner{}
	request := &facetpb.ReleaseObserveRequest{Context: &facetpb.ReleaseObserveRequest_Cursor{Cursor: token(8)}}
	for _, failure := range []error{errors.New("private secret"), status.Error(codes.Unavailable, "private secret"), status.Error(codes.ResourceExhausted, "private secret")} {
		s, err := NewService(owner, adapterCallers{failure}, owner.AuthorityRef(), "https://facet.example:443", 4096)
		if err != nil {
			t.Fatal(err)
		}
		response, err := s.ReleaseObserve(context.Background(), request)
		if response != nil || err == nil || strings.Contains(err.Error(), "secret") || owner.calls != 0 {
			t.Fatalf("unsafe failure %v %v", response, err)
		}
	}
	s, err := NewService(owner, adapterCallers{}, owner.AuthorityRef(), "https://facet.example:443", 4096)
	if err != nil {
		t.Fatal(err)
	}
	owner.err = errors.New("private storage secret")
	if response, err := s.ReleaseObserve(context.Background(), request); response != nil || status.Code(err) != codes.Internal || strings.Contains(err.Error(), "secret") {
		t.Fatalf("owner failure %v %v", response, err)
	}
	owner.err = nil
	for _, body := range []string{`{"released":false}`, `{"released":true,"private":"secret"}`, `{"released":"true"}`} {
		owner.raw = []byte(body)
		if response, err := s.ReleaseObserve(context.Background(), request); response != nil || status.Code(err) != codes.Internal {
			t.Fatalf("malformed response %v %v", response, err)
		}
	}
	owner.raw = []byte(`{"released":true}`)
	response, err := s.ReleaseObserve(context.Background(), request)
	if err != nil || !proto.Equal(response.GetResult(), &facetpb.ReleaseObserveResult{Released: proto.Bool(true)}) {
		t.Fatalf("valid response %v %v", response, err)
	}
	owner.raw = make([]byte, 4097)
	if response, err := s.ReleaseObserve(context.Background(), request); response != nil || status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("response capacity %v %v", response, err)
	}
}
