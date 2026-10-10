package contract

import (
	"errors"
	"time"
	"ucupa/shared/values"
)

var ErrSnapshotCapacity = errors.New("Core read snapshot capacity exhausted")

// Config fixes this Core run's read policy. A nil SnapshotRetention disables
// cross-request retention; a supplied duration must be positive. Its value is
// copied, so changing the caller's pointer cannot change a running Core.
// Zero resource limits select the finite engineering defaults below, not a TTL.
// ExpectedRealm is the open-time assembly assertion: it must equal the Realm of
// the durable contact in the original Store and never overrides it.
type Config struct {
	Writer             *values.WriterStartup
	PublicAccessPolicy *values.PublicAccessPolicy
	ExpectedRealm      values.RealmID
	SnapshotRetention  *time.Duration
	SnapshotCapacity   uint32
	SnapshotBytes      uint64
	ConcurrentReads    uint32
}
