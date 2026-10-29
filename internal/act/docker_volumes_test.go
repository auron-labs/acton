package act

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

type volumeDocker struct {
	client.APIClient
	created []client.VolumeCreateOptions
	items   []volume.Volume
	removed []string
	err     error
	foreign bool
}

func (d *volumeDocker) VolumeCreate(_ context.Context, opts client.VolumeCreateOptions) (client.VolumeCreateResult, error) {
	d.created = append(d.created, opts)
	labels := opts.Labels
	if d.foreign {
		labels = nil
	}
	return client.VolumeCreateResult{Volume: volume.Volume{Name: opts.Name, Labels: labels}}, d.err
}

func (d *volumeDocker) VolumeList(_ context.Context, _ client.VolumeListOptions) (client.VolumeListResult, error) {
	return client.VolumeListResult{Items: d.items}, d.err
}

func (d *volumeDocker) VolumeRemove(_ context.Context, name string, opts client.VolumeRemoveOptions) (client.VolumeRemoveResult, error) {
	if opts.Force {
		panic("must not force removal of mounted volumes")
	}
	d.removed = append(d.removed, name)
	return client.VolumeRemoveResult{}, d.err
}

func TestJobVolumesAreIsolatedAndUserMountsPreserved(t *testing.T) {
	d := &volumeDocker{}
	name := "act-ci-build-" + strings.Repeat("a", 64)
	raw := json.RawMessage(`{"Mounts":[{"Type":"volume","Source":"` + name + `","Target":"/work"},{"Type":"volume","Source":"` + name + `-env","Target":"/var/run/act"},{"Type":"volume","Source":"user-data","Target":"/data"},{"Type":"volume","Source":"act-toolcache","Target":"/opt/hostedtoolcache"}],"Binds":["/project:/bind"]}`)
	first, err := scopeJobVolumes(t.Context(), d, "project-one", "run-one", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.created) != 2 || !strings.Contains(string(first), `"Source":"user-data"`) || !strings.Contains(string(first), `"Source":"act-toolcache"`) || !strings.Contains(string(first), `/project:/bind`) {
		t.Fatalf("mounts changed incorrectly: %s %+v", first, d.created)
	}
	for _, opts := range d.created {
		if opts.Labels[projectLabel] != "project-one" || opts.Labels[volumeLabel] != "run" || opts.Name == name || opts.Name == name+"-env" {
			t.Fatalf("unowned or reused volume: %+v", opts)
		}
	}
	same, err := scopeJobVolumes(t.Context(), d, "project-one", "run-one", raw)
	if err != nil || string(first) != string(same) {
		t.Fatalf("actions in one run lost job volumes: %s %v", same, err)
	}
	next, err := scopeJobVolumes(t.Context(), d, "project-one", "run-two", raw)
	if err != nil || string(first) == string(next) {
		t.Fatalf("later run reused job volumes: %s %v", next, err)
	}
	other, err := scopeJobVolumes(t.Context(), d, "project-two", "run-one", raw)
	if err != nil || string(first) == string(other) {
		t.Fatalf("other project reused job volumes: %s %v", other, err)
	}
	d.foreign = true
	if _, err := scopeJobVolumes(t.Context(), d, "project-one", "run-one", raw); err == nil {
		t.Fatal("accepted an existing foreign volume")
	}
	d.foreign = false
	d.err = errors.New("volume creation failed")
	if _, err := scopeJobVolumes(t.Context(), d, "project-one", "run-one", raw); !errors.Is(err, d.err) {
		t.Fatalf("lost create failure: %v", err)
	}
}

func TestVolumeCleanupRequiresOwnership(t *testing.T) {
	d := &volumeDocker{items: []volume.Volume{
		{Name: "owned", Labels: map[string]string{projectLabel: "one", volumeLabel: "run"}},
		{Name: "other-project", Labels: map[string]string{projectLabel: "two", volumeLabel: "run"}},
		{Name: "user-cache", Labels: map[string]string{projectLabel: "one"}},
		{Name: "unlabeled"},
	}}
	if err := cleanupVolumes(t.Context(), d, "one"); err != nil || len(d.removed) != 1 || d.removed[0] != "owned" {
		t.Fatalf("removed=%v err=%v", d.removed, err)
	}
}
