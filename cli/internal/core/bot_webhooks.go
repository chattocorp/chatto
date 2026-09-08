package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	logv1 "hmans.de/chatto/internal/pb/chatto/core/log/v1"
	"hmans.de/chatto/pkg/events"
)

// BotOutboundWebhook exposes the saved destination only to bot managers.
// Authorization credentials and signing secrets remain write-only.
type BotOutboundWebhook struct {
	ID               string
	Name             string // Human label, encrypted alongside endpoint credentials.
	CreatedAt        time.Time
	URL              string // Saved destination; can include tool credentials and must not be logged.
	Enabled          bool   // Accepts new messages; resume does not recover earlier work.
	HasAuthorization bool
	Latest           *logv1.Entry // Latest retained failure; absence is not proof of delivery.
}
type botWebhookCredentials struct {
	Name          string `json:"name,omitempty"`
	URL           string `json:"url"`
	Authorization string `json:"authorization"`
	SigningSecret string `json:"signing_secret"`
}

// ListBotOutboundWebhooks returns all endpoints, including paused endpoints,
// only to a bot manager. The collection is bounded to 20 non-revoked credentials.
func (c *ChattoCore) ListBotOutboundWebhooks(ctx context.Context, actorID, botID string) ([]*BotOutboundWebhook, error) {
	if err := c.authorizeAtStableInputs(ctx, func() error { _, err := c.requireBotManager(ctx, actorID, botID); return err }); err != nil {
		return nil, err
	}
	if err := c.botWebhooks.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	result := []*BotOutboundWebhook{}
	for _, endpoint := range c.botWebhooks.projection.Projection().list(botID) {
		item, err := c.botWebhooks.metadata(ctx, endpoint)
		if err != nil {
			return nil, err
		}
		item.Latest, err = c.latestOperationalLog(ctx, botWebhookLogFilter(botID, item.ID))
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// GetBotOutboundWebhook reads one endpoint within a managed bot's collection.
func (c *ChattoCore) GetBotOutboundWebhook(ctx context.Context, actorID, botID, webhookID string) (*BotOutboundWebhook, error) {
	endpoint, err := c.readBotOutboundWebhook(ctx, actorID, botID, webhookID)
	if err != nil {
		return nil, err
	}
	item, err := c.botWebhooks.metadata(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	item.Latest, err = c.latestOperationalLog(ctx, botWebhookLogFilter(botID, webhookID))
	return item, err
}

// CreateBotOutboundWebhook creates an independent endpoint and
// returns its signing secret once. Paused endpoints count toward the limit.
func (c *ChattoCore) CreateBotOutboundWebhook(ctx context.Context, actorID, botID, name, rawURL, authorization string, enabled bool) (*BotOutboundWebhook, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return nil, "", invalidArgument("outbound webhook name must contain 1 to 64 characters")
	}
	if err := validateBotWebhookURL(rawURL); err != nil {
		return nil, "", err
	}
	if len(authorization) > 4096 || strings.ContainsAny(authorization, "\r\n\x00") {
		return nil, "", invalidArgument("invalid outbound webhook authorization header")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, "", err
	}
	creds := &botWebhookCredentials{Name: name, URL: rawURL, Authorization: authorization, SigningSecret: base64.RawURLEncoding.EncodeToString(secret)}
	result, err := c.botWebhooks.mutate(ctx, actorID, botID, NewBotOutboundWebhookID(), creds, &enabled, false, nil)
	if err != nil {
		return nil, "", err
	}
	return result, creds.SigningSecret, nil
}

// BotOutboundWebhookPatch preserves omitted fields. Empty Authorization removes it.
// Name and signing secret cannot be changed through a patch.
type BotOutboundWebhookPatch struct {
	Enabled       *bool
	URL           *string
	Authorization *string
}

// UpdateBotOutboundWebhook changes the supplied settings without rotating the secret.
// Changes cancel queued work; an HTTP request already in flight can still finish.
func (c *ChattoCore) UpdateBotOutboundWebhook(ctx context.Context, actorID, botID, webhookID string, patch BotOutboundWebhookPatch) (*BotOutboundWebhook, error) {
	if patch.URL != nil {
		if err := validateBotWebhookURL(*patch.URL); err != nil {
			return nil, err
		}
	}
	if patch.Authorization != nil && (len(*patch.Authorization) > 4096 || strings.ContainsAny(*patch.Authorization, "\r\n\x00")) {
		return nil, invalidArgument("invalid outbound webhook authorization header")
	}
	return c.botWebhooks.mutate(ctx, actorID, botID, webhookID, nil, patch.Enabled, false, &patch)
}

// RevokeBotOutboundWebhook permanently removes one credential. Already absent
// endpoints are successful no-ops; an in-flight HTTP request may still finish.
func (c *ChattoCore) RevokeBotOutboundWebhook(ctx context.Context, actorID, botID, webhookID string) error {
	_, err := c.botWebhooks.mutate(ctx, actorID, botID, webhookID, nil, nil, true, nil)
	return err
}

func (m *botWebhookModel) metadata(ctx context.Context, endpoint *botWebhookEndpoint) (*BotOutboundWebhook, error) {
	creds, err := m.credentials(ctx, endpoint.Configuration)
	if err != nil {
		return nil, err
	}
	return &BotOutboundWebhook{ID: endpoint.Configuration.GetBotOutboundWebhookConfigured().GetWebhookId(), Name: creds.Name, URL: creds.URL, CreatedAt: endpoint.CreatedAt, Enabled: endpoint.Enabled, HasAuthorization: creds.Authorization != ""}, nil
}

func validateBotWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u == nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(linkpreview.IsLocalhostHostname(u.Hostname()) && u.Scheme == "http")) {
		return invalidArgument("outbound webhook requires HTTPS (HTTP is allowed for localhost names), without user information or fragment")
	}
	return nil
}

