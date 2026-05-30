package migrations

import (
	"context"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func seenConfigPaths(ctx context.Context, publisher *events.Publisher, subject string) (map[string]struct{}, uint64, error) {
	agg := events.ConfigSubjectAggregate(subject)
	existingEvents, lastSeq, err := publisher.SubjectEvents(ctx, agg.AllEventsFilter())
	if err != nil {
		return nil, 0, err
	}
	seen := make(map[string]struct{})
	for _, event := range existingEvents {
		switch e := event.GetEvent().(type) {
		case *corev1.Event_ConfigValueSet:
			if e.ConfigValueSet.GetSubject() == subject {
				seen[e.ConfigValueSet.GetPath()] = struct{}{}
			}
		case *corev1.Event_ConfigValueCleared:
			if e.ConfigValueCleared.GetSubject() == subject {
				seen[e.ConfigValueCleared.GetPath()] = struct{}{}
			}
		}
	}
	return seen, lastSeq, nil
}
