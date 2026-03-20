package dockermanager

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockDockerClient struct {
	mock.Mock
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	args := m.Called(ctx, options)
	return args.Get(0).(client.ContainerListResult), args.Error(1)
}

func (m *MockDockerClient) ContainerStop(ctx context.Context, containerID string, options client.ContainerStopOptions) (client.ContainerStopResult, error) {
	args := m.Called(ctx, containerID, options)
	return args.Get(0).(client.ContainerStopResult), args.Error(1)
}

func (m *MockDockerClient) ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error) {
	args := m.Called(ctx, containerID, options)
	return args.Get(0).(client.ContainerStartResult), args.Error(1)
}

func (m *MockDockerClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestDiscoverJobs(t *testing.T) {
	ctx := context.Background()

	t.Run("Discover multiple jobs", func(t *testing.T) {
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		containers := []container.Summary{
			{
				ID: "c1",
				Labels: map[string]string{
					"volumesync.enabled":  "true",
					"volumesync.volume":   "vol1",
					"volumesync.schedule": "@daily",
				},
			},
			{
				ID: "c2",
				Labels: map[string]string{
					"volumesync.enabled":  "true",
					"volumesync.volume":   "vol1",
					"volumesync.schedule": "@daily",
				},
			},
			{
				ID: "c3",
				Labels: map[string]string{
					"volumesync.enabled":  "true",
					"volumesync.volume":   "vol2",
					"volumesync.schedule": "@hourly",
				},
			},
			{
				ID: "c4",
				Labels: map[string]string{
					"other-label": "true",
				},
			},
		}

		mockClient.On("ContainerList", ctx, client.ContainerListOptions{All: true}).Return(client.ContainerListResult{Items: containers}, nil)

		jobs, err := mgr.DiscoverJobs(ctx)
		assert.NoError(t, err)
		assert.Len(t, jobs, 2)

		var vol1Job, vol2Job bool
		for _, j := range jobs {
			if j.VolumeName == "vol1" {
				vol1Job = true
				assert.ElementsMatch(t, []string{"c1", "c2"}, j.ContainerIDs)
			}
			if j.VolumeName == "vol2" {
				vol2Job = true
				assert.ElementsMatch(t, []string{"c3"}, j.ContainerIDs)
			}
		}
		assert.True(t, vol1Job)
		assert.True(t, vol2Job)
	})
}

func TestStopContainers(t *testing.T) {
	ctx := context.Background()
	gracePeriod := 10 * time.Second

	t.Run("Stop multiple", func(t *testing.T) {
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		ids := []string{"c1", "c2"}

		mockClient.On("ContainerStop", ctx, "c1", mock.Anything).Return(client.ContainerStopResult{}, nil)
		mockClient.On("ContainerStop", ctx, "c2", mock.Anything).Return(client.ContainerStopResult{}, nil)

		stopped, err := mgr.StopContainers(ctx, ids, gracePeriod)
		assert.NoError(t, err)
		assert.ElementsMatch(t, ids, stopped)
	})

	t.Run("Skip self", func(t *testing.T) {
		hostname, _ := os.Hostname()
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		ids := []string{hostname, "c1"}

		mockClient.On("ContainerStop", ctx, "c1", mock.Anything).Return(client.ContainerStopResult{}, nil)

		stopped, err := mgr.StopContainers(ctx, ids, gracePeriod)
		assert.NoError(t, err)
		assert.Equal(t, []string{"c1"}, stopped)
	})
}

func TestStartContainers(t *testing.T) {
	ctx := context.Background()

	t.Run("Start containers successfully", func(t *testing.T) {
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		ids := []string{"container1", "container2"}

		mockClient.On("ContainerStart", ctx, "container1", client.ContainerStartOptions{}).Return(client.ContainerStartResult{}, nil)
		mockClient.On("ContainerStart", ctx, "container2", client.ContainerStartOptions{}).Return(client.ContainerStartResult{}, nil)

		err := mgr.StartContainers(ctx, ids)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
}
