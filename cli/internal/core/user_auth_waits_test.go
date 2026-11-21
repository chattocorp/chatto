package core

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"

	"github.com/chattocorp/chatto/cli/internal/config"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
)

// checkWhileHeld stops projector while write runs, and then starts check. It
// fails the test when check returns before the projector continues. Then it
// lets the projector continue and returns the result of check. Use it with
// the projector of a second replica and a write on the first replica: a check
// that does not wait for the event of the write reads the old state and
// returns early.
func checkWhileHeld[T any](t *testing.T, projector *events.Projector, write func(), check func() T) T {
	t.Helper()
	held, release, barrierDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	go func() {
		barrierDone <- projector.WithReadBarrier(func(uint64) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	write()
	checked := make(chan T, 1)
	go func() { checked <- check() }()
	select {
	case got := <-checked:
		t.Fatalf("check returned %v before the projection applied the event", got)
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce()
	if err := <-barrierDone; err != nil {
		t.Fatalf("read barrier: %v", err)
	}
	return <-checked
}

// botReplicas is a bot owner on two replicas of one server.
type botReplicas struct {
	first, second *ChattoCore
	ownerID       string
}

func startBotReplicas(t *testing.T) (*server.Server, botReplicas) {
	t.Helper()
	ns, _ := testutil.StartNATS(t)
	replicas := botReplicas{
		first:  startReplica(t, ns, testCoreConfig()),
		second: startReplica(t, ns, testCoreConfig()),
	}
	ctx := testContext(t)
	owner, err := replicas.first.CreateUser(ctx, SystemActorID, "replica-bot-owner", "Replica Bot Owner", "")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	allowBotCreation(t, ctx, replicas.first, owner.GetId())
	replicas.ownerID = owner.GetId()
	return ns, replicas
}

// newBot creates a bot on the first replica and waits until the second
// replica accepts its API key.
func (r botReplicas) newBot(t *testing.T, login string) *Bot {
	t.Helper()
	ctx := testContext(t)
	bot, err := r.first.CreateBot(ctx, r.ownerID, login, "Replica Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if _, err := r.second.ValidateBotAPIKey(ctx, bot.APIKey); err != nil {
		t.Fatalf("ValidateBotAPIKey on second replica: %v", err)
	}
	return bot
}

// deactivate deactivates an account on the first replica. The command that
// operators use refuses bots, so the test appends the fact.
func (r botReplicas) deactivate(t *testing.T, userID string) {
	t.Helper()
	r.appendUserEvent(t, userID, &evtv1.Event{Event: &evtv1.Event_UserAccountDeactivationChanged{
		UserAccountDeactivationChanged: &evtv1.UserAccountDeactivationChangedEvent{UserId: userID, Deactivated: true},
	}})
}

func (r botReplicas) appendUserEvent(t *testing.T, userID string, event *evtv1.Event) {
	t.Helper()
	if _, err := r.first.appendUserEvent(testContext(t), userID, newEvent(SystemActorID, event), "", nil); err != nil {
		t.Fatalf("append event: %v", err)
	}
}

// accountRemovalEvents returns the events that delete an account in the User
// Auth projection.
func accountRemovalEvents() map[string]func(userID string) *evtv1.Event {
	return map[string]func(userID string) *evtv1.Event{
		"account deleted": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: userID}}}
		},
		"key shredding requested": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserKeyShreddingRequested{UserKeyShreddingRequested: &evtv1.UserKeyShreddingRequestedEvent{UserId: userID}}}
		},
		"key shredded": func(userID string) *evtv1.Event {
			return &evtv1.Event{Event: &evtv1.Event_UserKeyShredded{UserKeyShredded: &evtv1.UserKeyShreddedEvent{UserId: userID}}}
		},
	}
}

