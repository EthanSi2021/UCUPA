package registry_test

import (
	"errors"
	"testing"

	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
	"ucupa/ucupa-core/internal/access/registry"
)

func TestPublicFiniteQueueExpiryAndExecutionLimits(t *testing.T) {
	c, h, p := newResourceFixture(t)
	if !registry.SetPublicResourceScenarioForTest(c.resources, "queue-one") {
		t.Fatal("queue fault missing")
	}
	for _, n := range []uint64{91501, 91502} {
		c.create(t, n, contract.OperationID(string(resourceID(n))))
	}
	if _, err := c.call(t, map[string]any{"op": "begin-observe", "preparation": p}); !errors.Is(err, values.PublicGap) {
		t.Fatal(err)
	}
	if _, err := c.elements.ReadElement(resourceActor, resourceID(91502), nil); err != nil {
		t.Fatal("observer overflow blocked facts", err)
	}
	release := c.occupySlots(t)
	if _, err := c.call(t, map[string]any{"op": "list", "handle": h, "address": map[string]any{"down": []any{}}}); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal(err)
	}
	release()
	if !registry.SetPublicResourceScenarioForTest(c.resources, "expired") {
		t.Fatal("expiry fault missing")
	}
	if _, err := c.call(t, map[string]any{"op": "begin-observe", "preparation": p}); !errors.Is(err, values.PublicGap) {
		t.Fatal(err)
	}
	if _, err := c.call(t, map[string]any{"op": "list", "handle": h, "address": map[string]any{"down": []any{}}}); !errors.Is(err, values.PublicInvalidContext) {
		t.Fatal(err)
	}
	if contexts, bytes := registry.PublicResourceCountsForTest(c.resources); contexts != 0 || bytes != 0 {
		t.Fatal("expiration leaked contexts", contexts, bytes)
	}
}

func TestPublicMemoryAdmissionDoesNotReplaceExistingOwnerOperations(t *testing.T) {
	c, h, _ := newResourceFixture(t)
	if !registry.SetPublicResourceScenarioForTest(c.resources, "working-memory") {
		t.Fatal("memory fault missing")
	}
	if _, err := c.call(t, map[string]any{"op": "list", "handle": h, "address": map[string]any{"down": []any{}}}); !errors.Is(err, values.PublicResourceExhausted) {
		t.Fatal(err)
	}
	c.create(t, 91503, "ordinary-write")
}
