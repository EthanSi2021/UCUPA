package exchange

import "sync"

// Cleanup belongs to one trusted-local invocation. It can only dispose the
// exact resources registered by the connected owner; it cannot read them.
type Cleanup struct {
	mu       sync.Mutex
	closed   bool
	keepPins bool
	items    []CleanupItem
}

type CleanupItem struct {
	Finish  func()
	Discard func()
}

func (cleanup *Cleanup) Add(item CleanupItem) bool {
	cleanup.mu.Lock()
	if cleanup.closed {
		cleanup.mu.Unlock()
		item.Finish()
		if item.Discard != nil {
			item.Discard()
		}
		return false
	}
	cleanup.items = append(cleanup.items, item)
	cleanup.mu.Unlock()
	return true
}

func (cleanup *Cleanup) PreservePins() {
	cleanup.mu.Lock()
	cleanup.keepPins = true
	cleanup.mu.Unlock()
}

func (cleanup *Cleanup) Close() {
	cleanup.mu.Lock()
	if cleanup.closed {
		cleanup.mu.Unlock()
		return
	}
	cleanup.closed = true
	items, keep := cleanup.items, cleanup.keepPins
	cleanup.items = nil
	cleanup.mu.Unlock()
	for _, item := range items {
		item.Finish()
		if !keep && item.Discard != nil {
			item.Discard()
		}
	}
}
