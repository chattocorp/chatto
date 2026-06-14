package graph

import (
	"strconv"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

func projectionStateToModel(state core.ProjectionAdminState) *model.ProjectionState {
	metrics := make([]*model.ProjectionMetric, 0, len(state.Metrics))
	for _, metric := range state.Metrics {
		metrics = append(metrics, &model.ProjectionMetric{
			Name:  metric.Name,
			Value: int(metric.Value),
			Bytes: int(metric.Bytes),
		})
	}
	return &model.ProjectionState{
		Name:                   state.Name,
		Subjects:               append([]string(nil), state.Subjects...),
		Started:                state.Started,
		LastAppliedSequence:    strconv.FormatUint(state.LastAppliedSeq, 10),
		MatchingStreamSequence: strconv.FormatUint(state.MatchingStreamSeq, 10),
		StreamLastSequence:     strconv.FormatUint(state.StreamLastSeq, 10),
		Lag:                    int(state.Lag),
		Failed:                 state.Failed,
		FailedSequence:         strconv.FormatUint(state.FailedSeq, 10),
		Failure:                state.Failure,
		EntryCount:             int(state.EntryCount),
		EstimatedBytes:         int(state.EstimatedBytes),
		AverageEntryBytes:      int(state.AverageEntryBytes),
		Metrics:                metrics,
	}
}
