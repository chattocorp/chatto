package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/evtstream"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// ErrSetupUnavailable means the first-run setup command is disabled or closed.
var ErrSetupUnavailable = errors.New("first-run setup is not available")

// ErrSetupRequired blocks public registration until the initial owner exists.
var ErrSetupRequired = errors.New("complete first-run setup before creating accounts")

// ServerSetupInput contains the initial public settings and local owner credentials.
// Password is transient and must never be logged or stored as plaintext.
type ServerSetupInput struct {
	ServerName  string
	Description string
	Login       string
	DisplayName string
	Password    string
}

// initializeServerSetup runs before any boot defaults are written. Only a truly
// empty history may offer setup. Existing histories, including purged histories,
// are permanently closed. Empty histories use whole-EVT OCC; existing histories
// use setup-only OCC so normal chat traffic cannot prevent an upgrade.
func (c *ChattoCore) initializeServerSetup(ctx context.Context) error {
	for attempt := 0; attempt < 10; attempt++ {
		seq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
		if err != nil {
			return err
		}
		if seq != 0 {
			return nil
		}
		info, err := c.storage.serverEvtStream.Info(ctx)
		if err != nil {
			return err
		}
		event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerInitialized{ServerInitialized: &evtv1.ServerInitializedEvent{}}})
		fresh := info.State.LastSeq == 0
		if fresh {
			// Old deployments can predate EVT. A legacy stream is evidence of
			// an existing installation even when the new EVT stream is empty.
			_, legacyErr := c.js.Stream(ctx, "SERVER_EVENTS")
			if legacyErr == nil {
				fresh = false
			} else if !errors.Is(legacyErr, jetstream.ErrStreamNotFound) {
				return legacyErr
			}
		}
		if fresh {
			event = newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerSetupOffered{ServerSetupOffered: &evtv1.ServerSetupOfferedEvent{}}})
		}
		filter := evtstream.SetupAggregate().AllEventsFilter()
		if fresh {
			filter = evtstream.EventSubjectFilter()
		}
		_, err = c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: evtstream.SetupAggregate().SubjectFor(event), Event: event, HasOCC: true, ExpectedSeq: 0, FilterSubject: filter}})
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		return err
	}
	return events.ErrConflict
}

// SetupRequired reads authoritative durable state, independent of snapshots or
// projection lag. Config only suppresses setup; it never changes durable state.
// Any user history also closes eligibility for operator/bootstrap-created users,
// even if all those users were later deleted.
func (c *ChattoCore) SetupRequired(ctx context.Context) (bool, error) {
	if c.config.SkipSetupWizard {
		return false, nil
	}
	seq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
	if err != nil {
		return false, err
	}
	if seq == 0 {
		return false, nil
	}
	record, err := c.eventReader.EventAt(ctx, seq)
	if err != nil {
		return false, err
	}
	if record.Event.GetServerSetupOffered() == nil {
		return false, nil
	}
	users, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.UserSubjectFilter())
	return users == 0, err
}

func (c *ChattoCore) requireSetupAvailable(ctx context.Context) error {
	required, err := c.SetupRequired(ctx)
	if err != nil {
		return err
	}
	if !required {
		return ErrSetupUnavailable
	}
	return nil
}

// CompleteServerSetup creates the owner, settings, and completion fact in one
// atomic EVT batch. All eligibility checks run again inside whole-EVT OCC. A lost
// response must be recovered through discovery and normal login, never by
// reopening setup. Required projections catch up before success is returned.
func (c *ChattoCore) CompleteServerSetup(ctx context.Context, input ServerSetupInput) error {
	if err := c.requireSetupAvailable(ctx); err != nil {
		return err
	}
	input.ServerName = strings.TrimSpace(input.ServerName)
	input.Description = strings.TrimSpace(input.Description)
	if input.ServerName == "" {
		return fmt.Errorf("%w: server name is required", ErrInvalidArgument)
	}
	if err := ValidatePassword(input.Password); err != nil {
		return err
	}
	if err := validateServerConfig(&configv1.ServerConfig{ServerName: input.ServerName, Description: input.Description}); err != nil {
		return fmt.Errorf("%w: invalid server settings", ErrInvalidArgument)
	}
	_, err := c.createUserWithOptions(ctx, SystemActorID, input.Login, input.DisplayName, input.Password, userCreationOptions{setup: &input})
	if err != nil {
		return err
	}
	return c.WaitForProjectionsCurrent(ctx)
}

// setupCompletionEntries accompanies the user's creation batch. Settings use
// explicit field events so unrelated boot configuration remains unchanged.
func setupCompletionEntries(userID string, input *ServerSetupInput) []evtstream.BatchEntry {
	entries := rbacSeedEntries(nil, []rbacSeedAssignment{{userID: userID, roleName: RoleOwner}}, nil)
	for _, event := range []*evtv1.Event{
		newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerNameChanged{ServerNameChanged: &evtv1.ServerNameChangedEvent{Name: input.ServerName}}}),
		newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerDescriptionChanged{ServerDescriptionChanged: &evtv1.ServerDescriptionChangedEvent{Description: input.Description}}}),
	} {
		entries = append(entries, evtstream.BatchEntry{Subject: evtstream.ConfigSubjectAggregate(ConfigSubjectServer).SubjectFor(event), Event: event})
	}
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerInitialized{ServerInitialized: &evtv1.ServerInitializedEvent{}}})
	return append(entries, evtstream.BatchEntry{Subject: evtstream.SetupAggregate().SubjectFor(event), Event: event})
}

// CompleteBootstrappedSetup records completion for trusted operator-created
// accounts. It runs after development bootstrap, before the HTTP listener starts.
func (c *ChattoCore) CompleteBootstrappedSetup(ctx context.Context) error {
	users, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.UserSubjectFilter())
	if err != nil || users == 0 {
		return err
	}
	seq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
	if err != nil || seq == 0 {
		return err
	}
	record, err := c.eventReader.EventAt(ctx, seq)
	if err != nil {
		return err
	}
	if record.Event.GetServerInitialized() != nil {
		return nil
	}
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerInitialized{ServerInitialized: &evtv1.ServerInitializedEvent{}}})
	_, err = c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: evtstream.SetupAggregate().SubjectFor(event), Event: event, HasOCC: true, ExpectedSeq: seq, FilterSubject: evtstream.SetupAggregate().AllEventsFilter()}})
	if errors.Is(err, events.ErrConflict) {
		return nil
	}
	return err
}
