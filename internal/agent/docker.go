package agent

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

		apps = append(apps, App{
			Name:          name,
			ContainerID:   c.ID,
			ContainerName: strings.TrimPrefix(c.Names[0], "/"),
			Port:          port,
			Labels:        c.Labels,
		})
	}
	Scan(ctx context.Context) ([]App, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string) error
	IsRunning(ctx context.Context, containerID string) (bool, error)
}

// DockerClient wraps the official docker client.
type DockerClient struct {
	cli *client.Client
}

// NewDockerClient creates a new DockerClient.
func NewDockerClient() (Docker, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerClient{cli: cli}, nil
}

// Scan finds all containers with label nudged.enable=true.
func (d *DockerClient) Scan(ctx context.Context) ([]App, error) {
	filter := filters.NewArgs()
	filter.Add("label", "nudged.enable=true")

	containers, err := d.cli.ContainerList(ctx, container.ListOptions{
		All:     true, // List stopped containers too
		Filters: filter,
	})
	if err != nil {
		return nil, err
	}

	var apps []App
	for _, c := range containers {
		// nudged.name or container name (strip /)
		name := c.Labels["nudged.name"]
		if name == "" {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		port := c.Labels["nudged.port"]
		if port == "" {
			port = "80"
		}

		apps = append(apps, App{
			Name:          name,
			ContainerID:   c.ID,
			ContainerName: strings.TrimPrefix(c.Names[0], "/"),
			Port:          port,
			Labels:        c.Labels,
		})
	}
	return apps, nil
}

// StartContainer starts the container with the given ID.
func (d *DockerClient) StartContainer(ctx context.Context, containerID string) error {
	return d.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// StopContainer stops the container with the given ID.
func (d *DockerClient) StopContainer(ctx context.Context, containerID string) error {
	// Default timeout of 10s is usually fine
	return d.cli.ContainerStop(ctx, containerID, container.StopOptions{})
}

// IsRunning checks if a container is running.
func (d *DockerClient) IsRunning(ctx context.Context, containerID string) (bool, error) {
	c, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}
	return c.State.Running, nil
}

// MockDocker is a mock implementation for testing when Docker is unavailable.
type MockDocker struct{}

func (m *MockDocker) Scan(ctx context.Context) ([]App, error) {
	return []App{
		{
			Name:          "demo-app",
			ContainerID:   "mock-id-123",
			ContainerName: "demo-app",
			Port:          "8080",
			Labels:        map[string]string{"nudged.enable": "true", "nudged.name": "demo-app"},
		},
	}, nil
}

func (m *MockDocker) StartContainer(ctx context.Context, containerID string) error {
	return nil
}

func (m *MockDocker) StopContainer(ctx context.Context, containerID string) error {
	return nil
}

func (m *MockDocker) IsRunning(ctx context.Context, containerID string) (bool, error) {
	return true, nil
}
