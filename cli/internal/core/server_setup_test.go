package core

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/natsresources"
	configv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/config/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
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
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	first.config.SkipSetupWizard = false
	ctx := testContext(t)
	second := startReplica(t, ns, first.config)
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

// setupTrafficJetStream inserts an unrelated durable write after each EVT
// information read, making a whole-stream OCC decision stale on every attempt.
type setupTrafficJetStream struct {
	jetstream.JetStream
	afterInfo func() error
}

func (s setupTrafficJetStream) Stream(ctx context.Context, name string) (jetstream.Stream, error) {
	stream, err := s.JetStream.Stream(ctx, name)
	if err != nil || name != natsresources.EVT {
		return stream, err
	}
	if err := s.afterInfo(); err != nil {
		return nil, err
	}
	return stream, nil
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
	c.js = setupTrafficJetStream{JetStream: c.js, afterInfo: func() error {
		event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ServerNameChanged{ServerNameChanged: &evtv1.ServerNameChangedEvent{Name: "Traffic"}}})
		_, err := c.EventPublisher.AppendEventually(ctx, evtstream.ConfigSubjectAggregate(ConfigSubjectServer).SubjectFor(event), event)
		return err
	}}
	if err := c.initializeServerSetup(ctx); err != nil {
		t.Fatalf("upgrade competed with unrelated traffic: %v", err)
	}
}

func TestConfiguredOwnerSetupMatrix(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	c.config.SkipSetupWizard = false
	for _, disabled := range []bool{false, true} {
		for _, owners := range []config.OwnersConfig{{}, {Emails: []string{"owner@example.test"}}, {DIDs: []string{"did:web:example.test"}}} {
			c.config.AuthDirectLoginDisabled = disabled
			c.config.Owners = owners
			got, err := c.SetupRequired(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := !(disabled && owners.Configured())
			if got != want {
				t.Fatalf("disabled=%v owners=%v: got %v", disabled, owners.Configured(), got)
			}
		}
	}
}

func TestConfiguredOwnerDIDProviderBoundary(t *testing.T) {
	c := &ChattoCore{config: config.CoreConfig{Owners: config.OwnersConfig{DIDs: []string{"did:web:owner.example"}}, AuthProviders: []config.AuthProviderConfig{{ID: "internet", Type: "atproto"}}}}
	for _, tc := range []struct {
		kind, issuer string
		want         bool
	}{{"atproto", "internet", true}, {"oidc", "internet", false}, {"atproto", "removed", false}} {
		if got := c.isConfiguredOwnerIdentity(ExternalIdentity{ProviderType: tc.kind, Issuer: tc.issuer, Subject: "did:web:owner.example"}); got != tc.want {
			t.Fatalf("kind=%s issuer=%s got=%v", tc.kind, tc.issuer, got)
		}
	}
}

func TestDIDOwnerReconciledAtReplicaBoot(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	cfg := testCoreConfig()
	cfg.AuthProviders = []config.AuthProviderConfig{{ID: "internet", Type: "atproto"}}
	first := startReplica(t, ns, cfg)
	ctx := testContext(t)
	user, err := first.CreateUser(ctx, SystemActorID, "founder", "Founder", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.LinkExternalIdentity(ctx, "internet", "atproto", "internet", "did:web:owner.example", user.Id); err != nil {
		t.Fatal(err)
	}
	if err := first.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.Owners.DIDs = []string{"did:web:owner.example"}
	second := startReplica(t, ns, cfg)
	owner, err := second.IsServerOwner(ctx, user.Id)
	if err != nil || !owner {
		t.Fatalf("boot owner=%v err=%v", owner, err)
	}
}

func TestConfiguredOwnerCreationCompletesSetup(t *testing.T) {
	for _, mode := range []string{"did", "email", "skip", "bootstrap", "old", "wizard", "non-owner"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			if mode == "wizard" {
				c.config.SkipSetupWizard = false
				if err := c.CompleteServerSetup(ctx, setupInput("wizardowner")); err != nil {
					t.Fatal(err)
				}
			}
			c.config.SkipSetupWizard = mode == "skip"
			c.config.AuthDirectLoginDisabled = true
			c.config.AuthProviders = []config.AuthProviderConfig{{ID: "internet", Type: "atproto"}}
			c.config.Owners.DIDs = []string{"did:web:owner.example"}
			if mode == "email" {
				c.config.Owners.DIDs = nil
				c.config.Owners.Emails = []string{"owner@example.test"}
			}
			if mode == "bootstrap" || mode == "old" {
				if err := c.ExcludeBootstrapOwnerWelcome(ctx); err != nil {
					t.Fatal(err)
				}
			}
			flow := &PendingExternalIdentityFlow{Kind: ExternalIdentityFlowKindCreate, ProviderID: "internet", ProviderType: "atproto", Issuer: "internet", Subject: "did:web:owner.example", VerifiedEmail: "owner@example.test"}
			if mode == "non-owner" {
				flow.Subject = "did:web:member.example"
			}
			user, err := c.CreateUserForExternalIdentity(ctx, "founder", "Founder", flow)
			if err != nil {
				t.Fatal(err)
			}
			want := mode == "did" || mode == "email"
			if flow.OwnerWelcome != want {
				t.Fatalf("welcome=%v want=%v", flow.OwnerWelcome, want)
			}
			owner, err := c.IsServerOwner(ctx, user.Id)
			if err != nil || owner != (mode != "non-owner") {
				t.Fatalf("owner=%v err=%v", owner, err)
			}
			seq, err := c.EventPublisher.LastSubjectSeq(ctx, evtstream.SetupAggregate().AllEventsFilter())
			if err != nil {
				t.Fatal(err)
			}
			record, err := c.eventReader.EventAt(ctx, seq)
			if err != nil {
				t.Fatal(err)
			}
			if want && record.Event.GetServerInitialized() == nil {
				t.Fatal("creation did not complete setup")
			}
			c.config.Owners = config.OwnersConfig{}
			if err := c.applyConfigOwners(ctx); err != nil {
				t.Fatal(err)
			}
			owner, err = c.IsServerOwner(ctx, user.Id)
			if err != nil || owner != (mode != "non-owner") {
				t.Fatal("removal changed owner")
			}
		})
	}
}

