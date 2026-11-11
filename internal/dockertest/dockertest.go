// Package dockertest holds test-only helpers for tests that need a Docker daemon.
package dockertest

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/nektos/act/pkg/container"
)

const pingTimeout = 5 * time.Second

// Require skips the test unless the Docker daemon act would use is reachable.
// It resolves the daemon the way the runner does (DOCKER_HOST, then act's
// default socket locations) without modifying the process environment.
func Require(t *testing.T) {
	t.Helper()
	socket, err := container.GetSocketAndHost("")
	if err != nil {
		t.Skipf("Docker is unavailable: %v", err)
	}
	cli, err := client.New(client.FromEnv, client.WithHost(socket.Host))
	if err != nil {
		t.Skipf("Docker is unavailable: %v", err)
	}
	defer func() { _ = cli.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), pingTimeout)
	defer cancel()
	if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
		t.Skipf("Docker daemon at %s is unreachable: %v", socket.Host, err)
	}
}
