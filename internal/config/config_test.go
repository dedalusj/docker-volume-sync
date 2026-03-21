package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int { return &i }

func TestLoadGlobal(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *GlobalConfig
		wantErr bool
	}{
		{
			name: "Success",
			env: map[string]string{
				"DESTINATION_PATH": "s3://my-bucket/path",
			},
			want: &GlobalConfig{
				DestinationPath: "s3://my-bucket/path",
			},
			wantErr: false,
		},
		{
			name:    "MissingDestinationPath",
			env:     map[string]string{},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := LoadGlobal()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name    string
		labels  map[string]string
		want    *VolumeJob
		wantErr bool
	}{
		{
			name: "SuccessFull",
			labels: map[string]string{
				"volumesync.enabled":           "true",
				"volumesync.volume":            "my-vol",
				"volumesync.schedule":          "0 0 * * *",
				"volumesync.delete":            "true",
				"volumesync.concurrency":       "4",
				"volumesync.stop_grace_period": "1m",
				"volumesync.subpath":           "custom/path",
				"volumesync.uid":               "1000",
				"volumesync.gid":               "1000",
			},
			want: &VolumeJob{
				VolumeName:      "my-vol",
				Schedule:        "0 0 * * *",
				Delete:          true,
				Concurrency:     4,
				StopContainer:   true,
				StopGracePeriod: time.Minute,
				SubPath:         "custom/path",
				UID:             intPtr(1000),
				GID:             intPtr(1000),
			},
			wantErr: false,
		},
		{
			name: "Defaults",
			labels: map[string]string{
				"volumesync.enabled":  "true",
				"volumesync.volume":   "my-vol",
				"volumesync.schedule": "@daily",
			},
			want: &VolumeJob{
				VolumeName:      "my-vol",
				Schedule:        "@daily",
				Delete:          false,
				Concurrency:     16,
				StopContainer:   true,
				StopGracePeriod: 30 * time.Second,
				SubPath:         "my-vol",
			},
			wantErr: false,
		},
		{
			name: "NotEnabled",
			labels: map[string]string{
				"volumesync.enabled": "false",
			},
			wantErr: false,
		},
		{
			name: "MissingVolume",
			labels: map[string]string{
				"volumesync.enabled":  "true",
				"volumesync.schedule": "@daily",
			},
			wantErr: true,
		},
		{
			name: "InvalidGracePeriod",
			labels: map[string]string{
				"volumesync.enabled":           "true",
				"volumesync.volume":            "vol",
				"volumesync.schedule":          "@daily",
				"volumesync.stop_grace_period": "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLabels(tt.labels)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
