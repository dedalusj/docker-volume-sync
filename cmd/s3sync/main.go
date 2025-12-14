package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dedalusj/docker-volume-sync/internal/config"
	"github.com/dedalusj/docker-volume-sync/internal/dockermanager"
	"github.com/dedalusj/docker-volume-sync/internal/syncer"
	"github.com/robfig/cron/v3"
)

const (
	sentinelFilename = ".s3sync_done"
)

func main() {
	log.Println("Starting Docker Volume S3 Sync...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mgr, err := dockermanager.New()
	if err != nil {
		log.Fatalf("Failed to create docker manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	s3Syncer, err := syncer.New(ctx,
		syncer.WithConcurrency(cfg.Concurrency),
		syncer.WithDelete(cfg.DeleteDestination),
		syncer.WithFilter(func(path string) bool {
			return !strings.HasSuffix(path, sentinelFilename)
		}),
	)
	if err != nil {
		log.Fatalf("Failed to create syncer: %v", err)
	}

	initialSync(ctx, cfg, s3Syncer)

	// Schedule Backup
	c := cron.New()
	_, err = c.AddFunc(cfg.SyncSchedule, sync(ctx, cfg, mgr, s3Syncer))
	if err != nil {
		log.Fatalf("Failed to schedule job: %v", err)
	}

	c.Start()
	log.Printf("Scheduler started with schedule: %s", cfg.SyncSchedule)

	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	c.Stop()
}

func initialSync(ctx context.Context, cfg *config.Config, s3Syncer *syncer.Syncer) {
	sentinelPath := filepath.Join(cfg.VolumePath, sentinelFilename)
	if _, err := os.Stat(sentinelPath); os.IsNotExist(err) {
		log.Println("Sentinel file not found. Starting INITIAL SYNC (S3 -> Volume)...")
		if err := s3Syncer.Sync(ctx, cfg.S3Path, cfg.VolumePath); err != nil {
			log.Fatalf("Initial sync failed: %v", err)
		}
		log.Println("Initial sync completed.")
		if err := os.WriteFile(sentinelPath, []byte(time.Now().String()), 0644); err != nil {
			log.Fatalf("Failed to create sentinel file: %v", err)
		}
	} else {
		log.Println("Sentinel file found. Skipping initial sync.")
	}
}

func sync(ctx context.Context, cfg *config.Config, mgr *dockermanager.Manager, s3Syncer *syncer.Syncer) func() {
	return func() {
		log.Println("Starting scheduled backup (Volume -> S3)...")

		stopped, stopErr := stopContainers(ctx, cfg, mgr)
		if stopErr != nil {
			log.Printf("Error stopping containers: %v", stopErr)
		}

		if stopErr == nil {
			if syncErr := syncVolume(ctx, cfg, s3Syncer); syncErr != nil {
				log.Printf("Error syncing volume: %v", syncErr)
			}
		}

		if restartErr := restartContainers(ctx, mgr, stopped); restartErr != nil {
			log.Printf("Error restarting containers: %v", restartErr)
		}
	}
}

func stopContainers(ctx context.Context, cfg *config.Config, mgr *dockermanager.Manager) (stopped []string, err error) {
	// Stop containers if VolumeName is set
	if cfg.VolumeName != "" {
		stopped, err = mgr.StopContainersAttachedToVolume(ctx, cfg.VolumeName, cfg.DockerStopGracePeriod)
		if err != nil {
			return nil, fmt.Errorf("stopping containers: %v", err)
		}
		if len(stopped) > 0 {
			log.Printf("Stopped %d containers.", len(stopped))
		}
	} else {
		log.Println("VOLUME_NAME not set. Skipping container stop.")
	}
	return stopped, nil
}

func syncVolume(ctx context.Context, cfg *config.Config, s3Syncer *syncer.Syncer) error {
	if err := s3Syncer.Sync(ctx, cfg.VolumePath, cfg.S3Path); err != nil {
		return fmt.Errorf("syncing volume: %v", err)
	}
	log.Println("Backup completed successfully.")
	return nil
}

func restartContainers(ctx context.Context, mgr *dockermanager.Manager, stopped []string) error {
	if len(stopped) == 0 {
		return nil
	}
	if err := mgr.StartContainers(ctx, stopped); err != nil {
		return fmt.Errorf("starting containers: %v", err)
	}
	log.Printf("Started %d containers.", len(stopped))
	return nil
}