// configurationEvent encrypts all endpoint settings with the new event identity.
func (m *botWebhookModel) configurationEvent(ctx context.Context, actorID, botID, webhookID string, creds *botWebhookCredentials, enabled bool) (*evtv1.Event, error) {
	dek, err := m.core.ensureActiveUserPIIDEK(ctx, botID)
	if err != nil {
		return nil, err
	}
	cfg := &evtv1.BotOutboundWebhookConfiguredEvent{BotUserId: botID, WebhookId: webhookID, Enabled: enabled, Independent: true}
	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_BotOutboundWebhookConfigured{BotOutboundWebhookConfigured: cfg}})
	data, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}
	cfg.Credentials, err = encryptUserPIIStringWithDEK(dek, event.GetId(), botID, "bot_outbound_webhook_configured", "credentials", string(data))
	if err != nil {
		return nil, err
	}
	return event, nil
}

// mutate serializes collection limits and endpoint lifecycle through the bot's
// user aggregate. Authorization and projection state are checked on every retry.
func (m *botWebhookModel) mutate(ctx context.Context, actorID, botID, webhookID string, creds *botWebhookCredentials, enabled *bool, revoke bool, patch *BotOutboundWebhookPatch) (*BotOutboundWebhook, error) {
	if _, err := m.core.requireBotManager(ctx, actorID, botID); err != nil {
		return nil, err
	}
	var creation *evtv1.Event
	if creds != nil {
		var err error
		creation, err = m.configurationEvent(ctx, actorID, botID, webhookID, creds, *enabled)
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 10; attempt++ {
		filter := evtstream.UserAggregate(botID).AllEventsFilter()
		seq, err := m.core.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, err
		}
		if err = m.core.userModel.waitForUsers(ctx, events.SubjectPosition(filter, seq)); err != nil {
			return nil, err
		}
		if err = m.projection.Projector().WaitForCurrent(ctx); err != nil {
			return nil, err
		}
		if err = m.core.authorizeAtStableInputs(ctx, func() error { _, err := m.core.requireBotManager(ctx, actorID, botID); return err }); err != nil {
			return nil, err
		}
		current := m.projection.Projection().get(botID, webhookID)
		event := creation
		if creation != nil {
			if len(m.projection.Projection().list(botID)) >= 20 {
				return nil, invalidArgument("outbound webhook limit reached")
			}
		} else {
			if current == nil {
				if revoke {
					return nil, nil
				}
				return nil, ErrNotFound
			}
			nextEnabled := current.Enabled
			if enabled != nil {
				nextEnabled = *enabled
			}
			var changedCredentials *botWebhookCredentials
			if patch != nil && (patch.URL != nil || patch.Authorization != nil) {
				fresh, err := m.credentials(ctx, current.Configuration)
				if err != nil {
					return nil, err
				}
				previous := fresh
				if patch.URL != nil {
					fresh.URL = *patch.URL
				}
				if patch.Authorization != nil {
					fresh.Authorization = *patch.Authorization
				}
				if fresh != previous {
					changedCredentials = &fresh
				}
			}
			if !revoke && nextEnabled == current.Enabled && changedCredentials == nil {
				return m.metadata(ctx, current)
			}
			if changedCredentials != nil {
				// Re-encrypt with this fact's AAD and current key; each OCC retry
				// reapplies only the supplied fields to fresh projected settings.
				event, err = m.configurationEvent(ctx, actorID, botID, webhookID, changedCredentials, nextEnabled)
				if err != nil {
					return nil, err
				}
			} else {
				state := &evtv1.BotOutboundWebhookStateChangedEvent{BotUserId: botID, WebhookId: webhookID, Revoked: revoke, Enabled: nextEnabled}
				event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_BotOutboundWebhookStateChanged{BotOutboundWebhookStateChanged: state}})
			}

		}
		subject := evtstream.UserAggregate(botID).SubjectFor(event)
		seqs, err := m.core.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: subject, Event: event, HasOCC: true, ExpectedSeq: seq, FilterSubject: filter}})
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err = m.projection.Projector().WaitFor(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
			return nil, err
		}
		if revoke {
			return nil, nil
		}
		// Return the result of this mutation, even if another replica changed it.
		if creation != nil {
			current = &botWebhookEndpoint{Configuration: creation, CreatedAt: creation.GetCreatedAt().AsTime(), Enabled: *enabled}
		} else {
			if cfg := event.GetBotOutboundWebhookConfigured(); cfg != nil {
				current.Configuration = event
				current.Enabled = cfg.GetEnabled()
			} else if enabled != nil {
				current.Enabled = *enabled
			}
		}
		return m.metadata(ctx, current)
	}
	return nil, events.ErrConflict
}
func (m *botWebhookModel) credentials(ctx context.Context, e *evtv1.Event) (botWebhookCredentials, error) {
	var result botWebhookCredentials
	x := e.GetBotOutboundWebhookConfigured()
	if x.GetCredentials() == nil {
		return result, ErrNotFound
	}
	if err := m.core.userModel.waitForUserAuthCurrent(ctx, "outbound webhook credentials"); err != nil {
		return result, err
	}
	key, ok, err := m.core.userModel.contentKeyAtEpoch(x.GetBotUserId(), evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, x.GetCredentials().GetContentKeyEpoch())
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrNotFound
	}
	dek, err := m.core.unwrapUserDEK(ctx, key, evtv1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII)
	if err != nil {
		return result, err
	}
	plain, err := decryptUserPIIString(dek.key, e.GetId(), x.GetBotUserId(), "bot_outbound_webhook_configured", "credentials", x.GetCredentials())
	if err != nil {
		return result, err
	}
	err = json.Unmarshal([]byte(plain), &result)
	return result, err
}

// readBotOutboundWebhook checks configuration access without requiring LOG.
// Configuration commands and scoped log reads must not depend on a diagnostic
// lookup for every other endpoint of this bot.
func (c *ChattoCore) readBotOutboundWebhook(ctx context.Context, actorID, botID, webhookID string) (*botWebhookEndpoint, error) {
	if !logToken(botID) || !logToken(webhookID) {
		return nil, invalidArgument("invalid endpoint ID")
	}
	if err := c.authorizeAtStableInputs(ctx, func() error { _, err := c.requireBotManager(ctx, actorID, botID); return err }); err != nil {
		return nil, err
	}
	if err := c.botWebhooks.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	endpoint := c.botWebhooks.projection.Projection().get(botID, webhookID)
	if endpoint == nil {
		return nil, ErrNotFound
	}
	return endpoint, nil
}
