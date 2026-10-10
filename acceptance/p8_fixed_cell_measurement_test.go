package acceptance_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	netpb "ucupa/shared/bindings/go/protobuf"
	"ucupa/shared/values"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
	corecontract "ucupa/ucupa-core/contract"
	facetpb "ucupa/ucupa-facet/bindings/go/protobuf"
	facetsdk "ucupa/ucupa-facet/sdk/go"
)

// The opt-in measurement extends the same two-product public-flow test. It is
// excluded from the normal full gate because loading 999 durable records is a
// workload measurement, not a correctness prerequisite for every test run.
func seedP8FixedCell(t *testing.T, c *networkFixture) {
	t.Helper()
	c.grant(t, corecontract.PermissionCreateEntity, values.CoreElementScope(c.c.AuthorityRef(), networkRoot), false)
	for i := 0; i < 999; i++ {
		id := values.ElementID(fmt.Sprintf("00000000-0000-4000-8000-%012x", 0x1000+i))
		_, err := c.c.Elements().CreateEntity(c.actor, id, corecontract.SpatialTuple{
			Parent: networkRoot, LocalPose: mustPose(t, corecontract.Vec3{}, mustRotation(t, 1, 0, 0, 0)), Range: mustSphere(t, 1),
		}, corecontract.OperationID(fmt.Sprintf("p8-load-%d", i)))
		if err != nil {
			t.Fatalf("load Entity %d: %v", i, err)
		}
	}
}

