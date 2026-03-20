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
	readyMarkerPath  = "/tmp/volumesync_ready"
	volumesBaseDir   = "/volumes"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "health" {
		if _, err := os.Stat(readyMarkerPath); err == nil {
			fmt.Println("Healthy: initial sync completed for all volumes")
			os.Exit(0)
		}
		fmt.Println("Unhealthy: initial sync not completed")
		os.Exit(1)
	}

	log.Println("Starting Docker Volume Sync (Single-Container Mode)...")

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		log.Fatalf("Failed to load global config: %v", err)
	}

	mgr, err := dockermanager.New()
	if err != nil {
		log.Fatalf("Failed to create docker manager: %v", err)
	}
	defer mgr.Close()

	ctx := context.Background()

	jobs, err := mgr.DiscoverJobs(ctx)
	if err != nil {
		log.Fatalf("Failed to discover jobs: %v", err)
	}

	if len(jobs) == 0 {
		log.Println("No volume jobs discovered. Waiting for signals...")
	} else {
		log.Printf("Discovered %d volume jobs.", len(jobs))
	}

	c := cron.New()

	for _, job := range jobs {
		job := job // capture loop var
		volumePath := filepath.Join(volumesBaseDir, job.VolumeName)
		remotePath := filepath.Join(globalCfg.DestinationPath, job.SubPath)

		// Create syncer for this job
		f := filter.Opt
		f.MinAge = fs.DurationOff
		f.MaxAge = fs.DurationOff
		f.FilterRule = []string{"- " + sentinelFilename}

		s, err := syncer.New(ctx,
			syncer.WithConcurrency(job.Concurrency),
			syncer.WithDelete(job.Delete),
			syncer.WithFilterOpt(f),
		)
		if err != nil {
			log.Fatalf("Failed to create syncer for %s: %v", job.VolumeName, err)
		}

		// 1. Initial Sync
		initialSync(ctx, volumePath, remotePath, s)

		// 2. Schedule Backup
		_, err = c.AddFunc(job.Schedule, syncJob(ctx, job, volumePath, remotePath, mgr, s))
		if err != nil {
			log.Fatalf("Failed to schedule job for %s: %v", job.VolumeName, err)
		}
		log.Printf("Scheduled backup for %s: %s", job.VolumeName, job.Schedule)
	}

	// All initial syncs done
	if err := os.WriteFile(readyMarkerPath, []byte(time.Now().String()), 0644); err != nil {
		log.Printf("Warning: failed to create ready marker: %v", err)
	}

	c.Start()

	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	c.Stop()
	_ = os.Remove(readyMarkerPath)
}

func initialSync(ctx context.Context, localPath, remotePath string, s *syncer.Syncer) {
	sentinelPath := filepath.Join(localPath, sentinelFilename)
	if _, err := os.Stat(sentinelPath); os.IsNotExist(err) {
		log.Printf("[%s] Sentinel file not found. Starting INITIAL SYNC (Remote -> Local)...", filepath.Base(localPath))
		if err := s.Sync(ctx, remotePath, localPath); err != nil {
			log.Fatalf("Initial sync failed for %s: %v", localPath, err)
		}
		log.Printf("[%s] Initial sync completed.", filepath.Base(localPath))
		if err := os.WriteFile(sentinelPath, []byte(time.Now().String()), 0644); err != nil {
			log.Fatalf("Failed to create sentinel file for %s: %v", localPath, err)
		}
	} else {
		log.Printf("[%s] Sentinel file found. Skipping initial sync.", filepath.Base(localPath))
	}
}

func syncJob(ctx context.Context, job config.VolumeJob, localPath, remotePath string, mgr *dockermanager.Manager, s *syncer.Syncer) func() {
	return func() {
		log.Printf("[%s] Starting scheduled backup...", job.VolumeName)

		var stopped []string
		var stopErr error

		if job.StopContainer {
			stopped, stopErr = mgr.StopContainers(ctx, job.ContainerIDs, job.StopGracePeriod)
			if stopErr != nil {
				log.Printf("[%s] Error stopping containers: %v", job.VolumeName, stopErr)
			}
		}

		if stopErr == nil {
			if err := s.Sync(ctx, localPath, remotePath); err != nil {
				log.Printf("[%s] Error syncing volume: %v", job.VolumeName, err)
			} else {
				log.Printf("[%s] Backup completed successfully.", job.VolumeName)
			}
		}

		if job.StopContainer && len(stopped) > 0 {
			if err := mgr.StartContainers(ctx, stopped); err != nil {
				log.Printf("[%s] Error restarting containers: %v", job.VolumeName, err)
			}
		}
	}
}
