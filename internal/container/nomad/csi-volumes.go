package nomad

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func (client *NomadClient) CreateCSIVolume(volume *model.CSIVolume) error {
	var volumes = model.CSIVolumes{}
	volumes.Volumes = append(volumes.Volumes, *volume)

	// If Id not set then use the name
	if volume.Id == "" {
		volume.Id = volume.Name
	}

	client.logger.Debug("creating csi volume", "volume_id", volume.Id)

	_, err := client.httpClient.Put(context.Background(), fmt.Sprintf("/v1/volume/csi/%s/create", volume.Id), &volumes, nil, http.StatusOK)
	if err != nil {
		client.logger.WithError(err).Debug("creating csi volume error", "volume_id", volume.Id)
		return err
	}

	return nil
}

// Nomad serializes CSI controller calls per volume: while a delete is in
// flight (e.g. an attempt a previous caller abandoned on timeout), further
// deletes are rejected with Aborted "an operation with the given Volume ID
// ... already exists". Retry until the operation clears and the volume is
// gone. The budget exceeds Nomad's own controller operation timeout, so a
// wedged operation gets one real chance to complete before we fail.
const (
	csiDeleteRetryDelay   = 10 * time.Second
	csiDeleteRetryTimeout = 5 * time.Minute
)

func (client *NomadClient) DeleteCSIVolume(id string, namespace string) error {
	client.logger.Debug("deleting csi volume", "id", id)

	deadline := time.Now().Add(csiDeleteRetryTimeout)
	for {
		code, err := client.httpClient.Delete(context.Background(),
			fmt.Sprintf("/v1/volume/csi/%s/delete?namespace=%s", id, namespace), nil, nil, http.StatusOK)
		if err == nil {
			return nil
		}

		// Ignore 500 errors where error includes "volume not found"
		if code == http.StatusInternalServerError && strings.Contains(err.Error(), "volume not found") {
			return nil
		}

		if !isDeleteInProgress(err) {
			client.logger.WithError(err).Debug("deleting csi volume error", "id", id)
			return err
		}

		if time.Now().Add(csiDeleteRetryDelay).After(deadline) {
			client.logger.WithError(err).Error("csi volume delete stuck — a controller operation appears wedged; restarting the CSI plugin or Nomad client on the node will clear it",
				"volume_id", id)
			return fmt.Errorf("csi volume delete stuck after %s: %w", csiDeleteRetryTimeout, err)
		}

		client.logger.Warn("csi volume delete already in progress, retrying", "volume_id", id)
		time.Sleep(csiDeleteRetryDelay)
	}
}

// isDeleteInProgress reports whether the error means a delete for the volume
// is already running: either Nomad's per-volume operation guard rejected the
// call, or the call timed out client-side while Nomad keeps the operation
// running server-side.
func isDeleteInProgress(err error) bool {
	return os.IsTimeout(err) || strings.Contains(err.Error(), "already exists")
}
