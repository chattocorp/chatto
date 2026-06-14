package core

import (
	"errors"
	"strings"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNewConfigServiceWiresDependencies(t *testing.T) {
	publisher := testEventPublisher(t)
	projector := testEventProjector(t)
	projection := NewConfigProjection()

	service := NewConfigService(publisher, projector, projection)

	if service.publisher != publisher {
		t.Fatal("publisher was not wired")
	}
	if service.projector != projector {
		t.Fatal("projector was not wired")
	}
	if service.projection != projection {
		t.Fatal("projection was not wired")
	}
}

func TestConfigServiceUpdateSubjectAppendsAndWaitsForProjection(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewConfigProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := NewConfigService(harness.publisher, projector, projection)
	ctx := testContext(t)

	err := service.updateSubject(ctx, ConfigSubjectServer, func(_ events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		return []*corev1.Event{
			newEvent(SystemActorID, &corev1.Event{
				Event: &corev1.Event_ServerNameChanged{
					ServerNameChanged: &corev1.ServerNameChangedEvent{Name: "Service Test"},
				},
			}),
		}, nil
	})
	if err != nil {
		t.Fatalf("updateSubject returned error: %v", err)
	}

	if got := projection.EffectiveServerName(); got != "Service Test" {
		t.Fatalf("EffectiveServerName = %q, want %q", got, "Service Test")
	}
}

func TestConfigServicePrepareSubjectValidatesDependenciesAndSubject(t *testing.T) {
	ctx := testContext(t)

	if _, _, _, err := (&ConfigService{}).prepareSubject(ctx, ConfigSubjectServer); err == nil {
		t.Fatal("prepareSubject with missing dependencies returned nil error")
	} else if !strings.Contains(err.Error(), "event publisher/projector not configured") {
		t.Fatalf("prepareSubject missing dependencies error = %q", err.Error())
	}

	service := NewConfigService(testEventPublisher(t), testEventProjector(t), NewConfigProjection())
	if _, _, _, err := service.prepareSubject(ctx, "invalid.subject"); err == nil {
		t.Fatal("prepareSubject with invalid subject returned nil error")
	} else if !strings.Contains(err.Error(), "invalid config subject") {
		t.Fatalf("prepareSubject invalid subject error = %q", err.Error())
	}
}

func TestConfigServicePrepareSubjectReturnsExistingExpectedSeq(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewConfigProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := NewConfigService(harness.publisher, projector, projection)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_ServerDescriptionChanged{
			ServerDescriptionChanged: &corev1.ServerDescriptionChangedEvent{Description: "existing"},
		},
	})
	subject := events.ConfigSubjectAggregate(ConfigSubjectServer).SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}

	agg, filter, expectedSeq, err := service.prepareSubject(ctx, ConfigSubjectServer)
	if err != nil {
		t.Fatalf("prepareSubject returned error: %v", err)
	}
	if filter != agg.AllEventsFilter() {
		t.Fatalf("filter = %q, want %q", filter, agg.AllEventsFilter())
	}
	if expectedSeq != seq {
		t.Fatalf("expectedSeq = %d, want %d", expectedSeq, seq)
	}
	if got := projection.EffectiveDescription(); got != "existing" {
		t.Fatalf("EffectiveDescription = %q, want %q", got, "existing")
	}
}

func TestConfigServiceAppendEventsAtEmptyBatchIsNoop(t *testing.T) {
	harness := newTestEventHarness(t)
	service := NewConfigService(harness.publisher, testEventProjector(t), NewConfigProjection())
	ctx := testContext(t)

	if err := service.appendEventsAt(ctx, events.ConfigSubjectAggregate(ConfigSubjectServer), events.ConfigSubjectAggregate(ConfigSubjectServer).AllEventsFilter(), 0, nil); err != nil {
		t.Fatalf("appendEventsAt empty batch returned error: %v", err)
	}
	info, err := harness.stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	if info.State.Msgs != 0 {
		t.Fatalf("stream messages = %d, want 0", info.State.Msgs)
	}
}