// Bot API key authentication must see every change of the bot and of its
// owner that another replica wrote, but it no longer waits for the complete
// User Auth projection. Each case holds the User Auth projection of the
// second replica, so a check that does not wait for the event fails.
func TestBotAPIKeyAuthenticationWaitsForEventFromOtherReplica(t *testing.T) {
	_, r := startBotReplicas(t)
	heldAuth := r.second.userModel.auth.Projector()
	validate := func(t *testing.T, key string) func() error {
		ctx := testContext(t)
		return func() error {
			_, err := r.second.ValidateBotAPIKey(ctx, key)
			return err
		}
	}
	requireRejected := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, ErrAuthTokenNotFound) {
			t.Fatalf("authentication err = %v, want ErrAuthTokenNotFound", err)
		}
	}

	t.Run("bot created", func(t *testing.T) {
		var bot *Bot
		err := checkWhileHeld(t, heldAuth, func() {
			var err error
			if bot, err = r.first.CreateBot(testContext(t), r.ownerID, "created_bot", "Created Bot"); err != nil {
				t.Fatalf("CreateBot: %v", err)
			}
		}, func() error { return validate(t, bot.APIKey)() })
		if err != nil {
			t.Fatalf("new bot key: %v", err)
		}
	})

	t.Run("key added", func(t *testing.T) {
		bot := r.newBot(t, "key_added_bot")
		var issue *BotAPIKeyIssue
		err := checkWhileHeld(t, heldAuth, func() {
			var err error
			if issue, err = r.first.CreateBotAPIKey(testContext(t), r.ownerID, bot.User.GetId(), "second"); err != nil {
				t.Fatalf("CreateBotAPIKey: %v", err)
			}
		}, func() error { return validate(t, issue.Credential)() })
		if err != nil {
			t.Fatalf("added key: %v", err)
		}
	})

	t.Run("legacy key created", func(t *testing.T) {
		// Bot creation writes the first key in the same batch as the account,
		// so this case writes a legacy key creation alone.
		bot := r.newBot(t, "legacy_key_bot")
		key, err := NewBotAPIKey(bot.User.GetId())
		if err != nil {
			t.Fatalf("NewBotAPIKey: %v", err)
		}
		err = checkWhileHeld(t, heldAuth, func() {
			r.appendUserEvent(t, bot.User.GetId(), &evtv1.Event{Event: &evtv1.Event_BotApiKeyCreated{
				BotApiKeyCreated: &evtv1.BotApiKeyCreatedEvent{UserId: bot.User.GetId(), Verifier: r.first.botAPIKeyVerifier(key)},
			}})
		}, validate(t, key))
		if err != nil {
			t.Fatalf("legacy key: %v", err)
		}
	})

	t.Run("key revoked", func(t *testing.T) {
		bot := r.newBot(t, "key_revoked_bot")
		issue, err := r.first.CreateBotAPIKey(testContext(t), r.ownerID, bot.User.GetId(), "second")
		if err != nil {
			t.Fatalf("CreateBotAPIKey: %v", err)
		}
		if err := validate(t, issue.Credential)(); err != nil {
			t.Fatalf("added key before revocation: %v", err)
		}
		requireRejected(t, checkWhileHeld(t, heldAuth, func() {
			if _, err := r.first.RevokeBotAPIKey(testContext(t), r.ownerID, bot.User.GetId(), issue.KeyID); err != nil {
				t.Fatalf("RevokeBotAPIKey: %v", err)
			}
		}, validate(t, issue.Credential)))
	})

	t.Run("key rotated", func(t *testing.T) {
		// The revocation of the first key writes a rotation, because older
		// binaries can read that key.
		bot := r.newBot(t, "key_rotated_bot")
		requireRejected(t, checkWhileHeld(t, heldAuth, func() {
			if _, err := r.first.RevokeBotAPIKey(testContext(t), r.ownerID, bot.User.GetId(), bot.APIKeyID); err != nil {
				t.Fatalf("RevokeBotAPIKey: %v", err)
			}
		}, validate(t, bot.APIKey)))
	})

	t.Run("bot deactivated", func(t *testing.T) {
		bot := r.newBot(t, "deactivated_bot")
		requireRejected(t, checkWhileHeld(t, heldAuth, func() {
			r.deactivate(t, bot.User.GetId())
		}, validate(t, bot.APIKey)))
	})

	for name, remove := range accountRemovalEvents() {
		t.Run("bot "+name, func(t *testing.T) {
			bot := r.newBot(t, strings.ReplaceAll(name, " ", "_")+"_bot")
			requireRejected(t, checkWhileHeld(t, heldAuth, func() {
				r.appendUserEvent(t, bot.User.GetId(), remove(bot.User.GetId()))
			}, validate(t, bot.APIKey)))
		})
	}

	t.Run("owner deactivated", func(t *testing.T) {
		ctx := testContext(t)
		owner, err := r.first.CreateUser(ctx, SystemActorID, "deactivated-owner", "Deactivated Owner", "")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		allowBotCreation(t, ctx, r.first, owner.GetId())
		bot, err := r.first.CreateBot(ctx, owner.GetId(), "owner_deactivated_bot", "Owner Deactivated Bot")
		if err != nil {
			t.Fatalf("CreateBot: %v", err)
		}
		if err := validate(t, bot.APIKey)(); err != nil {
			t.Fatalf("key before owner deactivation: %v", err)
		}
		requireRejected(t, checkWhileHeld(t, heldAuth, func() {
			r.deactivate(t, owner.GetId())
		}, validate(t, bot.APIKey)))
	})

	t.Run("owner reassigned", func(t *testing.T) {
		// The realtime transport registers a revocation watch right after the
		// authentication. The watch reads the owner from the User Auth
		// projection. When that projection still names the old, deactivated
		// owner, the watch closes a valid connection.
		ctx := testContext(t)
		oldOwner, err := r.first.CreateUser(ctx, SystemActorID, "old-owner", "Old Owner", "")
		if err != nil {
			t.Fatalf("CreateUser old owner: %v", err)
		}
		allowBotCreation(t, ctx, r.first, oldOwner.GetId())
		bot, err := r.first.CreateBot(ctx, oldOwner.GetId(), "reassigned_bot", "Reassigned Bot")
		if err != nil {
			t.Fatalf("CreateBot: %v", err)
		}
		manager, err := r.first.CreateUser(ctx, SystemActorID, "bot-manager", "Bot Manager", "")
		if err != nil {
			t.Fatalf("CreateUser manager: %v", err)
		}
		if err := r.first.GrantUserPermission(ctx, SystemActorID, manager.GetId(), PermBotManage); err != nil {
			t.Fatalf("grant bot.manage: %v", err)
		}
		r.deactivate(t, oldOwner.GetId())
		requireRejected(t, validate(t, bot.APIKey)())
		var verifier []byte
		err = checkWhileHeld(t, heldAuth, func() {
			if _, err := r.first.ReassignBotOwner(testContext(t), manager.GetId(), bot.User.GetId(), r.ownerID); err != nil {
				t.Fatalf("ReassignBotOwner: %v", err)
			}
		}, func() error {
			var err error
			_, verifier, err = r.second.ValidateBotAPIKeyCredential(ctx, bot.APIKey)
			return err
		})
		if err != nil {
			t.Fatalf("key after reassignment: %v", err)
		}
		invalidated, stop := r.second.WatchBotAPIKeyInvalidated(bot.User.GetId(), verifier)
		defer stop()
		select {
		case <-invalidated:
			t.Fatal("watch closed for a valid key after reassignment")
		default:
		}
	})
}

