package acceptance_test

import (
	"context"
	"google.golang.org/protobuf/proto"
	"sort"
	"testing"
	"time"
	networkpb "ucupa/shared/bindings/go/protobuf"
	corepb "ucupa/ucupa-core/bindings/go/protobuf"
)

// A repeatable small-load measurement, not a throughput acceptance threshold or
// a comparison with another transport. Each read still creates its real receipt.
func TestNetworkReadMeasurement(t *testing.T) {
	f := newNetworkFixture(t, true)
	request := &corepb.ReadElementRequest{Handle: f.enter(t, corepb.Intent_READ), Address: &networkpb.Address{}}
	const count = 24
	latencies := make([]time.Duration, count)
	bytes := 0
	start := time.Now()
	for i := range latencies {
		callStart := time.Now()
		result, err := f.client.Core().ReadElement(context.Background(), request)
		if err != nil || result.GetResult() == nil {
			t.Fatalf("read: %v %v", result, err)
		}
		latencies[i] = time.Since(callStart)
		bytes += proto.Size(result)
	}
	elapsed := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("loopback TLS, one durable Core, one sequential client, %d root reads; request=%d B mean-response=%d B; p50=%s p95=%s total=%s rate=%.1f RPC/s; not a production target", count, proto.Size(request), bytes/count, latencies[count/2], latencies[count*95/100], elapsed, count/elapsed.Seconds())
}
