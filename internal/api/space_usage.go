package api

import (
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/api/api_utils"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetSpaceUsageCurrent(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	space, err := api_utils.GetSpaceDetails(spaceId, user)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, &apiclient.SpaceUsagePoint{
		BucketStart:   time.Now().UTC(),
		BucketKind:    model.SpaceUsageBucketMinute,
		ResourceUsage: space.ResourceUsage,
	})
}

func HandleGetSpaceUsageHistory(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	space, err := api_utils.GetSpaceDetails(spaceId, user)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "1h"
	}

	to := time.Now().UTC()
	var from time.Time
	var bucketKind string

	switch rangeName {
	case "7d":
		bucketKind = model.SpaceUsageBucketDay
		from = to.Add(-model.SpaceUsageDayRetention)
	case "1h":
		bucketKind = model.SpaceUsageBucketMinute
		from = to.Add(-model.SpaceUsageMinuteRetention)
	default:
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid range"})
		return
	}

	if from.After(to) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "from must be before to"})
		return
	}

	samples, err := database.GetInstance().GetSpaceUsageSamples(space.SpaceId, bucketKind, from, to)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	response := &apiclient.SpaceUsageHistoryResponse{
		SpaceId:    space.SpaceId,
		Range:      rangeName,
		BucketKind: bucketKind,
		Points:     make([]apiclient.SpaceUsagePoint, 0, len(samples)),
	}

	for _, sample := range samples {
		point := apiclient.SpaceUsagePoint{
			BucketStart: sample.BucketStart.UTC(),
			BucketKind:  sample.BucketKind,
			ResourceUsage: &apiclient.SpaceResourceUsage{
				CPUPercent:       sample.CPUPercent,
				MemoryUsedBytes:  sample.MemoryUsedBytes,
				MemoryLimitBytes: sample.MemoryLimitBytes,
				DiskUsedBytes:    sample.DiskUsedBytes,
				DiskLimitBytes:   sample.DiskLimitBytes,
			},
		}

		response.Points = append(response.Points, point)
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}