// Bot incoming webhook authentication must see every change of the bot's
// webhooks that another replica wrote. Each case holds the User Auth
// projection of the second replica.
func TestBotIncomingWebhookAuthenticationWaitsForEventFromOtherReplica(t *testing.T) {
	_, r := startBotReplicas(t)
	heldAuth := r.second.userModel.auth.Projector()
	validate := func(t *testing.T, credential string) func() error {
		ctx := testContext(t)
		return func() error {
			_, err := r.second.ValidateBotIncomingWebhookCredential(ctx, credential)
			return err
		}
	}
	newWebhook := func(t *testing.T, login string) (*Bot, *BotIncomingWebhookIssue) {
		t.Helper()
		bot := r.newBot(t, login)
		issue, err := r.first.CreateBotIncomingWebhook(testContext(t), r.ownerID, bot.User.GetId(), "hook")
		if err != nil {
			t.Fatalf("CreateBotIncomingWebhook: %v", err)
		}
		if err := validate(t, issue.Credential)(); err != nil {
			t.Fatalf("webhook on second replica: %v", err)
		}
		return bot, issue
	}
	requireRejected := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, ErrAuthTokenNotFound) {
			t.Fatalf("authentication err = %v, want ErrAuthTokenNotFound", err)
		}
	}

	t.Run("webhook created", func(t *testing.T) {
		bot := r.newBot(t, "webhook_created_bot")
		var issue *BotIncomingWebhookIssue
		err := checkWhileHeld(t, heldAuth, func() {
			var err error
			if issue, err = r.first.CreateBotIncomingWebhook(testContext(t), r.ownerID, bot.User.GetId(), "hook"); err != nil {
				t.Fatalf("CreateBotIncomingWebhook: %v", err)
			}
		}, func() error { return validate(t, issue.Credential)() })
		if err != nil {
			t.Fatalf("new webhook: %v", err)
		}
	})

	t.Run("webhook revoked", func(t *testing.T) {
		bot, issue := newWebhook(t, "webhook_revoked_bot")
		requireRejected(t, checkWhileHeld(t, heldAuth, func() {
			if _, err := r.first.RevokeBotIncomingWebhook(testContext(t), r.ownerID, bot.User.GetId(), issue.WebhookID); err != nil {
				t.Fatalf("RevokeBotIncomingWebhook: %v", err)
			}
		}, validate(t, issue.Credential)))
	})

	t.Run("webhook rotated", func(t *testing.T) {
		// Current commands do not write rotations, but replay still applies
		// rotations that an unreleased implementation wrote.
		bot, issue := newWebhook(t, "webhook_rotated_bot")
		rotated, err := NewBotIncomingWebhookCredentialForID(bot.User.GetId(), issue.WebhookID)
		if err != nil {
			t.Fatalf("NewBotIncomingWebhookCredentialForID: %v", err)
		}
		err = checkWhileHeld(t, heldAuth, func() {
			r.appendUserEvent(t, bot.User.GetId(), &evtv1.Event{Event: &evtv1.Event_BotIncomingWebhookRotated{
				BotIncomingWebhookRotated: &evtv1.BotIncomingWebhookRotatedEvent{
					UserId: bot.User.GetId(), WebhookId: issue.WebhookID, Verifier: r.first.botIncomingWebhookVerifier(rotated),
				},
			}})
		}, validate(t, rotated))
		if err != nil {
			t.Fatalf("rotated webhook credential: %v", err)
		}
	})

	for name, remove := range accountRemovalEvents() {
		t.Run("bot "+name, func(t *testing.T) {
			bot, issue := newWebhook(t, "webhook_"+strings.ReplaceAll(name, " ", "_")+"_bot")
			requireRejected(t, checkWhileHeld(t, heldAuth, func() {
				r.appendUserEvent(t, bot.User.GetId(), remove(bot.User.GetId()))
			}, validate(t, issue.Credential)))
		})
	}
}

