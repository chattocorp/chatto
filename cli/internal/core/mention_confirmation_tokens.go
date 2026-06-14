package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	mentionConfirmationTokenTTL     = 2 * time.Minute
	mentionConfirmationTokenIssuer  = "chatto"
	mentionConfirmationTokenPurpose = "mention_confirmation"
)

var ErrMentionConfirmationTokenInvalid = errors.New("mention confirmation token invalid")

type MentionConfirmationScope struct {
	UserID            string
	RoomID            string
	Kind              RoomKind
	Body              string
	ThreadRootEventID string
	AlsoSendToChannel bool
}

type mentionConfirmationClaims struct {
	Purpose           string `json:"purpose"`
	RoomID            string `json:"room_id"`
	RoomKind          string `json:"room_kind"`
	BodyHash          string `json:"body_hash"`
	ThreadRootEventID string `json:"thread_root_event_id,omitempty"`
	AlsoSendToChannel bool   `json:"also_send_to_channel,omitempty"`
	RecipientCount    int    `json:"recipient_count"`
	jwt.RegisteredClaims
}

func (c *ChattoCore) CreateMentionConfirmationToken(scope MentionConfirmationScope, recipientCount int) (string, error) {
	claims := mentionConfirmationClaims{
		Purpose:           mentionConfirmationTokenPurpose,
		RoomID:            scope.RoomID,
		RoomKind:          string(scope.Kind),
		BodyHash:          mentionConfirmationBodyHash(scope.Body),
		ThreadRootEventID: scope.ThreadRootEventID,
		AlsoSendToChannel: scope.AlsoSendToChannel,
		RecipientCount:    recipientCount,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    mentionConfirmationTokenIssuer,
			Subject:   scope.UserID,
			Audience:  []string{mentionConfirmationTokenPurpose},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(mentionConfirmationTokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(c.mentionConfirmationSigningKey())
	if err != nil {
		return "", fmt.Errorf("sign mention confirmation token: %w", err)
	}
	return signed, nil
}

func (c *ChattoCore) ValidateMentionConfirmationToken(tokenString string, scope MentionConfirmationScope) error {
	if tokenString == "" {
		return ErrMentionConfirmationTokenInvalid
	}

	claims := &mentionConfirmationClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrMentionConfirmationTokenInvalid
			}
			return c.mentionConfirmationSigningKey(), nil
		},
		jwt.WithAudience(mentionConfirmationTokenPurpose),
		jwt.WithIssuer(mentionConfirmationTokenIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || token == nil || !token.Valid {
		return ErrMentionConfirmationTokenInvalid
	}

	if claims.Purpose != mentionConfirmationTokenPurpose ||
		claims.Subject != scope.UserID ||
		claims.RoomID != scope.RoomID ||
		claims.RoomKind != string(scope.Kind) ||
		claims.BodyHash != mentionConfirmationBodyHash(scope.Body) ||
		claims.ThreadRootEventID != scope.ThreadRootEventID ||
		claims.AlsoSendToChannel != scope.AlsoSendToChannel {
		return ErrMentionConfirmationTokenInvalid
	}

	return nil
}

func (c *ChattoCore) mentionConfirmationSigningKey() []byte {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte("chatto.mention_confirmation.v1"))
	return mac.Sum(nil)
}

func mentionConfirmationBodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}
