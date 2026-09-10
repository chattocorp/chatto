package core

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/testutil"

	"hmans.de/chatto/internal/evtstream"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func setupInput(login string) ServerSetupInput {
	return ServerSetupInput{ServerName: "Test community", Description: "A place to talk", Login: login, DisplayName: "First owner", Password: "correct-horse-battery"}
}

func TestServerSetupLifecycle(t *testing.T) {
	c, nc := setupTestCore(t)
	c.config.SkipSetupWizard = false
	ctx := testContext(t)
	if required, err := c.SetupRequired(ctx); err != nil || !required {
		t.Fatalf("initial setup = %v, %v", required, err)
	}
	if err := c.CompleteServerSetup(ctx, setupInput("founder")); err != nil {
		t.Fatal(err)
	}
	user, err := c.VerifyPassword(ctx, "founder", setupInput("founder").Password)
	if err != nil {
		t.Fatal(err)
	}
	if owner, err := c.IsServerOwner(ctx, user.Id); err != nil || !owner {
		t.Fatal("initial account is not owner")
	}
	cfg := c.ConfigModel().GetServerConfig()
	if cfg.GetServerName() != "Test community" || cfg.GetDescription() != "A place to talk" {
		t.Fatal("settings not visible after completion")
	}
	if required, err := c.SetupRequired(ctx); err != nil || required {
		t.Fatalf("completed setup = %v, %v", required, err)
	}
	if err := c.CompleteServerSetup(ctx, ServerSetupInput{}); !errors.Is(err, ErrSetupUnavailable) {
		t.Fatalf("second submission = %v", err)
	}
	if err := c.DeleteUser(ctx, SystemActorID, user.Id); err != nil {
		t.Fatal(err)
	}
	replica, err := NewChattoCore(ctx, nc, c.config)
	if err != nil {
		t.Fatal(err)
	}
	if required, err := replica.SetupRequired(ctx); err != nil || required {
		t.Fatalf("restart setup = %v, %v", required, err)
	}
}

func TestServerSetupConcurrentReplicas(t *testing.T) {
	first, nc := setupTestCore(t)
	first.config.SkipSetupWizard = false
	ctx := testContext(t)
	second, err := NewChattoCore(ctx, nc, first.config)
	if err != nil {
		t.Fatal(err)
	}
	startCoreServices(t, second)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, c := range []*ChattoCore{first, second} {
		wg.Add(1)
		go func(i int, c *ChattoCore) {
			defer wg.Done()
			results <- c.CompleteServerSetup(ctx, setupInput([]string{"founderone", "foundertwo"}[i]))
		}(i, c)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrSetupUnavailable) {
			t.Fatalf("unexpected loser error: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("successful submissions = %d", success)
	}
}

func TestServerSetupSkipAndValidation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	before, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CompleteServerSetup(ctx, ServerSetupInput{}); !errors.Is(err, ErrSetupUnavailable) {
		t.Fatalf("disabled setup = %v", err)
	}
	c.config.SkipSetupWizard = false
	for _, input := range []ServerSetupInput{{}, {ServerName: "Test", Password: "short"}, {ServerName: "Test", Password: "password123", Login: "admin", DisplayName: "Owner"}} {
		if err := c.CompleteServerSetup(ctx, input); err == nil {
			t.Fatal("invalid setup accepted")
		}
	}
	after, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
	if err != nil || after != before {
		t.Fatalf("setup state changed: %d => %d (%v)", before, after, err)
	}
	if required, err := c.SetupRequired(ctx); err != nil || !required {
		t.Fatalf("pending state lost: %v %v", required, err)
	}
}

func TestServerSetupExistingHistory(t *testing.T) {
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	// Remove only the new marker to represent an installation made by an old
	// binary. Existing RBAC and room-group events must close setup on upgrade.
	seq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.storage.serverEvtStream.DeleteMsg(ctx, seq); err != nil {
		t.Fatal(err)
	}
	cfg := c.config
	cfg.SkipSetupWizard = false
	upgraded, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if required, err := upgraded.SetupRequired(ctx); err != nil || required {
		t.Fatalf("upgrade reopened setup: %v %v", required, err)
	}
}

