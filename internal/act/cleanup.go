package act

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/moby/moby/client"
)

const projectLabel = "io.acton.project"

func projectID(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("absolute project path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonical project path: %w", err)
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), nil
}

// cleanupContainers checks labels itself even when the daemon applies the filter.
func cleanupContainers(ctx context.Context, cli client.APIClient, id string) error {
	result, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{"label": {projectLabel + "=" + id: true}}})
	if err != nil {
		return fmt.Errorf("list Acton containers: %w", err)
	}
	var errs []error
	for _, item := range result.Items {
		if item.Labels[projectLabel] != id {
			continue
		}
		if _, err := cli.ContainerRemove(ctx, item.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
			errs = append(errs, fmt.Errorf("remove Acton container %s: %w", item.ID, err))
		}
	}
	return errors.Join(errs...)
}