// Bot authentication reads the owner from the Users projection. The owner is
// known only after the bot read, so the authentication waits for the events
// of the owner in a second step. The test holds the Users projection of the
// second replica while the first replica deactivates the owner.
func TestBotAuthenticationWaitsForOwnerDeactivationFromOtherReplica(t *testing.T) {
	_, r := startBotReplicas(t)
	ctx := testContext(t)
	bot := r.newBot(t, "owner_users_bot")
	webhook, err := r.first.CreateBotIncomingWebhook(ctx, r.ownerID, bot.User.GetId(), "hook")
	if err != nil {
		t.Fatalf("CreateBotIncomingWebhook: %v", err)
	}
	if _, err := r.second.ValidateBotIncomingWebhookCredential(ctx, webhook.Credential); err != nil {
		t.Fatalf("webhook before owner deactivation: %v", err)
	}
	type results struct{ key, webhook error }
	got := checkWhileHeld(t, r.second.userModel.users.Projector(), func() {
		r.deactivate(t, r.ownerID)
	}, func() results {
		_, keyErr := r.second.ValidateBotAPIKey(ctx, bot.APIKey)
		_, webhookErr := r.second.ValidateBotIncomingWebhookCredential(ctx, webhook.Credential)
		return results{keyErr, webhookErr}
	})
	if !errors.Is(got.key, ErrAuthTokenNotFound) || !errors.Is(got.webhook, ErrAuthTokenNotFound) {
		t.Fatalf("authentication after owner deactivation = %+v, want ErrAuthTokenNotFound for both", got)
	}
}

