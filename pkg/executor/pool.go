package executor

import (
	"context"
	"fmt"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// ContainerPool maintains a pool of pre-warmed running containers.
type ContainerPool struct {
	cli      *client.Client
	mu       sync.Mutex
	pools    map[string][]string // image -> container IDs
	capacity int
}

func NewContainerPool(cli *client.Client, capacity int) *ContainerPool {
	return &ContainerPool{
		cli:      cli,
		pools:    make(map[string][]string),
		capacity: capacity,
	}
}

// Acquire pulls a warmed container or creates one.
func (cp *ContainerPool) Acquire(ctx context.Context, image string) (string, error) {
	cp.mu.Lock()
	list := cp.pools[image]
	if len(list) > 0 {
		id := list[len(list)-1]
		cp.pools[image] = list[:len(list)-1]
		cp.mu.Unlock()
		return id, nil
	}
	cp.mu.Unlock()

	// Create on demand
	resp, err := cp.cli.ContainerCreate(ctx,
		&container.Config{
			Image:      image,
			Cmd:        []string{"/bin/sh", "-c", "sleep 3600"},
			WorkingDir: "/box",
		},
		&container.HostConfig{
			NetworkMode:    "none",
			ReadonlyRootfs: true,
		},
		&network.NetworkingConfig{},
		nil,
		"",
	)
	if err != nil {
		return "", fmt.Errorf("failed creating container: %w", err)
	}
	if err := cp.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed starting container: %w", err)
	}
	return resp.ID, nil
}

// Release destroys a used container.
func (cp *ContainerPool) Release(ctx context.Context, id string) {
	timeout := 1
	_ = cp.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
	_ = cp.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}
