package migrations

import (
	"context"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func configSubjectEvents(ctx context.Context, publisher *events.Publisher, subject string) ([]*corev1.Event, uint64, error) {
	agg := events.ConfigSubjectAggregate(subject)
	return publisher.SubjectEvents(ctx, agg.AllEventsFilter())
}

func seenConfigEventTypes(ctx context.Context, publisher *events.Publisher, subject string) (map[string]struct{}, uint64, error) {
	existingEvents, lastSeq, err := configSubjectEvents(ctx, publisher, subject)
	if err != nil {
		return nil, 0, err
	}
	seen := make(map[string]struct{})
	for _, event := range existingEvents {
		if typ := events.EventTypeOf(event); typ != "" {
			seen[typ] = struct{}{}
		}
	}
	return seen, lastSeq, nil
}
