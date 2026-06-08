package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// RunOpts holds the configuration for running a sandboxed container.
type RunOpts struct {
	Image   string
	Cmd     []string
	Mounts  []MountSpec
	Env     []string
}

// MountSpec binds a host directory (read-only by default) into the container.
type MountSpec struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// Runner manages ephemeral sandbox containers with hard security limits.
type Runner struct {
	cli *client.Client
}

// NewRunner creates a Docker client from environment defaults (DOCKER_HOST etc.).
func NewRunner() (*Runner, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Runner{cli: cli}, nil
}

// Run creates and starts a container. Security limits are always enforced:
//   - network disabled (NetworkMode "none")
//   - memory capped at 256 MiB
//   - CPU capped at 0.5 vCPU
//
// The caller must always call Remove(id) after use.
func (r *Runner) Run(ctx context.Context, opts RunOpts) (string, error) {
	mounts := make([]mount.Mount, 0, len(opts.Mounts))
	for _, m := range opts.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.HostPath,
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
		})
	}

	resp, err := r.cli.ContainerCreate(ctx,
		&container.Config{
			Image: opts.Image,
			Cmd:   opts.Cmd,
			Env:   opts.Env,
		},
		&container.HostConfig{
			NetworkMode: "none",
			Resources: container.Resources{
				Memory:   256 * 1024 * 1024, // 256 MiB
				NanoCPUs: 500_000_000,        // 0.5 vCPU
			},
			Mounts: mounts,
		},
		nil, nil, "",
	)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := r.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = r.Remove(context.Background(), resp.ID)
		return "", fmt.Errorf("start container: %w", err)
	}
	return resp.ID, nil
}

// Wait blocks until the container exits or the timeout is reached.
func (r *Runner) Wait(ctx context.Context, id string, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	statusCh, errCh := r.cli.ContainerWait(waitCtx, id, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("wait container: %w", err)
		}
		return nil
	case <-statusCh:
		return nil
	case <-waitCtx.Done():
		return fmt.Errorf("container timed out after %s", timeout)
	}
}

// Logs returns combined stdout+stderr from the container.
func (r *Runner) Logs(ctx context.Context, id string) (string, error) {
	rc, err := r.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}
	defer rc.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, rc); err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}

	result := stdout.String()
	if s := stderr.String(); s != "" {
		if result != "" {
			result += "\nstderr:\n" + s
		} else {
			result = "stderr:\n" + s
		}
	}
	return result, nil
}

// Remove force-removes the container and its anonymous volumes.
func (r *Runner) Remove(ctx context.Context, id string) error {
	return r.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

// Close releases the Docker client.
func (r *Runner) Close() error {
	return r.cli.Close()
}