func TestConfiguredOwnerConcurrentCreationWelcome(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	cfg := testCoreConfig()
	cfg.SkipSetupWizard = false
	cfg.AuthDirectLoginDisabled = true
	cfg.AuthProviders = []config.AuthProviderConfig{{ID: "internet", Type: "atproto"}}
	cfg.Owners.DIDs = []string{"did:web:first.example", "did:web:second.example"}
	first := startReplica(t, ns, cfg)
	second := startReplica(t, ns, cfg)
	ctx := testContext(t)
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, c := range []*ChattoCore{first, second} {
		wg.Go(func() {
			name := []string{"first", "second"}[i]
			flow := &PendingExternalIdentityFlow{Kind: ExternalIdentityFlowKindCreate, ProviderID: "internet", ProviderType: "atproto", Issuer: "internet", Subject: cfg.Owners.DIDs[i]}
			_, err := c.CreateUserForExternalIdentity(ctx, name, name, flow)
			errs <- err
			results <- flow.OwnerWelcome
		})
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for welcome := range results {
		if welcome {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("welcomes=%d", count)
	}
}

func TestDIDOwnerGrantOnExistingAccountLink(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "internet", Type: "atproto"}}
	c.config.Owners.DIDs = []string{"did:web:owner.example"}
	user, err := c.CreateUser(ctx, SystemActorID, "founder", "Founder", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LinkExternalIdentity(ctx, "internet", "atproto", "internet", "did:web:owner.example", user.Id); err != nil {
		t.Fatal(err)
	}
	owner, err := c.IsServerOwner(ctx, user.Id)
	if err != nil || !owner {
		t.Fatalf("owner=%v err=%v", owner, err)
	}
}