// The bot credential commands decide from the credential lists of the bot.
// A command on the second replica must see a credential that the first
// replica added.
func TestBotCredentialCommandsWaitForEventFromOtherReplica(t *testing.T) {
	_, r := startBotReplicas(t)
	heldAuth := r.second.userModel.auth.Projector()

	t.Run("API key revocation", func(t *testing.T) {
		bot := r.newBot(t, "revoke_command_bot")
		var issue *BotAPIKeyIssue
		err := checkWhileHeld(t, heldAuth, func() {
			var err error
			if issue, err = r.first.CreateBotAPIKey(testContext(t), r.ownerID, bot.User.GetId(), "second"); err != nil {
				t.Fatalf("CreateBotAPIKey: %v", err)
			}
		}, func() error {
			_, err := r.second.RevokeBotAPIKey(testContext(t), r.ownerID, bot.User.GetId(), issue.KeyID)
			return err
		})
		if err != nil {
			t.Fatalf("RevokeBotAPIKey: %v", err)
		}
		if _, err := r.first.ValidateBotAPIKey(testContext(t), issue.Credential); !errors.Is(err, ErrAuthTokenNotFound) {
			t.Fatalf("revoked key: err = %v, want ErrAuthTokenNotFound", err)
		}
	})

	t.Run("incoming webhook revocation", func(t *testing.T) {
		bot := r.newBot(t, "webhook_command_bot")
		var issue *BotIncomingWebhookIssue
		err := checkWhileHeld(t, heldAuth, func() {
			var err error
			if issue, err = r.first.CreateBotIncomingWebhook(testContext(t), r.ownerID, bot.User.GetId(), "hook"); err != nil {
				t.Fatalf("CreateBotIncomingWebhook: %v", err)
			}
		}, func() error {
			_, err := r.second.RevokeBotIncomingWebhook(testContext(t), r.ownerID, bot.User.GetId(), issue.WebhookID)
			return err
		})
		if err != nil {
			t.Fatalf("RevokeBotIncomingWebhook: %v", err)
		}
		if _, err := r.first.ValidateBotIncomingWebhookCredential(testContext(t), issue.Credential); !errors.Is(err, ErrAuthTokenNotFound) {
			t.Fatalf("revoked webhook: err = %v, want ErrAuthTokenNotFound", err)
		}
	})
}

// The outbound webhook credentials are encrypted with a content key of the
// bot. The second replica must find the content key that the first replica
// generated.
func TestOutboundWebhookCredentialsWaitForContentKeyFromOtherReplica(t *testing.T) {
	_, r := startBotReplicas(t)
	type result struct {
		secret string
		err    error
	}
	var bot *Bot
	var secret string
	got := checkWhileHeld(t, r.second.userModel.contentKeys.Projector(), func() {
		ctx := testContext(t)
		var err error
		if bot, err = r.first.CreateBot(ctx, r.ownerID, "outbound_bot", "Outbound Bot"); err != nil {
			t.Fatalf("CreateBot: %v", err)
		}
		if _, secret, err = r.first.CreateBotOutboundWebhook(ctx, r.ownerID, bot.User.GetId(), "Endpoint", "https://example.com/hook", "", false); err != nil {
			t.Fatalf("CreateBotOutboundWebhook: %v", err)
		}
	}, func() result {
		ctx := testContext(t)
		records, _, err := r.first.EventPublisher.SubjectEvents(ctx, "evt.user."+bot.User.GetId()+".bot_outbound_webhook_configured")
		if err != nil || len(records) != 1 {
			return result{err: errors.Join(err, errors.New("want one configuration"))}
		}
		creds, err := r.second.botWebhooks.credentials(ctx, records[0])
		return result{creds.SigningSecret, err}
	})
	if got.err != nil || got.secret != secret {
		t.Fatalf("credentials on second replica: err = %v, secret matches = %v", got.err, got.secret == secret)
	}
}

