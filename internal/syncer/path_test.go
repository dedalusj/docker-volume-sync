package syncer

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestJoinPath(t *testing.T) {
	tests := []struct {
		base     string
		sub      string
		expected string
	}{
		{"s3://bucket", "radarr", "s3:bucket/radarr"},
		{"s3:bucket", "radarr", "s3:bucket/radarr"},
		{"s3:bucket/", "radarr", "s3:bucket/radarr"},
		{"s3:bucket", "/radarr", "s3:bucket/radarr"},
		{"s3:", "bucket/radarr", "s3:bucket/radarr"},
		{"/local/path", "sub", "/local/path/sub"},
		{"local/path", "sub", "local/path/sub"},
	}

	for _, tt := range tests {
		t.Run(tt.base+"+"+tt.sub, func(t *testing.T) {
			assert.Equal(t, tt.expected, JoinPath(tt.base, tt.sub))
		})
	}
}
