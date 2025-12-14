package syncer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"
)

type MockS3Client struct {
	ListObjectsV2Func func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	PutObjectFunc     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObjectFunc  func(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	GetObjectFunc     func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (m *MockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.ListObjectsV2Func != nil {
		return m.ListObjectsV2Func(ctx, params, optFns...)
	}
	return &s3.ListObjectsV2Output{}, nil
}

func (m *MockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.PutObjectFunc != nil {
		return m.PutObjectFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *MockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.DeleteObjectFunc != nil {
		return m.DeleteObjectFunc(ctx, params, optFns...)
	}
	return &s3.DeleteObjectOutput{}, nil
}

func (m *MockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.GetObjectFunc != nil {
		return m.GetObjectFunc(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{}, nil
}

func (m *MockS3Client) UploadPart(ctx context.Context, params *s3.UploadPartInput, optFns ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	return &s3.UploadPartOutput{}, nil
}

func (m *MockS3Client) CreateMultipartUpload(ctx context.Context, params *s3.CreateMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	return &s3.CreateMultipartUploadOutput{}, nil
}

func (m *MockS3Client) CompleteMultipartUpload(ctx context.Context, params *s3.CompleteMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (m *MockS3Client) AbortMultipartUpload(ctx context.Context, params *s3.AbortMultipartUploadInput, optFns ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, nil
}

func TestSync_S3ToLocal(t *testing.T) {
	tmpDir := t.TempDir()

	mockClient := &MockS3Client{
		ListObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: []types.Object{
					{Key: aws.String("key.txt"), Size: aws.Int64(12), LastModified: aws.Time(time.Now())},
				},
			}, nil
		},
		GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				ContentLength: aws.Int64(12),
				Body:          io.NopCloser(strings.NewReader("hello world!")),
			}, nil
		},
	}

	s, err := New(context.Background(), WithClient(mockClient))
	require.NoError(t, err)

	err = s.Sync(context.Background(), "s3://bucket", tmpDir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpDir, "key.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello world!", string(content))
}

func TestSync_LocalToS3(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "upload.txt"), []byte("upload me"), 0644)

	var uploadedKey string
	mockClient := &MockS3Client{
		ListObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{}, nil
		},
		PutObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			uploadedKey = *params.Key
			return &s3.PutObjectOutput{}, nil
		},
	}

	s, err := New(context.Background(), WithClient(mockClient))
	require.NoError(t, err)

	err = s.Sync(context.Background(), tmpDir, "s3://bucket")
	require.NoError(t, err)

	require.Equal(t, "upload.txt", uploadedKey)
}

func TestSync_S3ToLocal_MultipleFilesWithFilter(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup mock objects: mix of files to stay and files to be filtered
	s3Objects := []types.Object{
		{Key: aws.String("keep1.txt"), Size: aws.Int64(10), LastModified: aws.Time(time.Now())},
		{Key: aws.String("keep2.txt"), Size: aws.Int64(20), LastModified: aws.Time(time.Now())},
		{Key: aws.String("ignore1.tmp"), Size: aws.Int64(30), LastModified: aws.Time(time.Now())},
		{Key: aws.String("sub/keep3.txt"), Size: aws.Int64(40), LastModified: aws.Time(time.Now())},
		{Key: aws.String("sub/ignore2.tmp"), Size: aws.Int64(50), LastModified: aws.Time(time.Now())},
	}

	mockClient := &MockS3Client{
		ListObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: s3Objects,
			}, nil
		},
		GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				ContentLength: aws.Int64(10),
				Body:          io.NopCloser(strings.NewReader("content")),
			}, nil
		},
	}

	filter := func(path string) bool {
		return !strings.HasSuffix(path, ".tmp")
	}

	s, err := New(context.Background(), WithClient(mockClient), WithFilter(filter), WithConcurrency(5))
	require.NoError(t, err)

	err = s.Sync(context.Background(), "s3://bucket", tmpDir)
	require.NoError(t, err)

	expectedFiles := []string{
		"keep1.txt",
		"keep2.txt",
		"sub/keep3.txt",
	}
	for _, f := range expectedFiles {
		_, err := os.Stat(filepath.Join(tmpDir, f))
		require.NoError(t, err, "File %s should exist", f)
	}

	ignoredFiles := []string{
		"ignore1.tmp",
		"sub/ignore2.tmp",
	}
	for _, f := range ignoredFiles {
		_, err := os.Stat(filepath.Join(tmpDir, f))
		require.True(t, os.IsNotExist(err), "File %s should not exist", f)
	}
}

func TestSync_S3ToLocal_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	expectedError := fmt.Errorf("simulated download error")

	// Setup mock objects: multiple files, one will fail
	s3Objects := []types.Object{
		{Key: aws.String("file1.txt"), Size: aws.Int64(10), LastModified: aws.Time(time.Now())},
		{Key: aws.String("error.txt"), Size: aws.Int64(10), LastModified: aws.Time(time.Now())},
		{Key: aws.String("file2.txt"), Size: aws.Int64(10), LastModified: aws.Time(time.Now())},
	}

	mockClient := &MockS3Client{
		ListObjectsV2Func: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents: s3Objects,
			}, nil
		},
		GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if *params.Key == "error.txt" {
				return nil, expectedError
			}
			return &s3.GetObjectOutput{
				ContentLength: aws.Int64(10),
				Body:          io.NopCloser(strings.NewReader("content")),
			}, nil
		},
	}

	// Use concurrency > 1 to ensure that errors are propagated correctly even with multiple goroutines
	s, err := New(context.Background(), WithClient(mockClient), WithConcurrency(4))
	require.NoError(t, err)

	err = s.Sync(context.Background(), "s3://bucket", tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), expectedError.Error())
}