// The OAuth consent check must see a consent that another replica recorded,
// and must not keep the consent of a deleted account.
func TestOAuthConsentWaitsForEventFromOtherReplica(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	heldAuth := second.userModel.auth.Projector()
	const origin = "https://client.example"
	newUser := func(t *testing.T, login string) string {
		t.Helper()
		user, err := first.CreateUser(testContext(t), SystemActorID, login, login, "")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		return user.GetId()
	}
	hasConsent := func(t *testing.T, userID string, scopes []string) func() bool {
		ctx := testContext(t)
		return func() bool {
			has, err := second.HasOAuthClientScopedConsent(ctx, userID, "client", origin, "", scopes)
			if err != nil {
				t.Errorf("HasOAuthClientScopedConsent: %v", err)
			}
			return has
		}
	}

	for name, scopes := range map[string][]string{"consent": nil, "scoped consent": {"read"}} {
		t.Run(name+" granted", func(t *testing.T) {
			userID := newUser(t, strings.ReplaceAll(name, " ", "-")+"-user")
			if !checkWhileHeld(t, heldAuth, func() {
				if err := first.GrantOAuthClientScopedConsent(testContext(t), userID, "client", "Client", "", origin, "", scopes); err != nil {
					t.Fatalf("GrantOAuthClientScopedConsent: %v", err)
				}
			}, hasConsent(t, userID, scopes)) {
				t.Fatal("consent from other replica not found")
			}
		})
	}

	for name, remove := range accountRemovalEvents() {
		t.Run(name, func(t *testing.T) {
			userID := newUser(t, "consent-"+strings.ReplaceAll(name, " ", "-"))
			if err := first.GrantOAuthClientScopedConsent(testContext(t), userID, "client", "Client", "", origin, "", nil); err != nil {
				t.Fatalf("GrantOAuthClientScopedConsent: %v", err)
			}
			if !hasConsent(t, userID, nil)() {
				t.Fatal("consent before removal not found")
			}
			if checkWhileHeld(t, heldAuth, func() {
				if _, err := first.appendUserEvent(testContext(t), userID, newEvent(SystemActorID, remove(userID)), "", nil); err != nil {
					t.Fatalf("append event: %v", err)
				}
			}, hasConsent(t, userID, nil)) {
				t.Fatal("consent of removed account found")
			}
		})
	}
}

// The password checks must see a password that another replica set.
func TestPasswordChecksWaitForEventFromOtherReplica(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
	heldAuth := second.userModel.auth.Projector()
	ctx := testContext(t)
	user, err := first.CreateUser(ctx, SystemActorID, "replica-password-user", "Replica Password User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if has, err := second.HasPassword(ctx, user.GetId()); err != nil || has {
		t.Fatalf("HasPassword before = %v, %v; want false, nil", has, err)
	}

	if has := checkWhileHeld(t, heldAuth, func() {
		if err := first.SetInitialPasswordHash(testContext(t), user.GetId(), "firstpassword123"); err != nil {
			t.Fatalf("SetInitialPasswordHash: %v", err)
		}
	}, func() bool {
		has, err := second.HasPassword(ctx, user.GetId())
		if err != nil {
			t.Errorf("HasPassword: %v", err)
		}
		return has
	}); !has {
		t.Fatal("HasPassword = false after the first replica set a password")
	}

	if err := checkWhileHeld(t, heldAuth, func() {
		if err := first.SetPasswordHash(testContext(t), user.GetId(), "newpassword456"); err != nil {
			t.Fatalf("SetPasswordHash: %v", err)
		}
	}, func() error { return second.VerifyUserPassword(ctx, user.GetId(), "newpassword456") }); err != nil {
		t.Fatalf("VerifyUserPassword with new password: %v", err)
	}

	for name, remove := range accountRemovalEvents() {
		t.Run(name, func(t *testing.T) {
			ctx := testContext(t)
			removed, err := first.CreateUser(ctx, SystemActorID, "password-"+strings.ReplaceAll(name, " ", "-"), "Removed", "password123")
			if err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			if err := second.VerifyUserPassword(ctx, removed.GetId(), "password123"); err != nil {
				t.Fatalf("VerifyUserPassword before removal: %v", err)
			}
			if err := checkWhileHeld(t, heldAuth, func() {
				if _, err := first.appendUserEvent(testContext(t), removed.GetId(), newEvent(SystemActorID, remove(removed.GetId())), "", nil); err != nil {
					t.Fatalf("append event: %v", err)
				}
			}, func() error { return second.VerifyUserPassword(ctx, removed.GetId(), "password123") }); err == nil {
				t.Fatal("VerifyUserPassword accepted the password of a removed account")
			}
		})
	}
}

