package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	S3Path                string
	VolumeName            string
	VolumePath            string
	SyncSchedule          string
	DockerStopGracePeriod time.Duration
	DeleteDestination     bool
	Concurrency           int
}

func Load() (*Config, error) {
	cfg := &Config{
		S3Path:       os.Getenv("S3_PATH"),
		VolumePath:   os.Getenv("VOLUME_PATH"),
		SyncSchedule: os.Getenv("SYNC_SCHEDULE"),
		VolumeName:   os.Getenv("VOLUME_NAME"),
	}

	if cfg.S3Path == "" {
		return nil, fmt.Errorf("S3_PATH environment variable is required")
	}
	if cfg.SyncSchedule == "" {
		return nil, fmt.Errorf("SYNC_SCHEDULE environment variable is required")
	}

	if cfg.VolumePath == "" {
		cfg.VolumePath = "/data"
	}

	gracePeriodStr := os.Getenv("DOCKER_STOP_GRACE_PERIOD")
	if gracePeriodStr == "" {
		cfg.DockerStopGracePeriod = 2 * time.Minute
	} else {
		d, err := time.ParseDuration(gracePeriodStr)
		if err != nil {
			return nil, fmt.Errorf("invalid DOCKER_STOP_GRACE_PERIOD: %w", err)
		}
		cfg.DockerStopGracePeriod = d
	}

	if os.Getenv("SYNC_DELETE") == "true" {
		cfg.DeleteDestination = true
	}

	cfg.Concurrency = 16
	if concurrencyStr := os.Getenv("SYNC_CONCURRENCY"); concurrencyStr != "" {
		var c int
		if _, err := fmt.Sscanf(concurrencyStr, "%d", &c); err == nil && c > 0 {
			cfg.Concurrency = c
		}
	}

	return cfg, nil
}
