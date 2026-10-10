package access

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestRetiredEvidenceCleanupIsOnceAndOutsideFactLock(t *testing.T) {
	for _, mode := range []string{"release", "expire", "close"} {
		t.Run(mode, func(t *testing.T) {
			m, locker, clock := newTestManager(testLimits())
			var released atomic.Int64
			locker.Lock()
			c, err := m.IssueLocked(testCaller("caller", 1), "observation-preparation", nil)
			if err != nil {
				t.Fatal(err)
			}
			c.Release = func(ctx context.Context) {
				if ctx.Err() != nil || !locker.TryLock() {
					t.Error("cleanup waited under fact lock or inherited cancellation")
					return
				}
				locker.Unlock()
				released.Add(1)
			}
			switch mode {
			case "release":
				m.RemoveLocked(c.Token.Value)
			case "expire":
				clock.now = c.Expires
				m.ExpireLocked()
			case "close":
				m.CloseLocked()
			}
			if released.Load() != 0 {
				t.Error("remote release ran during locked retirement")
			}
			m.RemoveLocked(c.Token.Value)
			locker.Unlock()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			m.DrainRetired(ctx)
			m.DrainRetired(ctx)
			if released.Load() != 1 {
				t.Fatalf("evidence released %d times", released.Load())
			}
		})
	}
}