// The reads of the external identity list of one user must see a link or an
// unlink that another replica wrote.
func TestExternalIdentityReadsWaitForEventFromOtherReplica(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	cfg := testCoreConfig()
	// Identities of the provider github-main manage the username.
	cfg.AuthProviders = []config.AuthProviderConfig{{ID: "github-main", Type: "github", ManageUsernames: true}}
	first := startReplica(t, ns, cfg)
	second := startReplica(t, ns, cfg)
	heldAuth := second.userModel.auth.Projector()
	newUser := func(t *testing.T, login string) string {
		t.Helper()
		user, err := first.CreateUser(testContext(t), SystemActorID, login, login, "")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		return user.GetId()
	}
	link := func(t *testing.T, issuer, subject, userID string) {
		t.Helper()
		if err := first.LinkExternalIdentity(testContext(t), issuer, issuer, issuer, subject, userID); err != nil {
			t.Fatalf("LinkExternalIdentity: %v", err)
		}
	}
	identityCount := func(t *testing.T, userID string) func() int {
		ctx := testContext(t)
		return func() int {
			identities, err := second.ExternalIdentitiesForUser(ctx, userID)
			if err != nil {
				t.Errorf("ExternalIdentitiesForUser: %v", err)
			}
			return len(identities)
		}
	}

	t.Run("identity linked", func(t *testing.T) {
		userID := newUser(t, "linked-identity-user")
		if got := checkWhileHeld(t, heldAuth, func() { link(t, "github-main", "1", userID) }, identityCount(t, userID)); got != 1 {
			t.Fatalf("identities = %d, want 1", got)
		}
	})

	t.Run("OIDC subject linked", func(t *testing.T) {
		userID := newUser(t, "oidc-identity-user")
		got := checkWhileHeld(t, heldAuth, func() {
			if _, err := first.appendUserEvent(testContext(t), userID, newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserOidcSubjectLinked{
				UserOidcSubjectLinked: &evtv1.UserOIDCSubjectLinkedEvent{UserId: userID, Issuer: "https://oidc.example", Subject: "2"},
			}}), "", nil); err != nil {
				t.Fatalf("append event: %v", err)
			}
		}, identityCount(t, userID))
		if got != 1 {
			t.Fatalf("identities = %d, want 1", got)
		}
	})

	t.Run("identity unlinked", func(t *testing.T) {
		userID := newUser(t, "unlinked-identity-user")
		link(t, "github-main", "3", userID)
		link(t, "discord-main", "4", userID)
		if got := identityCount(t, userID)(); got != 2 {
			t.Fatalf("identities before unlink = %d, want 2", got)
		}
		identities := first.userModel.externalIdentities(userID)
		policy := ExternalIdentitySignInPolicy{Issuers: []string{"github-main", "discord-main"}}
		if got := checkWhileHeld(t, heldAuth, func() {
			if err := first.DisconnectExternalIdentityAs(testContext(t), SystemActorID, userID, identities[0].SubjectHash, policy); err != nil {
				t.Fatalf("DisconnectExternalIdentityAs: %v", err)
			}
		}, identityCount(t, userID)); got != 1 {
			t.Fatalf("identities after unlink = %d, want 1", got)
		}
	})

	for name, remove := range accountRemovalEvents() {
		t.Run(name, func(t *testing.T) {
			userID := newUser(t, "identity-"+strings.ReplaceAll(name, " ", "-"))
			link(t, "github-main", name, userID)
			if got := identityCount(t, userID)(); got != 1 {
				t.Fatalf("identities before removal = %d, want 1", got)
			}
			got := checkWhileHeld(t, heldAuth, func() {
				if _, err := first.appendUserEvent(testContext(t), userID, newEvent(SystemActorID, remove(userID)), "", nil); err != nil {
					t.Fatalf("append event: %v", err)
				}
			}, func() int {
				// ExternalIdentitiesForUser rejects a removed account from the
				// Users projection first. The provider group role check reads
				// the identity list without that check.
				if err := second.userModel.waitForUserAuthSubjectsCurrent(testContext(t), "test", userExternalIdentityAuthSubjects(userID)...); err != nil {
					t.Errorf("wait: %v", err)
				}
				return len(second.userModel.externalIdentities(userID))
			})
			if got != 0 {
				t.Fatalf("identities of removed account = %d, want 0", got)
			}
		})
	}

	t.Run("username policy", func(t *testing.T) {
		userID := newUser(t, "managed-username-user")
		managed := checkWhileHeld(t, heldAuth, func() { link(t, "github-main", "5", userID) }, func() bool {
			managed, err := second.UsernameManaged(testContext(t), userID)
			if err != nil {
				t.Errorf("UsernameManaged: %v", err)
			}
			return managed
		})
		if !managed {
			t.Fatal("UsernameManaged = false after the first replica linked a managing provider")
		}
	})

	t.Run("unlink with identity from other replica", func(t *testing.T) {
		userID := newUser(t, "unlink-second-identity-user")
		link(t, "github-main", "6", userID)
		identities, err := second.ExternalIdentitiesForUser(testContext(t), userID)
		if err != nil || len(identities) != 1 {
			t.Fatalf("identities before = %v, %v", identities, err)
		}
		// Without the new identity, the unlink removes the last method.
		policy := ExternalIdentitySignInPolicy{Issuers: []string{"github-main", "discord-main"}}
		if err := checkWhileHeld(t, heldAuth, func() { link(t, "discord-main", "7", userID) }, func() error {
			return second.DisconnectExternalIdentityAs(testContext(t), SystemActorID, userID, identities[0].SubjectHash, policy)
		}); err != nil {
			t.Fatalf("DisconnectExternalIdentityAs: %v", err)
		}
	})

	t.Run("unlink with password from other replica", func(t *testing.T) {
		userID := newUser(t, "unlink-password-user")
		link(t, "github-main", "8", userID)
		identities, err := second.ExternalIdentitiesForUser(testContext(t), userID)
		if err != nil || len(identities) != 1 {
			t.Fatalf("identities before = %v, %v", identities, err)
		}
		policy := ExternalIdentitySignInPolicy{PasswordLoginEnabled: true, Issuers: []string{"github-main"}}
		if err := checkWhileHeld(t, heldAuth, func() {
			if err := first.SetInitialPasswordHash(testContext(t), userID, "password123"); err != nil {
				t.Fatalf("SetInitialPasswordHash: %v", err)
			}
		}, func() error {
			return second.DisconnectExternalIdentityAs(testContext(t), SystemActorID, userID, identities[0].SubjectHash, policy)
		}); err != nil {
			t.Fatalf("DisconnectExternalIdentityAs: %v", err)
		}
	})

	t.Run("provider group roles", func(t *testing.T) {
		userID := newUser(t, "provider-roles-user")
		source := ProviderRoleSource{ProviderID: "github-main", Issuer: "github-main"}
		if err := checkWhileHeld(t, heldAuth, func() { link(t, "github-main", "9", userID) }, func() error {
			return second.revokeProviderGroupRolesWithoutIdentity(testContext(t), userID, source)
		}); err != nil {
			t.Fatalf("revokeProviderGroupRolesWithoutIdentity: %v", err)
		}
	})
}

