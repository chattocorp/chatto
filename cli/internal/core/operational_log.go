package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	logv1 "hmans.de/chatto/internal/pb/chatto/core/log/v1"
)

// logToken accepts opaque identifiers, never NATS wildcard or delimiter syntax.
func logToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func botWebhookLogFilter(botID, webhookID string) string {
	return "log.bot_webhook." + botID + "." + webhookID + ".delivery_failed.*"
}

// operationalLogSubject defines the persisted routing contract for each payload.
// New producers add a typed branch here instead of supplying arbitrary subjects.
func operationalLogSubject(entry *logv1.Entry) (string, error) {
	failure := entry.GetBotWebhookDeliveryFailed()
	if failure == nil || !logToken(entry.GetId()) || !logToken(failure.GetBotUserId()) || !logToken(failure.GetWebhookId()) {
		return "", invalidArgument("invalid operational log entry")
	}
	switch failure.GetReason() {
	case "internal_error", "expired", "invalid_request", "http_error", "transport_error":
	default:
		return "", invalidArgument("invalid webhook failure category")
	}
	return strings.TrimSuffix(botWebhookLogFilter(failure.GetBotUserId(), failure.GetWebhookId()), "*") + entry.GetId(), nil
}

// appendOperationalLog acknowledges storage, with duplicate suppression lasting
// exactly as long as this record exists. It does not schedule or retry work.
func (c *ChattoCore) appendOperationalLog(ctx context.Context, entry *logv1.Entry) error {
	subject, err := operationalLogSubject(entry)
	if err != nil {
		return err
	}
	if entry.GetRecordedAt() == nil || entry.GetRecordedAt().CheckValid() != nil {
		return invalidArgument("invalid log timestamp")
	}
	data, err := proto.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = c.js.Publish(ctx, subject, data, jetstream.WithExpectLastSequencePerSubject(0))
	if errors.Is(err, jetstream.ErrKeyExists) {
		return nil
	}
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence {
		return nil
	}
	return err
}

func (c *ChattoCore) latestOperationalLog(ctx context.Context, filter string) (*logv1.Entry, error) {
	msg, err := c.storage.logStream.GetLastMsgForSubject(ctx, filter)
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entry := &logv1.Entry{}
	if err := proto.Unmarshal(msg.Data, entry); err != nil {
		return nil, fmt.Errorf("decode LOG entry: %w", err)
	}
	return entry, nil
}

// BotWebhookFailurePage contains complete retained records in recording order.
// NextCursor is confidential and bound to the requesting viewer and endpoint.
type BotWebhookFailurePage struct {
	Entries    []*logv1.Entry
	NextCursor string
}

type operationalLogCursor struct {
	Next        uint64
	Tail        uint64
	Incarnation string
}

// ListBotWebhookFailures reads LOG directly, so replicas and expiry share the
// same source of truth. Pagination fixes a tail and skips records removed by
// retention. Each page performs at most pageSize+1 subject-filtered reads.
func (c *ChattoCore) ListBotWebhookFailures(ctx context.Context, actorID, botID, webhookID string, pageSize uint32, cursor string) (*BotWebhookFailurePage, error) {
	if !logToken(botID) || !logToken(webhookID) || pageSize > 100 || len(cursor) > 4096 {
		return nil, invalidArgument("invalid log request")
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if _, err := c.readBotOutboundWebhook(ctx, actorID, botID, webhookID); err != nil {
		return nil, err
	}
	info, err := c.storage.logStream.Info(ctx)
	if err != nil {
		return nil, err
	}
	scopeData, _ := json.Marshal([]string{actorID, botID, webhookID})
	scope := string(scopeData)
	position := operationalLogCursor{Next: info.State.FirstSeq, Tail: info.State.LastSeq, Incarnation: info.Created.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}
	if cursor != "" {
		data, err := c.OpenPublicCursor("operational-log", scope, cursor)
		if err != nil {
			return nil, invalidArgument("invalid log cursor")
		}
		var saved operationalLogCursor
		if json.Unmarshal(data, &saved) != nil || saved.Incarnation != position.Incarnation || saved.Next == 0 || saved.Next > saved.Tail {
			return nil, invalidArgument("log cursor expired or invalid")
		}
		position = saved
	}
	result := &BotWebhookFailurePage{Entries: []*logv1.Entry{}}
	filter := botWebhookLogFilter(botID, webhookID)
	for position.Next != 0 && position.Next <= position.Tail {
		msg, err := c.storage.logStream.GetMsg(ctx, position.Next, jetstream.WithGetMsgSubject(filter))
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			break
		}
		if err != nil {
			return nil, err
		}
		if msg.Sequence > position.Tail {
			break
		}
		if len(result.Entries) == int(pageSize) {
			position.Next = msg.Sequence
			data, _ := json.Marshal(position)
			result.NextCursor, err = c.SealPublicCursor("operational-log", scope, data)
			if err != nil {
				return nil, err
			}
			break
		}
		entry := &logv1.Entry{}
		if err := proto.Unmarshal(msg.Data, entry); err != nil {
			return nil, fmt.Errorf("decode LOG entry: %w", err)
		}
		result.Entries = append(result.Entries, entry)
		position.Next = msg.Sequence + 1
	}
	// Permission changes during a multi-read page must not release stale access.
	if err := c.authorizeAtStableInputs(ctx, func() error { _, err := c.requireBotManager(ctx, actorID, botID); return err }); err != nil {
		return nil, err
	}
	return result, nil
}
