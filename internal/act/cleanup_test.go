package act

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type cleanupDocker struct {
	client.APIClient
	items              []container.Summary
	removed            []string
	listErr, removeErr error
}

func (d *cleanupDocker) ContainerList(_ context.Context, o client.ContainerListOptions) (client.ContainerListResult, error) {
	if !o.All {
		panic("cleanup must include stopped containers")
	}
	return client.ContainerListResult{Items: d.items}, d.listErr
}

func (d *cleanupDocker) ContainerRemove(_ context.Context, id string, o client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	if !o.Force || o.RemoveVolumes {
		panic("unsafe removal options")
	}
	d.removed = append(d.removed, id)
	return client.ContainerRemoveResult{}, d.removeErr
}

func TestCleanupContainersScopeAndErrors(t *testing.T) {
	d := &cleanupDocker{items: []container.Summary{
		{ID: "owned", Labels: map[string]string{projectLabel: "one"}},
		{ID: "other", Labels: map[string]string{projectLabel: "two"}},
		{ID: "unlabeled"},
	}}
	if err := cleanupContainers(t.Context(), d, "one"); err != nil || len(d.removed) != 1 || d.removed[0] != "owned" {
		t.Fatalf("removed=%v err=%v", d.removed, err)
	}
	d.removeErr = errors.New("remove failed")
	if err := cleanupContainers(t.Context(), d, "one"); !errors.Is(err, d.removeErr) {
		t.Fatalf("remove error: %v", err)
	}
	d.listErr = errors.New("list failed")
	if err := cleanupContainers(t.Context(), d, "one"); !errors.Is(err, d.listErr) {
		t.Fatalf("list error: %v", err)
	}
}

func TestProjectID(t *testing.T) {
	dir := workflowProject(t, "name: CI\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    container: {image: alpine, options: '--cpus 1'}\n    services:\n      db: {image: redis}\n    steps:\n      - run: echo ok\n")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	id, err := projectID(dir)
	if err != nil {
		t.Fatal(err)
	}
	other, err := projectID(link)
	if err != nil || id != other {
		t.Fatalf("canonical identity: %s %s %v", id, other, err)
	}
}
