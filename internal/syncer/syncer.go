package syncer

import (
	"context"
	"fmt"
	"log"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/filter"
	"github.com/rclone/rclone/fs/sync"
	_ "github.com/rclone/rclone/backend/all" // register all rclone backends
)

type Syncer struct {
	deleteDestination bool
	concurrency       int
	filterOpt         filter.Options
}

type Option func(*Syncer)

// WithFilterOpt allows passing custom rclone filter options
func WithFilterOpt(opt filter.Options) Option {
	return func(s *Syncer) {
		s.filterOpt = opt
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

func New(ctx context.Context, opts ...Option) (*Syncer, error) {
	s := &Syncer{
		concurrency: 16,
		filterOpt:   filter.Opt,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

func (s *Syncer) Sync(ctx context.Context, src, dst string) error {
	log.Printf("Syncing %s -> %s", src, dst)

	srcFs, err := fs.NewFs(ctx, src)
	if err != nil {
		return fmt.Errorf("failed to create source fs: %w", err)
	}

	dstFs, err := fs.NewFs(ctx, dst)
	if err != nil {
		return fmt.Errorf("failed to create destination fs: %w", err)
	}

	// Apply filter if provided
	fi, err := filter.NewFilter(&s.filterOpt)
	if err != nil {
		return fmt.Errorf("failed to create filter: %w", err)
	}

	ci := fs.GetConfig(ctx)
	ci.Transfers = s.concurrency
	ci.Checkers = s.concurrency

	ctx = filter.ReplaceConfig(ctx, fi)

	if s.deleteDestination {
		err = sync.Sync(ctx, dstFs, srcFs, false)
	} else {
		err = sync.CopyDir(ctx, dstFs, srcFs, false)
	}

	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	log.Println("Sync completed successfully.")
	return nil
}
