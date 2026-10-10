package query

import (
	"time"
	"ucupa/shared/values"
	"ucupa/ucupa-core/contract"
)

type Settings struct {
	Public     values.PublicLimits
	Retention  time.Duration
	Handles    uint32
	Bytes      uint64
	Concurrent uint32
}

func NormalizeConfig(config contract.Config) (Settings, error) {
	settings := Settings{Handles: config.SnapshotCapacity, Bytes: config.SnapshotBytes, Concurrent: config.ConcurrentReads}
	var err error
	settings.Public, err = values.NormalizePublicPolicy(config.PublicAccessPolicy)
	if err != nil {
		return Settings{}, contract.ErrInvalid
	}
	if config.SnapshotRetention != nil {
		settings.Retention = *config.SnapshotRetention
		if settings.Retention <= 0 {
			return Settings{}, contract.ErrInvalid
		}
	}
	if settings.Handles == 0 {
		settings.Handles = 128
	}
	if settings.Bytes == 0 {
		settings.Bytes = 64 << 20
	}
	if settings.Concurrent == 0 {
		settings.Concurrent = 64
	}
	return settings, nil
}
