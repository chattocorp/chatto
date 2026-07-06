package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	emailOTPKeyPrefix       = "email_otp."
	emailOTPChallengeSuffix = "challenge"
	emailOTPWriteMaxRetries = 16
)

var (
	errEmailOTPNotFound     = errors.New("email otp not found")
	errEmailOTPExpired      = errors.New("email otp expired")
	errEmailOTPInvalid      = errors.New("invalid email otp")
	errEmailOTPExhausted    = errors.New("email otp exhausted")
	errEmailOTPTooManyCodes = errors.New("too many active email otp codes")
)

type emailOTPChallenge struct {
	Scope       string    `json:"scope"`
	Subject     string    `json:"subject"`
	Attempts    int       `json:"attempts"`
	IssuedCount int       `json:"issued_count"`
	Exhausted   bool      `json:"exhausted"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type emailOTPEntry struct {
	key      string
	value    []byte
	revision uint64
}

func (c *ChattoCore) emailOTPMaxAttempts() int {
	return c.config.EmailOTP.MaxWrongAttemptsOrDefault()
}

func (c *ChattoCore) emailOTPMaxDeliveredCodes() int {
	return c.config.EmailOTP.MaxDeliveredCodesOrDefault()
}

func (c *ChattoCore) emailOTPThrottlingEnabled() bool {
	return c.config.EmailOTP.ThrottlingEnabledOrDefault()
}

func (c *ChattoCore) emailOTPSubjectHash(scope, subject string) string {
	return c.runtimeTokenHash("email_otp_subject."+scope, strings.TrimSpace(subject))
}

func (c *ChattoCore) emailOTPPrefix(scope, subject string) string {
	return emailOTPKeyPrefix + c.emailOTPSubjectHash(scope, subject) + "."
}

func (c *ChattoCore) emailOTPChallengeKey(scope, subject string) string {
	return c.emailOTPPrefix(scope, subject) + emailOTPChallengeSuffix
}

func (c *ChattoCore) emailOTPCodeKey(scope, subject, code string) string {
	codeHash := c.runtimeTokenHash("email_otp_code."+scope, strings.TrimSpace(subject)+"\x00"+strings.TrimSpace(code))
	return c.emailOTPPrefix(scope, subject) + codeHash
}

func (c *ChattoCore) createEmailOTP(ctx context.Context, scope, subject string, ttl time.Duration, marshalValue func(time.Time) ([]byte, error), recordIssued func(time.Time) error) (string, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", fmt.Errorf("email otp subject is required")
	}

	if c.emailOTPThrottlingEnabled() {
		activeCodes, err := c.countEmailOTPActiveCodes(ctx, scope, subject)
		if err != nil {
			return "", err
		}
		if activeCodes >= c.emailOTPMaxDeliveredCodes() {
			return "", errEmailOTPTooManyCodes
		}
	}

	createdAt := time.Now()
	challengeKey, challengeRevision, challengeCreated, err := c.reserveEmailOTPIssuance(ctx, scope, subject, createdAt, ttl)
	if err != nil {
		return "", err
	}

	var code string
	var codeKey string
	var codeRevision uint64
	for i := 0; i < 8; i++ {
		code, err = NewVerificationCode()
		if err != nil {
			_ = c.rollbackEmailOTPIssuance(ctx, challengeKey, challengeRevision, challengeCreated, ttl)
			return "", err
		}

		data, err := marshalValue(createdAt)
		if err != nil {
			_ = c.rollbackEmailOTPIssuance(ctx, challengeKey, challengeRevision, challengeCreated, ttl)
			return "", err
		}

		codeKey = c.emailOTPCodeKey(scope, subject, code)
		codeRevision, err = c.storage.runtimeStateKV.Create(ctx, codeKey, data, jetstream.KeyTTL(ttl))
		if errors.Is(err, jetstream.ErrKeyExists) {
			continue
		}
		if err != nil {
			_ = c.rollbackEmailOTPIssuance(ctx, challengeKey, challengeRevision, challengeCreated, ttl)
			return "", fmt.Errorf("failed to store email otp code: %w", err)
		}
		break
	}
	if codeRevision == 0 {
		_ = c.rollbackEmailOTPIssuance(ctx, challengeKey, challengeRevision, challengeCreated, ttl)
		return "", fmt.Errorf("failed to generate unique email otp code")
	}

	if err := recordIssued(createdAt); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, codeKey, jetstream.LastRevision(codeRevision))
		_ = c.rollbackEmailOTPIssuance(ctx, challengeKey, challengeRevision, challengeCreated, ttl)
		return "", err
	}

	return code, nil
}

func (c *ChattoCore) getEmailOTPCode(ctx context.Context, scope, subject, code string, ttl time.Duration) (*emailOTPEntry, error) {
	subject = strings.TrimSpace(subject)
	code = strings.TrimSpace(code)
	if subject == "" || !verificationCodePattern.MatchString(code) {
		return nil, errEmailOTPInvalid
	}

	challenge, _, err := c.getEmailOTPChallenge(ctx, scope, subject)
	if err != nil {
		return nil, err
	}
	if c.emailOTPThrottlingEnabled() && (challenge.Exhausted || challenge.Attempts >= c.emailOTPMaxAttempts()) {
		return nil, errEmailOTPExhausted
	}

	key := c.emailOTPCodeKey(scope, subject, code)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if isRuntimeStateKeyAbsent(err) {
			return nil, c.recordEmailOTPFailure(ctx, scope, subject, ttl)
		}
		return nil, fmt.Errorf("failed to get email otp code: %w", err)
	}

	return &emailOTPEntry{key: key, value: entry.Value(), revision: entry.Revision()}, nil
}

func (c *ChattoCore) consumeEmailOTPCode(ctx context.Context, scope, subject string, entry *emailOTPEntry) error {
	if entry == nil {
		return errEmailOTPNotFound
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, entry.key, jetstream.LastRevision(entry.revision)); err != nil {
		if isRuntimeStateKeyAbsent(err) || isRuntimeStateRevisionConflict(err) {
			return errEmailOTPNotFound
		}
		return fmt.Errorf("failed to consume email otp code: %w", err)
	}
	if err := c.deleteEmailOTPKeys(ctx, scope, subject, ""); err != nil {
		return err
	}
	return nil
}

func (c *ChattoCore) reserveEmailOTPIssuance(ctx context.Context, scope, subject string, now time.Time, ttl time.Duration) (string, uint64, bool, error) {
	key := c.emailOTPChallengeKey(scope, subject)
	for i := 0; i < emailOTPWriteMaxRetries; i++ {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if !isRuntimeStateKeyAbsent(err) {
				return "", 0, false, fmt.Errorf("failed to get email otp challenge: %w", err)
			}
			challenge := emailOTPChallenge{
				Scope:       scope,
				Subject:     subject,
				IssuedCount: 1,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			data, err := json.Marshal(challenge)
			if err != nil {
				return "", 0, false, fmt.Errorf("failed to marshal email otp challenge: %w", err)
			}
			revision, err := c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(ttl))
			if err == nil {
				return key, revision, true, nil
			}
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue
			}
			return "", 0, false, fmt.Errorf("failed to create email otp challenge: %w", err)
		}

		var challenge emailOTPChallenge
		if err := json.Unmarshal(entry.Value(), &challenge); err != nil {
			return "", 0, false, fmt.Errorf("failed to unmarshal email otp challenge: %w", err)
		}
		if c.emailOTPThrottlingEnabled() && (challenge.Exhausted || challenge.Attempts >= c.emailOTPMaxAttempts()) {
			return "", 0, false, errEmailOTPExhausted
		}
		if c.emailOTPThrottlingEnabled() && challenge.IssuedCount >= c.emailOTPMaxDeliveredCodes() {
			return "", 0, false, errEmailOTPTooManyCodes
		}
		challenge.IssuedCount++
		challenge.UpdatedAt = now
		data, err := json.Marshal(challenge)
		if err != nil {
			return "", 0, false, fmt.Errorf("failed to marshal email otp challenge: %w", err)
		}
		revision, err := c.updateRuntimeStateTokenTTL(ctx, key, data, entry.Revision(), ttl)
		if err == nil {
			return key, revision, false, nil
		}
		if isRuntimeStateRevisionConflict(err) {
			continue
		}
		return "", 0, false, fmt.Errorf("failed to update email otp challenge: %w", err)
	}
	return "", 0, false, fmt.Errorf("email otp challenge update conflict after %d retries", emailOTPWriteMaxRetries)
}

func (c *ChattoCore) rollbackEmailOTPIssuance(ctx context.Context, key string, revision uint64, created bool, ttl time.Duration) error {
	if key == "" || revision == 0 {
		return nil
	}
	if created {
		if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(revision)); err != nil && !isRuntimeStateKeyAbsent(err) && !isRuntimeStateRevisionConflict(err) {
			return err
		}
		return nil
	}
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if isRuntimeStateKeyAbsent(err) {
			return nil
		}
		return err
	}
	var challenge emailOTPChallenge
	if err := json.Unmarshal(entry.Value(), &challenge); err != nil {
		return err
	}
	if challenge.IssuedCount > 0 {
		challenge.IssuedCount--
	}
	challenge.UpdatedAt = time.Now()
	data, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	if _, err := c.updateRuntimeStateTokenTTL(ctx, key, data, entry.Revision(), ttl); err != nil && !isRuntimeStateRevisionConflict(err) {
		return err
	}
	return nil
}

func (c *ChattoCore) cancelEmailOTP(ctx context.Context, scope, subject, code string, ttl time.Duration) error {
	subject = strings.TrimSpace(subject)
	code = strings.TrimSpace(code)
	if subject == "" || !verificationCodePattern.MatchString(code) {
		return nil
	}

	codeKey := c.emailOTPCodeKey(scope, subject, code)
	if err := c.storage.runtimeStateKV.Delete(ctx, codeKey); err != nil {
		if isRuntimeStateKeyAbsent(err) {
			return nil
		}
		return fmt.Errorf("failed to delete email otp code: %w", err)
	}
	return c.decrementEmailOTPIssuance(ctx, scope, subject, ttl)
}

func (c *ChattoCore) decrementEmailOTPIssuance(ctx context.Context, scope, subject string, ttl time.Duration) error {
	key := c.emailOTPChallengeKey(scope, subject)
	for i := 0; i < emailOTPWriteMaxRetries; i++ {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if isRuntimeStateKeyAbsent(err) {
				return nil
			}
			return fmt.Errorf("failed to get email otp challenge: %w", err)
		}

		var challenge emailOTPChallenge
		if err := json.Unmarshal(entry.Value(), &challenge); err != nil {
			return fmt.Errorf("failed to unmarshal email otp challenge: %w", err)
		}
		if challenge.IssuedCount > 0 {
			challenge.IssuedCount--
		}
		challenge.UpdatedAt = time.Now()

		if challenge.IssuedCount == 0 && challenge.Attempts == 0 && !challenge.Exhausted {
			if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil {
				if isRuntimeStateKeyAbsent(err) {
					return nil
				}
				if isRuntimeStateRevisionConflict(err) {
					continue
				}
				return fmt.Errorf("failed to delete email otp challenge: %w", err)
			}
			return nil
		}

		data, err := json.Marshal(challenge)
		if err != nil {
			return fmt.Errorf("failed to marshal email otp challenge: %w", err)
		}
		if _, err := c.updateRuntimeStateTokenTTL(ctx, key, data, entry.Revision(), ttl); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return fmt.Errorf("failed to update email otp challenge: %w", err)
		}
		return nil
	}
	return fmt.Errorf("email otp challenge decrement conflict after %d retries", emailOTPWriteMaxRetries)
}

func (c *ChattoCore) getEmailOTPChallenge(ctx context.Context, scope, subject string) (*emailOTPChallenge, uint64, error) {
	key := c.emailOTPChallengeKey(scope, subject)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if isRuntimeStateKeyAbsent(err) {
			return nil, 0, errEmailOTPNotFound
		}
		return nil, 0, fmt.Errorf("failed to get email otp challenge: %w", err)
	}
	var challenge emailOTPChallenge
	if err := json.Unmarshal(entry.Value(), &challenge); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal email otp challenge: %w", err)
	}
	return &challenge, entry.Revision(), nil
}

func (c *ChattoCore) recordEmailOTPFailure(ctx context.Context, scope, subject string, ttl time.Duration) error {
	if !c.emailOTPThrottlingEnabled() {
		if _, _, err := c.getEmailOTPChallenge(ctx, scope, subject); err != nil {
			if errors.Is(err, errEmailOTPNotFound) {
				return errEmailOTPNotFound
			}
			return err
		}
		return errEmailOTPInvalid
	}

	key := c.emailOTPChallengeKey(scope, subject)
	for i := 0; i < emailOTPWriteMaxRetries; i++ {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if isRuntimeStateKeyAbsent(err) {
				return errEmailOTPNotFound
			}
			return fmt.Errorf("failed to get email otp challenge: %w", err)
		}

		var challenge emailOTPChallenge
		if err := json.Unmarshal(entry.Value(), &challenge); err != nil {
			return fmt.Errorf("failed to unmarshal email otp challenge: %w", err)
		}
		if challenge.Exhausted || challenge.Attempts >= c.emailOTPMaxAttempts() {
			return errEmailOTPExhausted
		}

		challenge.Attempts++
		challenge.UpdatedAt = time.Now()
		if challenge.Attempts >= c.emailOTPMaxAttempts() {
			challenge.Exhausted = true
		}

		data, err := json.Marshal(challenge)
		if err != nil {
			return fmt.Errorf("failed to marshal email otp challenge: %w", err)
		}
		if _, err := c.updateRuntimeStateTokenTTL(ctx, key, data, entry.Revision(), ttl); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return fmt.Errorf("failed to update email otp challenge: %w", err)
		}
		if challenge.Exhausted {
			if err := c.deleteEmailOTPKeys(ctx, scope, subject, key); err != nil {
				return err
			}
			return errEmailOTPExhausted
		}
		return errEmailOTPInvalid
	}
	return fmt.Errorf("email otp attempt update conflict after %d retries", emailOTPWriteMaxRetries)
}

func (c *ChattoCore) countEmailOTPActiveCodes(ctx context.Context, scope, subject string) (int, error) {
	prefix := c.emailOTPPrefix(scope, subject)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix+"*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list email otp keys: %w", err)
	}
	challengeKey := c.emailOTPChallengeKey(scope, subject)
	count := 0
	for key := range lister.Keys() {
		if key == challengeKey {
			continue
		}
		count++
	}
	return count, nil
}

func (c *ChattoCore) deleteEmailOTPKeys(ctx context.Context, scope, subject, keepKey string) error {
	prefix := c.emailOTPPrefix(scope, subject)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix+"*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("failed to list email otp keys: %w", err)
	}
	for key := range lister.Keys() {
		if key == keepKey {
			continue
		}
		if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil && !isRuntimeStateKeyAbsent(err) {
			return fmt.Errorf("failed to delete email otp key: %w", err)
		}
	}
	return nil
}
