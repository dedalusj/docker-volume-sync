package dockermanager

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

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

// StopContainersAttachedToVolume finds all containers attached to the given volume (except self)
// and stops them. It returns a list of stopped container IDs.
func (m *Manager) StopContainersAttachedToVolume(ctx context.Context, volumeName string, gracePeriod time.Duration) ([]string, error) {
	selfID, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname (self container ID): %w", err)
	}

	res, err := m.client.ContainerList(ctx, dockerClient.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	containers := res.Items

	var stoppedIDs []string
	timeoutSeconds := int(gracePeriod.Seconds())

	for _, c := range containers {
		isAttached := false
		for _, mount := range c.Mounts {
			if mount.Type == "volume" && mount.Name == volumeName {
				isAttached = true
				break
			}
		}

		if isAttached {
			if c.ID == selfID || (len(c.ID) >= 12 && len(selfID) >= 12 && c.ID[:12] == selfID[:12]) {
				log.Printf("Skipping self (%s)", c.ID)
				continue
			}

			idToLog := c.ID
			if len(c.ID) > 12 {
				idToLog = c.ID[:12]
			}
			log.Printf("Stopping container %s (%s) attached to volume %s...", idToLog, c.Names, volumeName)
			_, err := m.client.ContainerStop(ctx, c.ID, dockerClient.ContainerStopOptions{Timeout: &timeoutSeconds})
			if err != nil {
				return stoppedIDs, fmt.Errorf("failed to stop container %s: %w", c.ID, err)
			}
			stoppedIDs = append(stoppedIDs, c.ID)
		}
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
