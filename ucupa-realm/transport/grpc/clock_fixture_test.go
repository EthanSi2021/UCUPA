package realmgrpc

import (
	"sync"
	"time"

	"ucupa/shared/values"
)

// fakeClock is the deterministic white-box seam for expiry and lease floors.
// Production always runs on time.Now; only these in-package tests move it.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Now()} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

const handleTestOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000001"
const handleOtherOwner values.FactAuthorityRef = "00000000-0000-4000-8000-000000000002"

func handleTestCaller(principal values.PrincipalID) values.CallerContext {
	return values.CallerContext{Subject: values.PrincipalRef{Realm: "realm-a", Principal: principal}}
}
