package dockermanager

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dedalusj/docker-volume-sync/internal/config"
	dockerClient "github.com/moby/moby/client"
)

type DockerClient interface {
	ContainerList(ctx context.Context, options dockerClient.ContainerListOptions) (dockerClient.ContainerListResult, error)
	ContainerStop(ctx context.Context, containerID string, options dockerClient.ContainerStopOptions) (dockerClient.ContainerStopResult, error)
	ContainerStart(ctx context.Context, containerID string, options dockerClient.ContainerStartOptions) (dockerClient.ContainerStartResult, error)
	Close() error
}

type Manager struct {
	client DockerClient
}

func New() (*Manager, error) {
	client, err := dockerClient.New(dockerClient.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &Manager{client: client}, nil
}

func (m *Manager) Close() error {
	return m.client.Close()
}

// DiscoverJobs finds all containers with volumesync labels and groups them into jobs.
func (m *Manager) DiscoverJobs(ctx context.Context) ([]config.VolumeJob, error) {
	res, err := m.client.ContainerList(ctx, dockerClient.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	containers := res.Items

	jobsMap := make(map[string]*config.VolumeJob)

	for _, c := range containers {
		job, err := config.ParseLabels(c.Labels)
		if err != nil {
			log.Printf("Warning: failed to parse labels for container %s: %v", c.ID, err)
			continue
		}
		if job == nil {
			continue
		}

		if existing, ok := jobsMap[job.VolumeName]; ok {
			// Merge container IDs for the same volume job
			existing.ContainerIDs = append(existing.ContainerIDs, c.ID)
		} else {
			job.ContainerIDs = []string{c.ID}
			jobsMap[job.VolumeName] = job
		}
	}

	var jobs []config.VolumeJob
	for _, job := range jobsMap {
		jobs = append(jobs, *job)
	}

	return jobs, nil
}

// StopContainers stops the given containers with a grace period.
func (m *Manager) StopContainers(ctx context.Context, ids []string, gracePeriod time.Duration) ([]string, error) {
	selfID, _ := os.Hostname()

	var stoppedIDs []string
	timeoutSeconds := int(gracePeriod.Seconds())

	for _, id := range ids {
		if id == selfID || (len(id) >= 12 && len(selfID) >= 12 && id[:12] == selfID[:12]) {
			log.Printf("Skipping self (%s)", id)
			continue
		}

		idToLog := id
		if len(id) > 12 {
			idToLog = id[:12]
		}
		log.Printf("Stopping container %s...", idToLog)
		_, err := m.client.ContainerStop(ctx, id, dockerClient.ContainerStopOptions{Timeout: &timeoutSeconds})
		if err != nil {
			log.Printf("Failed to stop container %s: %v", id, err)
			continue
		}
		stoppedIDs = append(stoppedIDs, id)
	}

	return stoppedIDs, nil
}

func (m *Manager) StartContainers(ctx context.Context, ids []string) error {
	for _, id := range ids {
		idToLog := id
		if len(id) > 12 {
			idToLog = id[:12]
		}
		log.Printf("Restarting container %s...", idToLog)
		_, err := m.client.ContainerStart(ctx, id, dockerClient.ContainerStartOptions{})
		if err != nil {
			log.Printf("Failed to start container %s: %v", id, err)
		}
	}
	return nil
}