func TestConfigServiceUpdateSubjectNoEventsIsNoop(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewConfigProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := NewConfigService(harness.publisher, projector, projection)
	ctx := testContext(t)

	if err := service.updateSubject(ctx, ConfigSubjectServer, func(events.Aggregate, string, uint64) ([]*corev1.Event, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("updateSubject no-op returned error: %v", err)
	}
	info, err := harness.stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	if info.State.Msgs != 0 {
		t.Fatalf("stream messages = %d, want 0", info.State.Msgs)
	}
}

func TestConfigServiceUpdateSubjectPropagatesBuildError(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewConfigProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := NewConfigService(harness.publisher, projector, projection)
	ctx := testContext(t)
	wantErr := errors.New("build failed")

	err := service.updateSubject(ctx, ConfigSubjectServer, func(events.Aggregate, string, uint64) ([]*corev1.Event, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("updateSubject error = %v, want %v", err, wantErr)
	}
}

func TestConfigServiceUpdateSubjectRetriesConflicts(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewConfigProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := NewConfigService(harness.publisher, projector, projection)
	ctx := testContext(t)
	attempts := 0

	err := service.updateSubject(ctx, ConfigSubjectServer, func(agg events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		attempts++
		if attempts == 1 {
			conflicting := newEvent(SystemActorID, &corev1.Event{
				Event: &corev1.Event_ServerNameChanged{
					ServerNameChanged: &corev1.ServerNameChangedEvent{Name: "conflicting write"},
				},
			})
			if _, err := harness.publisher.AppendEventually(ctx, agg.SubjectFor(conflicting), conflicting); err != nil {
				return nil, err
			}
		}
		return []*corev1.Event{
			newEvent(SystemActorID, &corev1.Event{
				Event: &corev1.Event_ServerNameChanged{
					ServerNameChanged: &corev1.ServerNameChangedEvent{Name: "retried write"},
				},
			}),
		}, nil
	})
	if err != nil {
		t.Fatalf("updateSubject returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if got := projection.EffectiveServerName(); got != "retried write" {
		t.Fatalf("EffectiveServerName = %q, want retried write", got)
	}
}

func TestConfigServiceUpdateSubjectReturnsConflictAfterRetries(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewConfigProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := NewConfigService(harness.publisher, projector, projection)
	ctx := testContext(t)
	attempts := 0

	err := service.updateSubject(ctx, ConfigSubjectServer, func(agg events.Aggregate, _ string, _ uint64) ([]*corev1.Event, error) {
		attempts++
		conflicting := newEvent(SystemActorID, &corev1.Event{
			Event: &corev1.Event_ServerDescriptionChanged{
				ServerDescriptionChanged: &corev1.ServerDescriptionChangedEvent{Description: "conflict"},
			},
		})
		if _, err := harness.publisher.AppendEventually(ctx, agg.SubjectFor(conflicting), conflicting); err != nil {
			return nil, err
		}
		return []*corev1.Event{
			newEvent(SystemActorID, &corev1.Event{
				Event: &corev1.Event_ServerNameChanged{
					ServerNameChanged: &corev1.ServerNameChangedEvent{Name: "never lands"},
				},
			}),
		}, nil
	})
	if !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("updateSubject error = %v, want ErrConfigConflict", err)
	}
	if attempts != maxConfigUpdateRetries {
		t.Fatalf("attempts = %d, want %d", attempts, maxConfigUpdateRetries)
	}
}

func TestValidateConfigSubject(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		wantErr string
	}{
		{name: "valid", subject: "interface"},
		{name: "empty", subject: "", wantErr: "config subject is empty"},
		{name: "dot", subject: "interface.theme", wantErr: "invalid config subject"},
		{name: "space", subject: "interface theme", wantErr: "invalid config subject"},
		{name: "star", subject: "*", wantErr: "invalid config subject"},
		{name: "greater-than", subject: ">", wantErr: "invalid config subject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfigSubject(tt.subject)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateConfigSubject(%q) returned error: %v", tt.subject, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateConfigSubject(%q) returned nil error", tt.subject)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateConfigSubject(%q) error = %q, want substring %q", tt.subject, err.Error(), tt.wantErr)
			}
		})
	}
}