func measureP8FixedCell(t *testing.T, c *networkFixture, client *facetsdk.Client, access *facetpb.Access, typ values.FacetTypeRef, facetDir string) {
	t.Helper()
	ctx := context.Background()
	measure := func(name string, prepare func(int) error, call func(int) error, release func(int) error) {
		t.Helper()
		const samples = 24
		times := make([]time.Duration, samples)
		for i := range times {
			if prepare != nil {
				if err := prepare(i); err != nil {
					t.Fatalf("%s preparation %d: %v", name, i, err)
				}
			}
			start := time.Now()
			if err := call(i); err != nil {
				t.Fatalf("%s sample %d: %v", name, i, err)
			}
			times[i] = time.Since(start)
			if release != nil {
				if err := release(i); err != nil {
					t.Fatalf("%s release %d: %v", name, i, err)
				}
			}
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		t.Logf("P8 loopback TLS 1000 Entities, %s n=%d p50=%s p95=%s", name, samples, times[samples/2], times[(samples*95+99)/100-1])
	}
	var enterHandle *netpb.Token
	closeHandle := func(handle *netpb.Token) error {
		out, err := c.client.Core().Close(ctx, &corepb.CloseRequest{Handle: handle})
		if err == nil && out.GetResult() == nil {
			return fmt.Errorf("no Close result: %v", out)
		}
		return err
	}
	measure("Core Enter", nil, func(_ int) error {
		out, err := c.client.Core().Enter(ctx, &corepb.EnterRequest{Reference: c.reference(), Intent: corepb.Intent_READ.Enum()})
		if err == nil && out.GetResult() == nil {
			return fmt.Errorf("no Enter result: %v", out)
		}
		if err == nil {
			enterHandle = out.GetResult().Handle
		}
		return err
	}, func(_ int) error { return closeHandle(enterHandle) })
	var readHandle *netpb.Token
	measure("Core ReadElement", func(_ int) error { readHandle = c.enter(t, corepb.Intent_READ); return nil }, func(_ int) error {
		out, err := c.client.Core().ReadElement(ctx, &corepb.ReadElementRequest{Handle: readHandle, Address: &netpb.Address{}})
		if err == nil && out.GetResult().GetRecord() == nil {
			return fmt.Errorf("no ReadElement record: %v", out)
		}
		return err
	}, func(_ int) error { return closeHandle(readHandle) })
	var queryHandle *netpb.Token
	measure("Core QueryRegion limit=32", func(_ int) error { queryHandle = c.enter(t, corepb.Intent_QUERY); return nil }, func(_ int) error {
		out, err := c.client.Core().QueryRegion(ctx, &corepb.QueryRegionRequest{
			Handle: queryHandle, Start: &netpb.Address{}, Basis: &netpb.Address{},
			Sphere: &corepb.Sphere{Center: &netpb.Vector3{Items: []float64{0, 0, 0}}, Radius: proto.Float64(2)}, Limit: proto.Uint32(32),
		})
		if err == nil && (out.GetResult() == nil || len(out.GetResult().Matches) == 0) {
			return fmt.Errorf("no QueryRegion matches: %v", out)
		}
		return err
	}, func(_ int) error { return closeHandle(queryHandle) })
	get := func() (*facetpb.GetResponse, error) {
		return client.Facet().Get(ctx, &facetpb.GetRequest{Access: access, TypeRef: typ.Bytes()})
	}
	measure("Facet Get", nil, func(_ int) error {
		out, err := get()
		if err == nil && out.GetResult() == nil {
			return fmt.Errorf("no Facet Get result: %v", out)
		}
		return err
	}, nil)
	var previous *facetpb.GetResponse
	measure("Facet Put", func(_ int) error {
		var err error
		previous, err = get()
		if err != nil || previous.GetResult() == nil {
			return fmt.Errorf("preceding Get: %v %v", previous, err)
		}
		return nil
	}, func(i int) error {
		out, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Revision{Revision: previous.GetResult().Revision}},
			OperationId: []byte(fmt.Sprintf("p8-measure-put-%d", i)), Payload: []byte(fmt.Sprintf("p8-value-%d", i))})
		if err == nil && out.GetResult() == nil {
			return fmt.Errorf("no Facet Put result: %v", out)
		}
		return err
	}, nil)
	previous, err := get()
	if err != nil || previous.GetResult() == nil {
		t.Fatalf("capacity preparation: %v %v", previous, err)
	}
	maximum := bytes.Repeat([]byte{0xab}, 1<<20)
	large, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Revision{Revision: previous.GetResult().Revision}},
		OperationId: []byte("p8-one-mib"), Payload: maximum})
	if err != nil || large.GetResult() == nil {
		t.Fatalf("one MiB payload: %v %v", large, err)
	}
	readLarge, err := get()
	if err != nil || !bytes.Equal(readLarge.GetResult().Payload, maximum) {
		t.Fatalf("one MiB roundtrip: %v %v", readLarge, err)
	}
	tooLarge, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Revision{Revision: readLarge.GetResult().Revision}},
		OperationId: []byte("p8-over-one-mib"), Payload: make([]byte, 1<<20+1)})
	if err != nil && status.Code(err) != codes.ResourceExhausted || err == nil && tooLarge.GetFailure() != netpb.Failure_RESOURCE_EXHAUSTED {
		t.Fatalf("over one MiB accepted: %v %v", tooLarge, err)
	}
	restored, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: typ.Bytes(), Expected: &facetpb.Expected{Condition: &facetpb.Expected_Revision{Revision: readLarge.GetResult().Revision}},
		OperationId: []byte("p8-restore-payload"), Payload: []byte(`"network"`)})
	if err != nil || restored.GetResult() == nil {
		t.Fatalf("restore payload: %v %v", restored, err)
	}
	for i := 0; i < 255; i++ {
		other := fixtureType(fmt.Sprintf("p8-type-%d", i))
		out, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: other.Bytes(),
			Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte(fmt.Sprintf("p8-type-%d", i)), Payload: []byte{byte(i)}})
		if err != nil || out.GetResult() == nil {
			t.Fatalf("type %d of 256: %v %v", i+2, out, err)
		}
	}
	listed, err := client.Facet().ListTypes(ctx, &facetpb.ListTypesRequest{Access: access})
	if err != nil || listed.GetResult() == nil || len(listed.GetResult().Types) != 256 {
		t.Fatalf("256 types not retained: %v %v", listed, err)
	}
	extra := fixtureType("p8-type-255")
	overTypes, err := client.Facet().Put(ctx, &facetpb.PutRequest{Access: access, TypeRef: extra.Bytes(),
		Expected: &facetpb.Expected{Condition: &facetpb.Expected_Absent{Absent: true}}, OperationId: []byte("p8-over-256-types"), Payload: []byte("extra")})
	if err != nil && status.Code(err) != codes.ResourceExhausted || err == nil && overTypes.GetFailure() != netpb.Failure_RESOURCE_EXHAUSTED {
		t.Fatalf("257th type accepted: %v %v", overTypes, err)
	}
	for name, path := range map[string]string{"core": filepath.Join(c.dir, "core.db"), "core-realm": filepath.Join(c.dir, "realm.db"),
		"facet": filepath.Join(facetDir, "facet.db"), "facet-realm": filepath.Join(facetDir, "realm.db")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("size %s: %v", name, err)
		}
		t.Logf("P8 Store %s=%d bytes", name, info.Size())
	}
}
