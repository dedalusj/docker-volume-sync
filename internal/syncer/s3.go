package syncer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type ObjectInfo struct {
	Key     string
	Size    int64
	ModTime time.Time
}

// ListS3Objects lists all objects in a bucket with a given prefix.
// It returns a map where the keys are the relative paths (prefix removed).
func ListS3Objects(ctx context.Context, client S3Client, bucket, prefix string, filter func(string) bool) (map[string]ObjectInfo, error) {
	objs := make(map[string]ObjectInfo)

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &prefix,
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)

			// Calculate relative path
			relPath := key
			if prefix != "" {
				if !strings.HasPrefix(key, prefix) {
					continue // Should not happen given the input prefix, but good to be safe
				}
				relPath = strings.TrimPrefix(key, prefix)
				relPath = strings.TrimPrefix(relPath, "/") // Ensure no leading slash
			}

			if relPath == "" {
				continue // Skip the directory itself if it appears as an object
			}

			// Apply filter if provided
			if filter != nil && !filter(relPath) {
				continue
			}

			objs[relPath] = ObjectInfo{
				Key:     key,
				Size:    aws.ToInt64(obj.Size),
				ModTime: aws.ToTime(obj.LastModified),
			}
		}
	}

	return objs, nil
}
