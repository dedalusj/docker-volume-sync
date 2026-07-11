package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type GlobalConfig struct {
	DestinationPath string
	Location        *time.Location
	Compression     bool
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
	// Compression is nil when the container carries no compression label, in
	// which case the global default applies. See ResolveCompression.
	Compression *bool
	// Include and Exclude are rclone glob patterns. Excludes take precedence
	// over includes, and any include restricts the sync to matching paths.
	Include []string
	Exclude []string
}

// ResolveCompression reports whether compression is enabled for a job, falling
// back to the global default when the job carries no label override.
func (g *GlobalConfig) ResolveCompression(job VolumeJob) bool {
	if job.Compression != nil {
		return *job.Compression
	}
	return g.Compression
}

func LoadGlobal() (*GlobalConfig, error) {
	dest := os.Getenv("DESTINATION_PATH")
	if dest == "" {
		return nil, fmt.Errorf("DESTINATION_PATH environment variable is required")
	}

	loc := time.Local
	if tz := os.Getenv("TZ"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	return &GlobalConfig{
		DestinationPath: dest,
		Location:        loc,
		Compression:     os.Getenv("COMPRESSION") == "true",
	}, nil
}

const (
	labelPrefix = "volumesync"

	enabledLabel         = labelPrefix + ".enabled"
	volumeLabel          = labelPrefix + ".volume"
	scheduleLabel        = labelPrefix + ".schedule"
	deleteLabel          = labelPrefix + ".delete"
	concurrencyLabel     = labelPrefix + ".concurrency"
	stopLabel            = labelPrefix + ".stop"
	stopGracePeriodLabel = labelPrefix + ".stop_grace_period"
	subPathLabel         = labelPrefix + ".subpath"
	uidLabel             = labelPrefix + ".uid"
	gidLabel             = labelPrefix + ".gid"
	compressionLabel     = labelPrefix + ".compression"
	includeLabel         = labelPrefix + ".include"
	excludeLabel         = labelPrefix + ".exclude"

	// patternSeparator splits pattern lists. Not a comma: rclone globs use
	// commas for brace alternation, as in *.{jpg,png}.
	patternSeparator = ";"
)

// parsePatterns splits a pattern label into its individual glob patterns,
// trimming whitespace and dropping empty entries.
func parsePatterns(value string) []string {
	var patterns []string
	for _, p := range strings.Split(value, patternSeparator) {
		if p = strings.TrimSpace(p); p != "" {
			patterns = append(patterns, p)
		}
	}
	return patterns
}

func ParseLabels(labels map[string]string) (*VolumeJob, error) {
	if labels[enabledLabel] != "true" {
		return nil, nil
	}

	volume := labels[volumeLabel]
	if volume == "" {
		return nil, fmt.Errorf("%s is required", volumeLabel)
	}

	schedule := labels[scheduleLabel]
	if schedule == "" {
		return nil, fmt.Errorf("%s is required", scheduleLabel)
	}

	job := &VolumeJob{
		VolumeName:    volume,
		Schedule:      schedule,
		Delete:        labels[deleteLabel] == "true",
		Concurrency:   16,
		StopContainer: true,
		SubPath:       volume,
	}

	if labels[stopLabel] == "false" {
		job.StopContainer = false
	}

	if grace := labels[stopGracePeriodLabel]; grace != "" {
		d, err := time.ParseDuration(grace)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", stopGracePeriodLabel, err)
		}
		job.StopGracePeriod = d
	} else {
		job.StopGracePeriod = 30 * time.Second
	}

	if cStr := labels[concurrencyLabel]; cStr != "" {
		c, err := strconv.Atoi(cStr)
		if err == nil && c > 0 {
			job.Concurrency = c
		}
	}

	if sub := labels[subPathLabel]; sub != "" {
		job.SubPath = sub
	}

	if uidStr := labels[uidLabel]; uidStr != "" {
		uid, err := strconv.Atoi(uidStr)
		if err == nil {
			job.UID = &uid
		}
	}

	if gidStr := labels[gidLabel]; gidStr != "" {
		gid, err := strconv.Atoi(gidStr)
		if err == nil {
			job.GID = &gid
		}
	}

	// Only set when the label is present, so an absent label inherits the
	// global default while an explicit "false" can disable it.
	if c, ok := labels[compressionLabel]; ok {
		compression := c == "true"
		job.Compression = &compression
	}

	job.Include = parsePatterns(labels[includeLabel])
	job.Exclude = parsePatterns(labels[excludeLabel])

	return job, nil
}
