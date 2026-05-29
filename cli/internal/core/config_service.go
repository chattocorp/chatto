package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type configWrite struct {
	path  string
	value *configv1.ConfigValue
}

// ConfigService owns dynamic configuration writes and typed reads. It is the
// generic substrate underneath compatibility surfaces like ConfigManager.
type ConfigService struct {
	publisher  *events.Publisher
	projector  *events.Projector
	projection *ConfigProjection
}

func NewConfigService(publisher *events.Publisher, projector *events.Projector, projection *ConfigProjection) *ConfigService {
	return &ConfigService{publisher: publisher, projector: projector, projection: projection}
}

func (s *ConfigService) Get(subject string, path ConfigPath[string]) (string, bool, error) {
	return getProjectedConfig(s.projection, subject, path)
}

func GetConfig[T any](s *ConfigService, subject string, path ConfigPath[T]) (T, bool, error) {
	return getProjectedConfig(s.projection, subject, path)
}

func SetConfig[T any](ctx context.Context, s *ConfigService, actorID, subject string, path ConfigPath[T], value T) error {
	if path.Authorize != nil {
		if err := path.Authorize(ctx, actorID, subject); err != nil {
			return err
		}
	}
	encoded, err := path.encode(subject, value)
	if err != nil {
		return err
	}
	return s.setValues(ctx, actorID, subject, []configWrite{{path: path.Name, value: encoded}})
}

func ClearConfig[T any](ctx context.Context, s *ConfigService, actorID, subject string, path ConfigPath[T]) error {
	if path.Authorize != nil {
		if err := path.Authorize(ctx, actorID, subject); err != nil {
			return err
		}
	}
	return s.clearPaths(ctx, actorID, subject, []string{path.Name})
}

func (s *ConfigService) setValues(ctx context.Context, actorID, subject string, writes []configWrite) error {
	if s.publisher == nil || s.projector == nil {
		return fmt.Errorf("config service: event publisher/projector not configured")
	}
	if err := validateConfigSubject(subject); err != nil {
		return err
	}
	if len(writes) == 0 {
		return nil
	}
	if err := validateConfigWrites(writes); err != nil {
		return err
	}

	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		agg, filter, expectedSeq, err := s.prepareSubject(ctx, subject)
		if err != nil {
			return err
		}
		if err := s.appendValuesAt(ctx, actorID, agg, filter, expectedSeq, writes); err == nil {
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

func (s *ConfigService) clearPaths(ctx context.Context, actorID, subject string, paths []string) error {
	if s.publisher == nil || s.projector == nil {
		return fmt.Errorf("config service: event publisher/projector not configured")
	}
	if err := validateConfigSubject(subject); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	for _, path := range paths {
		if err := validateConfigPath(path); err != nil {
			return err
		}
	}

	for attempt := 0; attempt < maxConfigUpdateRetries; attempt++ {
		agg, filter, expectedSeq, err := s.prepareSubject(ctx, subject)
		if err != nil {
			return err
		}
		if err := s.appendClearsAt(ctx, actorID, agg, filter, expectedSeq, paths); err == nil {
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

func validateConfigWrites(writes []configWrite) error {
	for _, write := range writes {
		if err := validateConfigPath(write.path); err != nil {
			return err
		}
		if write.value == nil {
			return fmt.Errorf("config path %q has nil value", write.path)
		}
	}
	return nil
}

func (s *ConfigService) prepareSubject(ctx context.Context, subject string) (events.Aggregate, string, uint64, error) {
	agg := events.ConfigSubjectAggregate(subject)
	filter := agg.AllEventsFilter()
	expectedSeq, err := s.publisher.LastSubjectSeq(ctx, filter)
	if err != nil {
		return events.Aggregate{}, "", 0, fmt.Errorf("read config OCC seq: %w", err)
	}
	if expectedSeq > 0 {
		if err := s.projector.WaitForSeq(ctx, expectedSeq); err != nil {
			return events.Aggregate{}, "", 0, fmt.Errorf("wait for config projection: %w", err)
		}
	}
	return agg, filter, expectedSeq, nil
}

func (s *ConfigService) appendValuesAt(ctx context.Context, actorID string, agg events.Aggregate, filter string, expectedSeq uint64, writes []configWrite) error {
	writes = s.changedWrites(agg.ID, writes)
	if len(writes) == 0 {
		return nil
	}
	entries := make([]events.BatchEntry, 0, len(writes))
	for i, write := range writes {
		event := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_ConfigValueSet{
				ConfigValueSet: &corev1.ConfigValueSetEvent{
					Subject: agg.ID,
					Path:    write.path,
					Value:   write.value,
				},
			},
		})
		entry := events.BatchEntry{
			Subject: agg.Subject(events.EventConfigValueSet),
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
		if err := s.projector.WaitForSeq(ctx, seqs[len(seqs)-1]); err != nil {
			return fmt.Errorf("wait for config projection: %w", err)
		}
	}
	return nil
}

func (s *ConfigService) appendClearsAt(ctx context.Context, actorID string, agg events.Aggregate, filter string, expectedSeq uint64, paths []string) error {
	paths = s.existingPaths(agg.ID, paths)
	if len(paths) == 0 {
		return nil
	}
	entries := make([]events.BatchEntry, 0, len(paths))
	for i, path := range paths {
		event := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_ConfigValueCleared{
				ConfigValueCleared: &corev1.ConfigValueClearedEvent{
					Subject: agg.ID,
					Path:    path,
				},
			},
		})
		entry := events.BatchEntry{
			Subject: agg.Subject(events.EventConfigValueCleared),
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
		if err := s.projector.WaitForSeq(ctx, seqs[len(seqs)-1]); err != nil {
			return fmt.Errorf("wait for config projection: %w", err)
		}
	}
	return nil
}

func (s *ConfigService) changedWrites(subject string, writes []configWrite) []configWrite {
	if s.projection == nil {
		return writes
	}
	changed := writes[:0]
	for _, write := range writes {
		current, ok := s.projection.Value(subject, write.path)
		if ok && proto.Equal(current, write.value) {
			continue
		}
		changed = append(changed, write)
	}
	return changed
}

func (s *ConfigService) existingPaths(subject string, paths []string) []string {
	if s.projection == nil {
		return paths
	}
	existing := paths[:0]
	for _, path := range paths {
		if _, ok := s.projection.Value(subject, path); ok {
			existing = append(existing, path)
		}
	}
	return existing
}
