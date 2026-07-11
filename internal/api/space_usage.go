package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/api/api_utils"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetSpaceUsageCurrent(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	space, err := api_utils.GetAccessibleSpace(spaceId, user)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	var resourceUsage *apiclient.SpaceResourceUsage
	isLive := false
	cfg := config.GetServerConfig()
	if space.IsDeployed && (space.Zone == "" || space.Zone == cfg.Zone) {
		if state := agent_server.GetSession(space.Id); state != nil {
			resourceUsage = &apiclient.SpaceResourceUsage{
				CPUPercent:       state.CPUPercent,
				MemoryUsedBytes:  state.MemoryUsedBytes,
				MemoryLimitBytes: state.MemoryLimitBytes,
				DiskUsedBytes:    state.DiskUsedBytes,
				DiskLimitBytes:   state.DiskLimitBytes,
			}
			// Only present the reading as live if a real state report has
			// arrived within the liveness window. A wedged state-report loop
			// can leave a ping-alive session holding a frozen last value (e.g.
			// a stuck 99.9%); don't show that as current data. The last-known
			// values are still returned so callers can inspect them, but
			// is_live=false signals they are stale.
			isLive = state.TelemetryLive()
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, &apiclient.SpaceUsagePoint{
		BucketStart:   time.Now().UTC(),
		BucketKind:    model.SpaceUsageBucketMinute,
		IsLive:        isLive,
		ResourceUsage: resourceUsage,
	})
}

func HandleGetSpaceUsageHistory(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	space, err := api_utils.GetAccessibleSpace(spaceId, user)
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

	response := &apiclient.SpaceUsageHistoryResponse{
		SpaceId:    space.Id,
		Range:      rangeName,
		BucketKind: bucketKind,
		Points:     []apiclient.SpaceUsagePoint{},
	}

	if bucketKind == model.SpaceUsageBucketDay {
		points, err := buildDailySpaceUsagePoints(space.Id, from, to)
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		response.Points = points
	} else {
		samples, err := database.GetInstance().GetSpaceUsageSamples(space.Id, bucketKind, from, to)
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		response.Points = make([]apiclient.SpaceUsagePoint, 0, len(samples))
		for _, sample := range samples {
			response.Points = append(response.Points, pointFromSpaceUsageSample(sample))
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func buildDailySpaceUsagePoints(spaceId string, from time.Time, to time.Time) ([]apiclient.SpaceUsagePoint, error) {
	daySamples, err := database.GetInstance().GetSpaceUsageSamples(spaceId, model.SpaceUsageBucketDay, from, to)
	if err != nil {
		return nil, err
	}

	pointsByDay := map[time.Time]*apiclient.SpaceUsagePoint{}
	for _, sample := range daySamples {
		point := pointFromSpaceUsageSample(sample)
		pointCopy := point
		pointsByDay[point.BucketStart] = &pointCopy
	}

	minuteFrom := to.Add(-model.SpaceUsageMinuteRetention)
	if minuteFrom.Before(from) {
		minuteFrom = from
	}
	minuteSamples, err := database.GetInstance().GetSpaceUsageSamples(spaceId, model.SpaceUsageBucketMinute, minuteFrom, to)
	if err != nil {
		return nil, err
	}

	for _, sample := range minuteSamples {
		dayBucket := model.BucketStartForKind(sample.BucketStart.UTC(), model.SpaceUsageBucketDay)
		point := pointsByDay[dayBucket]
		if point == nil {
			point = &apiclient.SpaceUsagePoint{
				BucketStart:   dayBucket,
				BucketKind:    model.SpaceUsageBucketDay,
				ResourceUsage: &apiclient.SpaceResourceUsage{},
			}
			pointsByDay[dayBucket] = point
		}
		mergeSpaceResourceUsage(point.ResourceUsage, sample)
	}

	points := make([]apiclient.SpaceUsagePoint, 0, len(pointsByDay))
	for _, point := range pointsByDay {
		points = append(points, *point)
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].BucketStart.Before(points[j].BucketStart)
	})
	return points, nil
}

func pointFromSpaceUsageSample(sample *model.SpaceUsageSample) apiclient.SpaceUsagePoint {
	return apiclient.SpaceUsagePoint{
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
}

func mergeSpaceResourceUsage(target *apiclient.SpaceResourceUsage, sample *model.SpaceUsageSample) {
	if target == nil {
		return
	}
	if sample.CPUPercent > target.CPUPercent {
		target.CPUPercent = sample.CPUPercent
	}
	if sample.MemoryUsedBytes > target.MemoryUsedBytes {
		target.MemoryUsedBytes = sample.MemoryUsedBytes
	}
	if sample.MemoryLimitBytes > target.MemoryLimitBytes {
		target.MemoryLimitBytes = sample.MemoryLimitBytes
	}
	if sample.DiskUsedBytes > target.DiskUsedBytes {
		target.DiskUsedBytes = sample.DiskUsedBytes
	}
	if sample.DiskLimitBytes > target.DiskLimitBytes {
		target.DiskLimitBytes = sample.DiskLimitBytes
	}
}
