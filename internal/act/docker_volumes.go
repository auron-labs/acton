package act

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/moby/moby/client"
)

const volumeLabel = "io.acton.volume"

// act derives its temporary workspace and environment volume names from the
// job container name. User mounts and the shared tool cache keep their names.
var actJobVolume = regexp.MustCompile(`^act-.+-[0-9a-f]{64}(?:-env)?$`)

func scopeJobVolumes(ctx context.Context, cli client.APIClient, project, run string, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return raw, nil
	}
	var host map[string]json.RawMessage
	if err := json.Unmarshal(raw, &host); err != nil {
		return nil, fmt.Errorf("decode container host configuration: %w", err)
	}
	var mounts []map[string]json.RawMessage
	if err := json.Unmarshal(host["Mounts"], &mounts); len(host["Mounts"]) > 0 && err != nil {
		return nil, fmt.Errorf("decode container mounts: %w", err)
	}
	for _, mount := range mounts {
		var kind, source string
		if err := json.Unmarshal(mount["Type"], &kind); err != nil || kind != "volume" {
			continue
		}
		if err := json.Unmarshal(mount["Source"], &source); err != nil || !actJobVolume.MatchString(source) {
			continue
		}
		hash := sha256.Sum256([]byte(source))
		name := "acton-" + project + "-" + run + "-" + hex.EncodeToString(hash[:])
		created, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Labels: map[string]string{projectLabel: project, volumeLabel: "run"}})
		if err != nil {
			return nil, fmt.Errorf("create Acton job volume: %w", err)
		}
		if created.Volume.Labels[projectLabel] != project || created.Volume.Labels[volumeLabel] != "run" {
			return nil, errors.New("refusing to use a job volume without Acton ownership")
		}
		mount["Source"], _ = json.Marshal(name)
	}
	var err error
	host["Mounts"], err = json.Marshal(mounts)
	if err != nil {
		return nil, fmt.Errorf("encode container mounts: %w", err)
	}
	data, err := json.Marshal(host)
	if err != nil {
		return nil, fmt.Errorf("encode container host configuration: %w", err)
	}
	return data, nil
}

func cleanupVolumes(ctx context.Context, cli client.APIClient, project string) error {
	result, err := cli.VolumeList(ctx, client.VolumeListOptions{Filters: client.Filters{"label": {projectLabel + "=" + project: true, volumeLabel + "=run": true}}})
	if err != nil {
		return fmt.Errorf("list Acton job volumes: %w", err)
	}
	var errs []error
	for _, volume := range result.Items {
		if volume.Labels[projectLabel] != project || volume.Labels[volumeLabel] != "run" {
			continue
		}
		if _, err := cli.VolumeRemove(ctx, volume.Name, client.VolumeRemoveOptions{}); err != nil {
			errs = append(errs, fmt.Errorf("remove Acton job volume %s: %w", volume.Name, err))
		}
	}
	return errors.Join(errs...)
}

func cleanupResources(ctx context.Context, cli client.APIClient, project string) error {
	containerErr := cleanupContainers(ctx, cli, project)
	return errors.Join(containerErr, cleanupVolumes(ctx, cli, project))
}
