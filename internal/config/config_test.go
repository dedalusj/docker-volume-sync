package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *Config
		wantErr bool
	}{
		{
			name: "SuccessDefaults",
			env: map[string]string{
				"S3_PATH":       "s3://my-bucket/path",
				"SYNC_SCHEDULE": "@every 5m",
			},
			want: &Config{
				S3Path:                "s3://my-bucket/path",
				SyncSchedule:          "@every 5m",
				VolumePath:            "/data",
				DockerStopGracePeriod: 2 * time.Minute,
				Concurrency:           16,
				DeleteDestination:     false,
			},
			wantErr: false,
		},
		{
			name: "SuccessFull",
			env: map[string]string{
				"S3_PATH":                  "s3://other-bucket",
				"VOLUME_NAME":              "my-vol",
				"VOLUME_PATH":              "/app/data",
				"SYNC_SCHEDULE":            "0 0 * * *",
				"DOCKER_STOP_GRACE_PERIOD": "30s",
				"SYNC_DELETE":              "true",
				"SYNC_CONCURRENCY":         "4",
			},
			want: &Config{
				S3Path:                "s3://other-bucket",
				VolumeName:            "my-vol",
				VolumePath:            "/app/data",
				SyncSchedule:          "0 0 * * *",
				DockerStopGracePeriod: 30 * time.Second,
				DeleteDestination:     true,
				Concurrency:           4,
			},
			wantErr: false,
		},
		{
			name: "MissingS3Path",
			env: map[string]string{
				"SYNC_SCHEDULE": "@every 1m",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "MissingSyncSchedule",
			env: map[string]string{
				"S3_PATH": "s3://bucket",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "InvalidGracePeriod",
			env: map[string]string{
				"S3_PATH":                  "s3://bucket",
				"SYNC_SCHEDULE":            "@every 1m",
				"DOCKER_STOP_GRACE_PERIOD": "invalid",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "InvalidConcurrency",
			env: map[string]string{
				"S3_PATH":          "s3://bucket",
				"SYNC_SCHEDULE":    "@every 1m",
				"SYNC_CONCURRENCY": "not-a-number",
			},
			// Expect defaults for concurrency if invalid
			want: &Config{
				S3Path:                "s3://bucket",
				SyncSchedule:          "@every 1m",
				VolumePath:            "/data",
				DockerStopGracePeriod: 2 * time.Minute,
				Concurrency:           16,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env before each test
			os.Clearenv()

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := Load()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
