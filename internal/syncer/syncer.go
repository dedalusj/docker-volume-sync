package syncer

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Client interface {
	s3.ListObjectsV2APIClient
	manager.DownloadAPIClient
	manager.UploadAPIClient
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type Syncer struct {
	client            S3Client
	deleteDestination bool
	concurrency       int
	filter            func(string) bool
}

type Option func(*Syncer)

func WithFilter(f func(string) bool) Option {
	return func(s *Syncer) {
		s.filter = f
	}
}

func WithDelete(delete bool) Option {
	return func(s *Syncer) {
		s.deleteDestination = delete
	}
}

func WithConcurrency(n int) Option {
	return func(s *Syncer) {
		s.concurrency = n
	}
}

func WithClient(client S3Client) Option {
	return func(s *Syncer) {
		s.client = client
	}
}

func New(ctx context.Context, opts ...Option) (*Syncer, error) {
	sdkConfig, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load default aws config: %w", err)
	}

	s := &Syncer{
		concurrency: 16,
		client:      s3.NewFromConfig(sdkConfig),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Syncer) Sync(ctx context.Context, src, dst string) error {
	log.Printf("Syncing %s -> %s", src, dst)

	srcIsS3 := strings.HasPrefix(src, "s3://")
	dstIsS3 := strings.HasPrefix(dst, "s3://")

	if srcIsS3 && !dstIsS3 {
		return s.syncS3ToLocal(ctx, src, dst)
	} else if !srcIsS3 && dstIsS3 {
		return s.syncLocalToS3(ctx, src, dst)
	} else {
		return fmt.Errorf("unsupported sync mode: %s -> %s (only S3<->Local supported)", src, dst)
	}
}

func (s *Syncer) syncS3ToLocal(ctx context.Context, src, dst string) error {
	bucket, prefix := parseS3URI(src)

	srcObjs, err := ListS3Objects(ctx, s.client, bucket, prefix, s.filter)
	if err != nil {
		return err
	}

	dstObjs, err := WalkLocal(dst, s.filter)
	if err != nil {
		return err
	}

	downloader := manager.NewDownloader(s.client)
	downloader.Concurrency = s.concurrency

	return s.executeSync(ctx, srcObjs, dstObjs,
		func(ctx context.Context, relPath string) error {
			key := filepath.Join(prefix, relPath)
			key = strings.TrimPrefix(key, "/")

			localPath := filepath.Join(dst, relPath)

			// Ensure dir exists
			if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
				return err
			}

			// Create file
			f, err := os.Create(localPath)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err = downloader.Download(ctx, f, &s3.GetObjectInput{
				Bucket: &bucket,
				Key:    &key,
			}); err != nil {
				return err
			}

			fmt.Printf("Downloaded %s -> %s\n", key, localPath)
			return nil
		},
		func(ctx context.Context, relPath string) error {
			if err := os.Remove(filepath.Join(dst, relPath)); err != nil {
				return err
			}
			fmt.Printf("Deleted %s\n", filepath.Join(dst, relPath))
			return nil
		},
	)
}

func (s *Syncer) syncLocalToS3(ctx context.Context, src, dst string) error {
	bucket, prefix := parseS3URI(dst)

	srcObjs, err := WalkLocal(src, s.filter)
	if err != nil {
		return err
	}

	dstObjs, err := ListS3Objects(ctx, s.client, bucket, prefix, s.filter)
	if err != nil {
		return err
	}

	uploader := manager.NewUploader(s.client)
	uploader.Concurrency = s.concurrency

	return s.executeSync(ctx, srcObjs, dstObjs,
		func(ctx context.Context, relPath string) error {
			localPath := filepath.Join(src, relPath)
			key := filepath.Join(prefix, relPath)
			key = strings.TrimPrefix(key, "/")

			f, err := os.Open(localPath)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err = uploader.Upload(ctx, &s3.PutObjectInput{
				Bucket: &bucket,
				Key:    &key,
				Body:   f,
			}); err != nil {
				return err
			}
			fmt.Printf("Uploaded %s -> %s\n", localPath, key)
			return nil
		},
		func(ctx context.Context, relPath string) error {
			key := filepath.Join(prefix, relPath)
			key = strings.TrimPrefix(key, "/")
			if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: &bucket,
				Key:    &key,
			}); err != nil {
				return err
			}
			fmt.Printf("Deleted %s\n", key)
			return nil
		},
	)
}

func (s *Syncer) executeSync(ctx context.Context, srcObjs, dstObjs map[string]ObjectInfo, copyFunc, deleteFunc func(context.Context, string) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan job, 1000)
	errChan := make(chan error, 2000)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < s.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-jobs:
					if !ok {
						return
					}
					if err := j.exec(ctx); err != nil {
						select {
						case errChan <- err:
						default:
						}
						cancel()
						return
					}
				}
			}
		}()
	}

	submit := func(j job) bool {
		select {
		case jobs <- j:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Submit copy jobs
	for path, srcInfo := range srcObjs {
		if ctx.Err() != nil {
			break
		}
		dstInfo, exists := dstObjs[path]

		shouldCopy := !exists
		if exists {
			// Check size and modtime
			if srcInfo.Size != dstInfo.Size {
				shouldCopy = true
			} else {
				if srcInfo.ModTime.After(dstInfo.ModTime) {
					shouldCopy = true
				}
			}
		}

		if shouldCopy {
			if !submit(job{
				path: path,
				fn:   copyFunc,
			}) {
				break
			}
		}
	}

	// Submit deletion jobs
	if s.deleteDestination {
		for path := range dstObjs {
			if ctx.Err() != nil {
				break
			}
			if _, exists := srcObjs[path]; !exists {
				if !submit(job{
					path: path,
					fn:   deleteFunc,
				}) {
					break
				}
			}
		}
	}

	close(jobs)
	wg.Wait()
	close(errChan)

	// Return first error if any
	if err := ctx.Err(); err != nil && err != context.Canceled {
		return err
	}
	for err := range errChan {
		return err
	}

	return nil
}

type job struct {
	path string
	fn   func(context.Context, string) error
}

func (j job) exec(ctx context.Context) error {
	return j.fn(ctx, j.path)
}

func parseS3URI(uri string) (bucket, prefix string) {
	trimmed := strings.TrimPrefix(uri, "s3://")
	parts := strings.SplitN(trimmed, "/", 2)
	bucket = parts[0]
	if len(parts) > 1 {
		prefix = parts[1]
	}
	return bucket, prefix
}
