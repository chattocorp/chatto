package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

var (
	ErrEventDeliveryCursorInvalid = errors.New("event delivery cursor is invalid")
	ErrEventDeliveryCursorExpired = errors.New("event delivery cursor has expired")
	ErrEventReplayTooLarge        = errors.New("event replay requires a full refresh")
)

const (
	eventDeliveryCursorVersion       = 1
	eventDeliveryCursorPrefix        = "edc1"
	eventDeliveryCursorScope         = "event-delivery-cursor"
	eventDeliveryCursorMaxAge        = 7 * 24 * time.Hour
	eventDeliveryCursorFutureSkew    = 5 * time.Minute
	maxMyEventsReplayEvents          = 1000
	defaultEventDeliveryCursorIssued = 0
)

type eventDeliveryCursorClaims struct {
	Version  int    `json:"v"`
	UserID   string `json:"u"`
	Seq      uint64 `json:"s"`
	IssuedAt int64  `json:"iat"`
}

func (c *ChattoCore) FormatEventDeliveryCursor(userID string, seq uint64, issuedAt time.Time) string {
	if seq == 0 || userID == "" {
		return ""
	}
	if issuedAt.IsZero() {
		issuedAt = time.Now()
	}
	claims := eventDeliveryCursorClaims{
		Version:  eventDeliveryCursorVersion,
		UserID:   userID,
		Seq:      seq,
		IssuedAt: issuedAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return ""
	}
	payloadEncoded := base64.RawURLEncoding.EncodeToString(payload)
	macEncoded := base64.RawURLEncoding.EncodeToString(c.eventDeliveryCursorMAC(payloadEncoded))
	return eventDeliveryCursorPrefix + "." + payloadEncoded + "." + macEncoded
}

func (c *ChattoCore) ParseEventDeliveryCursor(userID, cursor string, now time.Time) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	prefix, rest, ok := cutCursorPart(cursor)
	if !ok || prefix != eventDeliveryCursorPrefix {
		return 0, ErrEventDeliveryCursorInvalid
	}
	payloadEncoded, macEncoded, ok := cutCursorPart(rest)
	if !ok || payloadEncoded == "" || macEncoded == "" {
		return 0, ErrEventDeliveryCursorInvalid
	}
	wantMAC, err := base64.RawURLEncoding.DecodeString(macEncoded)
	if err != nil {
		return 0, ErrEventDeliveryCursorInvalid
	}
	gotMAC := c.eventDeliveryCursorMAC(payloadEncoded)
	if !hmac.Equal(gotMAC, wantMAC) {
		return 0, ErrEventDeliveryCursorInvalid
	}

	payload, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return 0, ErrEventDeliveryCursorInvalid
	}
	var claims eventDeliveryCursorClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, ErrEventDeliveryCursorInvalid
	}
	if claims.Version != eventDeliveryCursorVersion || claims.UserID != userID || claims.Seq == 0 || claims.IssuedAt == defaultEventDeliveryCursorIssued {
		return 0, ErrEventDeliveryCursorInvalid
	}
	issuedAt := time.Unix(claims.IssuedAt, 0)
	if issuedAt.After(now.Add(eventDeliveryCursorFutureSkew)) {
		return 0, ErrEventDeliveryCursorInvalid
	}
	if now.Sub(issuedAt) > eventDeliveryCursorMaxAge {
		return 0, ErrEventDeliveryCursorExpired
	}
	return claims.Seq, nil
}

func (c *ChattoCore) eventDeliveryCursorMAC(payloadEncoded string) []byte {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte(eventDeliveryCursorScope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strconv.Itoa(eventDeliveryCursorVersion)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(payloadEncoded))
	return mac.Sum(nil)
}

func cutCursorPart(s string) (head, tail string, ok bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

func EventReplayRequiresFullRefresh(err error) bool {
	return errors.Is(err, ErrEventDeliveryCursorInvalid) ||
		errors.Is(err, ErrEventDeliveryCursorExpired) ||
		errors.Is(err, ErrEventReplayTooLarge)
}

func newEventReplayTooLargeError(limit int) error {
	return fmt.Errorf("%w: replay exceeds %d events", ErrEventReplayTooLarge, limit)
}
