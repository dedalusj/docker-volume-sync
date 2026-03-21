package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type GlobalConfig struct {
	DestinationPath string
}

type VolumeJob struct {
	VolumeName      string
	Schedule        string
	Delete          bool
	Concurrency     int
	StopContainer   bool
	StopGracePeriod time.Duration
	SubPath         string
	ContainerIDs    []string
	UID             *int
	GID             *int
}

func LoadGlobal() (*GlobalConfig, error) {
	dest := os.Getenv("DESTINATION_PATH")
	if dest == "" {
		return nil, fmt.Errorf("DESTINATION_PATH environment variable is required")
	}

	return &GlobalConfig{
		DestinationPath: dest,
	}, nil
}

func ParseLabels(labels map[string]string) (*VolumeJob, error) {
	if labels["volumesync.enabled"] != "true" {
		return nil, fmt.Errorf("volumesync.enabled is not true")
	}

	volume := labels["volumesync.volume"]
	if volume == "" {
		return nil, fmt.Errorf("volumesync.volume is required")
	}

	schedule := labels["volumesync.schedule"]
	if schedule == "" {
		return nil, fmt.Errorf("volumesync.schedule is required")
	}

	job := &VolumeJob{
		VolumeName:    volume,
		Schedule:      schedule,
		Delete:        labels["volumesync.delete"] == "true",
		Concurrency:   16,
		StopContainer: true,
		SubPath:       volume,
	}

	if labels["volumesync.stop"] == "false" {
		job.StopContainer = false
	}

	if grace := labels["volumesync.stop_grace_period"]; grace != "" {
		d, err := time.ParseDuration(grace)
		if err != nil {
			return nil, fmt.Errorf("invalid volumesync.stop_grace_period: %w", err)
		}
		job.StopGracePeriod = d
	} else {
		job.StopGracePeriod = 30 * time.Second
	}

	if cStr := labels["volumesync.concurrency"]; cStr != "" {
		c, err := strconv.Atoi(cStr)
		if err == nil && c > 0 {
			job.Concurrency = c
		}
	}

	if sub := labels["volumesync.subpath"]; sub != "" {
		job.SubPath = sub
	}
	
	if uidStr := labels["volumesync.uid"]; uidStr != "" {
		uid, err := strconv.Atoi(uidStr)
		if err == nil {
			job.UID = &uid
		}
	}
	
	if gidStr := labels["volumesync.gid"]; gidStr != "" {
		gid, err := strconv.Atoi(gidStr)
		if err == nil {
			job.GID = &gid
		}
	}

	return job, nil
}