// Bot API key authentication runs for every bot request, so its cost in
// requests to the stream leader matters. It reads positions only of subjects
// of the bot and of its owner, and no wildcard filter across all users.
func TestBotAPIKeyAuthenticationReadsOnlySubjectsOfBotAndOwner(t *testing.T) {
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "read-budget-owner", "Read Budget Owner", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	allowBotCreation(t, ctx, c, owner.GetId())
	bot, err := c.CreateBot(ctx, owner.GetId(), "read_budget_bot", "Read Budget Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	botID := bot.User.GetId()

	sub, err := nc.SubscribeSync("$JS.API.STREAM.MSG.GET.EVT")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, bot.APIKey); err != nil {
		t.Fatalf("ValidateBotAPIKey: %v", err)
	}
	// The authentication has the replies to its requests, so the server has
	// sent the requests to this subscription already. The flush returns
	// after them.
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	pending, _, err := sub.Pending()
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	var got []string
	for range pending {
		msg, err := sub.NextMsg(time.Second)
		if err != nil {
			t.Fatalf("next request: %v", err)
		}
		var request struct {
			LastBySubject string `json:"last_by_subj"`
		}
		if err := json.Unmarshal(msg.Data, &request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// Background source checks read wildcard filters of the form
		// evt.user.*.{type} at an interval. They cannot be told apart from a
		// wildcard read of the authentication, so the test examines only the
		// reads of subjects of one user.
		if subject := request.LastBySubject; strings.HasPrefix(subject, "evt.user.") && !strings.HasPrefix(subject, "evt.user.*.") {
			got = append(got, subject)
		}
	}
	want := []string{"evt.user." + botID + ".>", "evt.user." + owner.GetId() + ".>", "evt.user." + owner.GetId() + ".account_deactivation_changed"}
	for _, eventType := range []string{
		"account_created", "account_deleted", "user_key_shredding_requested", "user_key_shredded",
		"bot_api_key_created", "bot_api_key_rotated", "bot_api_key_added", "bot_api_key_revoked",
		"account_deactivation_changed", "bot_owner_reassigned",
	} {
		want = append(want, "evt.user."+botID+"."+eventType)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("position reads of bot and owner subjects = %v, want %v", got, want)
	}
}