func TestServerSetupOperatorAccountClosesEligibility(t *testing.T) {
	c, _ := setupTestCore(t)
	c.config.SkipSetupWizard = false
	ctx := context.Background()
	if _, err := c.CreateUser(ctx, SystemActorID, "operatoruser", "Operator", "password123"); err != nil {
		t.Fatal(err)
	}
	if required, err := c.SetupRequired(ctx); err != nil || required {
		t.Fatalf("operator account left setup open: %v %v", required, err)
	}
	if err := c.CompleteBootstrappedSetup(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestServerSetupBlocksPublicRegistration(t *testing.T) {
	c, _ := setupTestCore(t)
	c.config.SkipSetupWizard = false
	ctx := testContext(t)
	// HTTP registration uses the system actor; it must not bypass setup.
	if _, err := c.CreateVerifiedUser(ctx, SystemActorID, "invalid", "", "", "person@example.com"); !errors.Is(err, ErrSetupRequired) {
		t.Fatalf("public registration = %v", err)
	}
	if required, err := c.SetupRequired(ctx); err != nil || !required {
		t.Fatalf("registration changed eligibility: %v %v", required, err)
	}
}

func TestServerSetupOnlyReplacesSelectedSettings(t *testing.T) {
	c, _ := setupTestCore(t)
	c.config.SkipSetupWizard = false
	ctx := testContext(t)
	if err := c.ConfigModel().SetServerConfig(ctx, SystemActorID, &configv1.ServerConfig{Description: "Previous", WelcomeMessage: "Welcome"}); err != nil {
		t.Fatal(err)
	}
	input := setupInput("founder")
	input.Description = ""
	if err := c.CompleteServerSetup(ctx, input); err != nil {
		t.Fatal(err)
	}
	cfg := c.ConfigModel().GetServerConfig()
	if cfg.Description != "" || cfg.WelcomeMessage != "Welcome" {
		t.Fatal("setup did not preserve unselected settings or clear selected description")
	}
}

func TestServerSetupLegacyHistoryStaysClosed(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "SERVER_EVENTS", Subjects: []string{"server.>"}}); err != nil {
		t.Fatal(err)
	}
	c, err := NewChattoCore(ctx, nc, config.CoreConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if required, err := c.SetupRequired(ctx); err != nil || required {
		t.Fatalf("legacy server reopened setup: %v %v", required, err)
	}
}

// setupTrafficStream inserts an unrelated durable write after each Info read,
// making a whole-stream OCC decision stale on every attempt.
type setupTrafficStream struct {
	jetstream.Stream
	afterInfo func() error
}

func (s setupTrafficStream) Info(ctx context.Context, opts ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	info, err := s.Stream.Info(ctx, opts...)
	if err != nil {
		return nil, err
	}
	if err := s.afterInfo(); err != nil {
		return nil, err
	}
	return info, nil
}

func TestServerSetupUpgradeDoesNotContendWithChatTraffic(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	seq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.storage.serverEvtStream.DeleteMsg(ctx, seq); err != nil {
		t.Fatal(err)
	}
	c.storage.serverEvtStream = setupTrafficStream{Stream: c.storage.serverEvtStream, afterInfo: func() error {
		event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerNameChanged{ServerNameChanged: &evtv1.ServerNameChangedEvent{Name: "Traffic"}}})
		_, err := c.EventPublisher.Append(ctx, evtstream.ConfigSubjectAggregate(ConfigSubjectServer).SubjectFor(event), event)
		return err
	}}
	if err := c.initializeServerSetup(ctx); err != nil {
		t.Fatalf("upgrade competed with unrelated traffic: %v", err)
	}
}
