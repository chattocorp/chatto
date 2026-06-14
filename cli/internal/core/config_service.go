package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ConfigService owns semantic configuration/preference event writes.
type ConfigService struct {
	publisher  *events.Publisher
	projector  *events.Projector
	projection *ConfigProjection
}

func NewConfigService(publisher *events.Publisher, projector *events.Projector, projection *ConfigProjection) *ConfigService {
	return &ConfigService{publisher: publisher, projector: projector, projection: projection}
}

func (s *ConfigService) prepareSubject(ctx context.Context, subject string) (events.Aggregate, string, uint64, error) {
	if s.publisher == nil || s.projector == nil {
		return events.Aggregate{}, "", 0, fmt.Errorf("config service: event publisher/projector not configured")
	}
	if err := validateConfigSubject(subject); err != nil {
		return events.Aggregate{}, "", 0, err
	}
	agg := events.ConfigSubjectAggregate(subject)
	filter := agg.AllEventsFilter()
	expectedSeq, err := s.publisher.LastSubjectSeq(ctx, filter)
	if err != nil {
		return events.Aggregate{}, "", 0, fmt.Errorf("read config OCC seq: %w", err)
	}
	if expectedSeq > 0 {
		if err := s.waitFor(ctx, events.SubjectPosition(filter, expectedSeq)); err != nil {
			return events.Aggregate{}, "", 0, fmt.Errorf("wait for config projection: %w", err)
		}
	}
	return agg, filter, expectedSeq, nil
}

func (s *ConfigService) appendEventsAt(ctx context.Context, agg events.Aggregate, filter string, expectedSeq uint64, evs []*corev1.Event) error {
	if len(evs) == 0 {
		return nil
	}
	entries := make([]events.BatchEntry, 0, len(evs))
	for i, event := range evs {
		entry := events.BatchEntry{
			Subject: agg.SubjectFor(event),
			Event:   event,
		}
		if i == 0 {
			entry.ExpectedSeq = expectedSeq
			entry.FilterSubject = filter
			entry.HasOCC = true
		}
		entries = append(entries, entry)
	}
	seqs, err := s.publisher.AppendBatch(ctx, entries)
	if err != nil {
		return err
	}
	if len(seqs) > 0 {
		lastSubject := entries[len(entries)-1].Subject
		if err := s.waitFor(ctx, events.SubjectPosition(lastSubject, seqs[len(seqs)-1])); err != nil {
			return fmt.Errorf("wait for config projection: %w", err)
		}
	}
	return nil
}

func (s *ConfigService) updateSubject(
	ctx context.Context,
	subject string,
	build func(agg events.Aggregate, filter string, expectedSeq uint64) ([]*corev1.Event, error),
) error {
	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		agg, filter, expectedSeq, err := s.prepareSubject(ctx, subject)
		if err != nil {
			return err
		}
		evs, err := build(agg, filter, expectedSeq)
		if err != nil {
			return err
		}
		if len(evs) == 0 {
			return nil
		}
		if err := s.appendEventsAt(ctx, agg, filter, expectedSeq, evs); err == nil {
			return nil
		} else if !errors.Is(err, events.ErrConflict) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return ErrConfigConflict
}

func (s *ConfigService) waitFor(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("config", s.projector))
}

func validateConfigSubject(subject string) error {
	if subject == "" {
		return fmt.Errorf("config subject is empty")
	}
	if strings.ContainsAny(subject, ". \t\r\n") || subject == "*" || subject == ">" {
		return fmt.Errorf("invalid config subject %q", subject)
	}
	return nil
}
