package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// BotOutboundWebhook exposes safe endpoint metadata to bot managers. URLs and
// credentials are write-only because generic tools can use secret URL paths.
type BotOutboundWebhook struct {
	ID               string
	Enabled          bool
	HasAuthorization bool
	Latest           *evtv1.Event
}
type botWebhookCredentials struct {
	URL           string `json:"url"`
	Authorization string `json:"authorization"`
	SigningSecret string `json:"signing_secret"`
}

// GetBotOutboundWebhook returns endpoint metadata only to a bot manager.
func (c *ChattoCore) GetBotOutboundWebhook(ctx context.Context, actorID, botID string) (*BotOutboundWebhook, error) {
	if err := c.authorizeAtStableInputs(ctx, func() error { _, err := c.requireBotManager(ctx, actorID, botID); return err }); err != nil {
		return nil, err
	}
	if err := c.botWebhooks.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	e, _, latest := c.botWebhooks.projection.Projection().get(botID)
	if e == nil {
		return nil, nil
	}
	creds, err := c.botWebhooks.credentials(ctx, e)
	if err != nil {
		return nil, err
	}
	x := e.GetBotOutboundWebhookConfigured()
	return &BotOutboundWebhook{ID: x.GetWebhookId(), Enabled: x.GetEnabled(), HasAuthorization: creds.Authorization != "", Latest: latest}, nil
}

// ReplaceBotOutboundWebhook replaces the full endpoint configuration. Pending
// work never moves to a new URL. The returned signing secret is shown once.
func (c *ChattoCore) ReplaceBotOutboundWebhook(ctx context.Context, actorID, botID, rawURL, authorization string, enabled bool) (*BotOutboundWebhook, string, error) {
	if err := validateBotWebhookURL(rawURL, c.config.BotWebhooks.AllowPrivateNetworks); err != nil {
		return nil, "", err
	}
	if len(authorization) > 4096 || strings.ContainsAny(authorization, "\r\n\x00") {
		return nil, "", invalidArgument("invalid outbound webhook authorization header")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, "", err
	}
	creds := botWebhookCredentials{URL: rawURL, Authorization: authorization, SigningSecret: base64.RawURLEncoding.EncodeToString(secret)}
	result, err := c.botWebhooks.configure(ctx, actorID, botID, &creds, enabled)
	if err != nil {
		return nil, "", err
	}
	// Return this mutation's generation, even if another manager has already
	// replaced it. A later read must not pair another generation with our secret.
	return result, creds.SigningSecret, nil
}

// DeleteBotOutboundWebhook removes configuration and cancels outstanding work.
func (c *ChattoCore) DeleteBotOutboundWebhook(ctx context.Context, actorID, botID string) error {
	_, err := c.botWebhooks.configure(ctx, actorID, botID, nil, false)
	return err
}

func validateBotWebhookURL(raw string, private bool) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u == nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(private && u.Scheme == "http")) {
		return invalidArgument("outbound webhook requires an absolute HTTPS URL without user information or fragment")
	}
	return nil
}

func (m *botWebhookModel) configure(ctx context.Context, actorID, botID string, creds *botWebhookCredentials, enabled bool) (*BotOutboundWebhook, error) {
	if _, err := m.core.requireBotManager(ctx, actorID, botID); err != nil {
		return nil, err
	}
	// Key creation is its own durable user fact and precedes the configuration OCC boundary.
	var dek *userDEK
	var err error
	if creds != nil {
		dek, err = m.core.ensureActiveUserPIIDEK(ctx, botID)
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 5; attempt++ {
		filter := evtstream.UserAggregate(botID).AllEventsFilter()
		seq, err := m.core.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, err
		}
		if err = m.core.userModel.waitForUsers(ctx, events.SubjectPosition(filter, seq)); err != nil {
			return nil, err
		}
		if err = m.core.authorizeAtStableInputs(ctx, func() error { _, err := m.core.requireBotManager(ctx, actorID, botID); return err }); err != nil {
			return nil, err
		}
		if creds == nil {
			if err := m.projection.Projector().WaitForCurrent(ctx); err != nil {
				return nil, err
			}
			current, _, _ := m.projection.Projection().get(botID)
			if current == nil {
				return nil, nil
			}
		}
		x := &evtv1.BotOutboundWebhookConfiguredEvent{BotUserId: botID, WebhookId: NewBotIncomingWebhookID(), Enabled: enabled}
		e := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_BotOutboundWebhookConfigured{BotOutboundWebhookConfigured: x}})
		if creds != nil {
			data, err := json.Marshal(creds)
			if err != nil {
				return nil, err
			}
			x.Credentials, err = encryptUserPIIStringWithDEK(dek, e.GetId(), botID, "bot_outbound_webhook_configured", "credentials", string(data))
			if err != nil {
				return nil, err
			}
		}
		subject := evtstream.UserAggregate(botID).SubjectFor(e)
		seqs, err := m.core.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: subject, Event: e, HasOCC: true, ExpectedSeq: seq, FilterSubject: filter}})
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := m.projection.Projector().WaitFor(ctx, events.SubjectPosition(subject, seqs[0])); err != nil {
			return nil, err
		}
		if creds == nil {
			return nil, nil
		}
		return &BotOutboundWebhook{ID: x.GetWebhookId(), Enabled: enabled, HasAuthorization: creds.Authorization != ""}, nil
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
