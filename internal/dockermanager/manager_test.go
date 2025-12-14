package dockermanager

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
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

func TestStopContainersAttachedToVolume(t *testing.T) {
	ctx := context.Background()
	volumeName := "test-volume"
	gracePeriod := 10 * time.Second

	t.Run("Stop attached containers", func(t *testing.T) {
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		containers := []container.Summary{
			{
				ID:    "container1",
				Names: []string{"/test-container-1"},
				Mounts: []container.MountPoint{
					{Type: mount.TypeVolume, Name: volumeName},
				},
			},
			{
				ID:    "container2",
				Names: []string{"/test-container-2"},
				Mounts: []container.MountPoint{
					{Type: mount.TypeVolume, Name: "other-volume"},
				},
			},
		}

		mockClient.On("ContainerList", ctx, client.ContainerListOptions{}).Return(client.ContainerListResult{Items: containers}, nil)
		mockClient.On("ContainerStop", ctx, "container1", mock.Anything).Return(client.ContainerStopResult{}, nil)

		stoppedIDs, err := mgr.StopContainersAttachedToVolume(ctx, volumeName, gracePeriod)
		assert.NoError(t, err)
		assert.Equal(t, []string{"container1"}, stoppedIDs)
		mockClient.AssertExpectations(t)
	})

	t.Run("Skip self", func(t *testing.T) {
		hostname, err := os.Hostname()
		if err != nil {
			t.Fatal(err)
		}

		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		containers := []container.Summary{
			{
				ID:    hostname,
				Names: []string{"/self-container"},
				Mounts: []container.MountPoint{
					{Type: mount.TypeVolume, Name: volumeName},
				},
			},
		}

		mockClient.On("ContainerList", ctx, client.ContainerListOptions{}).Return(client.ContainerListResult{Items: containers}, nil)

		stoppedIDs, err := mgr.StopContainersAttachedToVolume(ctx, volumeName, gracePeriod)
		assert.NoError(t, err)
		assert.Empty(t, stoppedIDs)
		mockClient.AssertExpectations(t)
	})

	t.Run("Error listing containers", func(t *testing.T) {
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		mockClient.On("ContainerList", ctx, client.ContainerListOptions{}).Return(client.ContainerListResult{}, fmt.Errorf("list error"))

		_, err := mgr.StopContainersAttachedToVolume(ctx, volumeName, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "list error")
		mockClient.AssertExpectations(t)
	})

	t.Run("Error stopping container", func(t *testing.T) {
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		containers := []container.Summary{
			{
				ID:    "container1",
				Names: []string{"/test-container-1"},
				Mounts: []container.MountPoint{
					{Type: mount.TypeVolume, Name: volumeName},
				},
			},
		}

		mockClient.On("ContainerList", ctx, client.ContainerListOptions{}).Return(client.ContainerListResult{Items: containers}, nil)
		mockClient.On("ContainerStop", ctx, "container1", mock.Anything).Return(client.ContainerStopResult{}, fmt.Errorf("stop error"))

		_, err := mgr.StopContainersAttachedToVolume(ctx, volumeName, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "stop error")
		mockClient.AssertExpectations(t)
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

	t.Run("Start container failure (logs but does not return error)", func(t *testing.T) {
		mockClient := new(MockDockerClient)
		mgr := &Manager{client: mockClient}

		ids := []string{"container1"}

		mockClient.On("ContainerStart", ctx, "container1", client.ContainerStartOptions{}).Return(client.ContainerStartResult{}, fmt.Errorf("start error"))

		err := mgr.StartContainers(ctx, ids)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
}
