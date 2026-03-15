package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dedalusj/docker-volume-sync/internal/config"
	"github.com/dedalusj/docker-volume-sync/internal/dockermanager"
	"github.com/dedalusj/docker-volume-sync/internal/syncer"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/filter"
	"github.com/robfig/cron/v3"
)

const (
	sentinelFilename = ".volumesync_done"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "health" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Healthcheck failed to load config: %v\n", err)
			os.Exit(1)
		}
		sentinelPath := filepath.Join(cfg.VolumePath, sentinelFilename)
		if _, err := os.Stat(sentinelPath); err == nil {
			fmt.Println("Healthy: initial sync completed")
			os.Exit(0)
		}
		fmt.Println("Unhealthy: initial sync not completed")
		os.Exit(1)
	}

	log.Println("Starting Docker Volume Sync...")

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

	f := filter.Opt
	f.MinAge = fs.DurationOff
	f.MaxAge = fs.DurationOff
	f.FilterRule = []string{"- " + sentinelFilename}

	remoteSyncer, err := syncer.New(ctx,
		syncer.WithConcurrency(cfg.Concurrency),
		syncer.WithDelete(cfg.DeleteDestination),
		syncer.WithFilterOpt(f),
	)
	if err != nil {
		log.Fatalf("Failed to create syncer: %v", err)
	}

	initialSync(ctx, cfg, remoteSyncer)

	// Schedule Backup
	c := cron.New()
	_, err = c.AddFunc(cfg.SyncSchedule, sync(ctx, cfg, mgr, remoteSyncer))
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

func initialSync(ctx context.Context, cfg *config.Config, remoteSyncer *syncer.Syncer) {
	sentinelPath := filepath.Join(cfg.VolumePath, sentinelFilename)
	if _, err := os.Stat(sentinelPath); os.IsNotExist(err) {
		log.Println("Sentinel file not found. Starting INITIAL SYNC (Destination -> Volume)...")
		if err := remoteSyncer.Sync(ctx, cfg.DestinationPath, cfg.VolumePath); err != nil {
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

func sync(ctx context.Context, cfg *config.Config, mgr *dockermanager.Manager, remoteSyncer *syncer.Syncer) func() {
	return func() {
		log.Println("Starting scheduled backup (Volume -> Destination)...")

		stopped, stopErr := stopContainers(ctx, cfg, mgr)
		if stopErr != nil {
			log.Printf("Error stopping containers: %v", stopErr)
		}

		if stopErr == nil {
			if syncErr := syncVolume(ctx, cfg, remoteSyncer); syncErr != nil {
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

func syncVolume(ctx context.Context, cfg *config.Config, remoteSyncer *syncer.Syncer) error {
	if err := remoteSyncer.Sync(ctx, cfg.VolumePath, cfg.DestinationPath); err != nil {
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
