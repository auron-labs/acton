package act

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	actcontainer "github.com/nektos/act/pkg/container"
)

func TestDockerJobVolumesDoNotCarryFilesAcrossRuns(t *testing.T) {
	if os.Getenv("ACTON_TEST_DOCKER") != "1" {
		t.Skip("set ACTON_TEST_DOCKER=1 for the local Docker fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cli, err := actcontainer.GetDockerClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli.Close() }()
	project := fmt.Sprintf("acton-volume-test-%d", time.Now().UnixNano())
	otherProject := project + "-other"
	legacyName := "act-" + project + "-" + strings.Repeat("a", 64)
	foreign, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: legacyName})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer stop()
		for _, id := range []string{project, otherProject} {
			if err := cleanupResources(cleanupCtx, cli, id); err != nil {
				t.Error(err)
			}
		}
		if _, err := cli.VolumeRemove(cleanupCtx, foreign.Volume.Name, client.VolumeRemoveOptions{}); err != nil {
			t.Error(err)
		}
	})
	var previousVolume string
	for _, id := range []string{project, project, otherProject} {
		host, closeProxy, err := startDockerProxy(ctx, cli, id)
		if err != nil {
			t.Fatal(err)
		}
		scoped, err := client.New(client.WithHost(host))
		if err != nil {
			closeProxy()
			t.Fatal(err)
		}
		func() {
			defer closeProxy()
			defer func() { _ = scoped.Close() }()
			created, err := scoped.ContainerCreate(ctx, client.ContainerCreateOptions{
				Name:       id + fmt.Sprintf("-%d", time.Now().UnixNano()),
				Config:     &container.Config{Image: "postgres:16-alpine", Entrypoint: []string{"sh", "-c"}, Cmd: []string{"test ! -f /work/previous-run && touch /work/previous-run"}},
				HostConfig: &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: legacyName, Target: "/work"}, {Type: mount.TypeVolume, Source: legacyName + "-env", Target: "/var/run/act"}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			inspect, err := scoped.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range inspect.Container.Mounts {
				if m.Destination == "/work" {
					if m.Name == legacyName || m.Name == previousVolume {
						t.Fatalf("run reused workspace %s", m.Name)
					}
					previousVolume = m.Name
				}
			}
			if _, err := scoped.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
				t.Fatal(err)
			}
			wait := scoped.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{})
			select {
			case result := <-wait.Result:
				if result.StatusCode != 0 {
					t.Fatalf("previous run's file survived: %+v", result)
				}
			case err := <-wait.Error:
				t.Fatal(err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}()
		if err := cleanupResources(ctx, cli, id); err != nil {
			t.Fatal(err)
		}
		volumes, err := cli.VolumeList(ctx, client.VolumeListOptions{Filters: client.Filters{"label": {projectLabel + "=" + id: true}}})
		if err != nil || len(volumes.Items) != 0 {
			t.Fatalf("temporary volumes survived cleanup: %+v %v", volumes.Items, err)
		}
		if _, err := cli.VolumeInspect(ctx, foreign.Volume.Name, client.VolumeInspectOptions{}); err != nil {
			t.Fatalf("foreign volume removed: %v", err)
		}
	}
}

func TestReusableWorkflowCleansJobVolumes(t *testing.T) {
	if os.Getenv("ACTON_TEST_DOCKER") != "1" {
		t.Skip("set ACTON_TEST_DOCKER=1 for the local Docker fixture")
	}
	dir := workflowProject(t, "name: Caller\non: push\njobs:\n  call:\n    uses: ./.github/workflows/reusable.yml\n")
	body := `name: Reusable
on: workflow_call
jobs:
  check:
    runs-on: ubuntu-latest
    container:
      image: postgres:16-alpine
    defaults:
      run:
        shell: sh
    steps:
      - run: test ! -f previous-run && touch previous-run && printf 'fresh workspace\n'
`
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", "reusable.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		r, complete := startRunner(t, dir, []string{"push", "-W", ".github/workflows/ci.yml", "--pull=false", "--no-cache-server", "--action-cache-path", t.TempDir()}, func() {})
		lines, err := finishRunner(t, r, complete)
		if err != nil || !strings.Contains(strings.Join(lines, "\n"), "fresh workspace") {
			t.Fatalf("reusable workflow failed: %v\n%s", err, strings.Join(lines, "\n"))
		}
	}
	cli, err := actcontainer.GetDockerClient(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli.Close() }()
	id, err := projectID(dir)
	if err != nil {
		t.Fatal(err)
	}
	volumes, err := cli.VolumeList(t.Context(), client.VolumeListOptions{Filters: client.Filters{"label": {projectLabel + "=" + id: true}}})
	if err != nil || len(volumes.Items) != 0 {
		t.Fatalf("reusable job volumes survived: %+v %v", volumes.Items, err)
	}
}
