package commit

import (
	"fmt"

	"ucupa/shared/values"
)

// CorruptProjectionParentForTest injects only the three original faults into
// the fixed projection fixture. It is absent from production builds and does
// not expose an arbitrary target, mutation callback or mutable owner state.
func CorruptProjectionParentForTest(owner *Owner, fault string) error {
	owner.Lock()
	defer owner.Unlock()
	middle := values.ElementID("20000000-0000-4000-8000-000000000002")
	anchor, ok := owner.state.elements.Anchor(middle)
	if !ok {
		return fmt.Errorf("projection fixture middle is absent")
	}
	switch fault {
	case "missing parent":
		anchor.Parent = values.ElementID("20000000-0000-4000-8000-0000000003e7")
	case "cycle":
		anchor.Parent = middle
	case "non-canonical parent":
		anchor.Parent = values.ElementID("not-a-canonical-element-id")
	default:
		return fmt.Errorf("unknown projection parent fault %q", fault)
	}
	return owner.state.elements.RestoreAnchor(anchor)
}
