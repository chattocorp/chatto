// Package lease provides a small JetStream KV-backed leader lease for singleton
// background work.
//
// This is deliberately a coordination primitive, not a consensus or fencing
// layer. Callers still need idempotent work and durable OCC-protected writes for
// correctness if leadership changes, a lease expires, or two processes race near
// a renewal boundary.
package lease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultLeaseTTL    = 45 * time.Second
	defaultRenewEvery  = 15 * time.Second
	defaultRetryEvery  = 5 * time.Second
	defaultReleaseWait = 5 * time.Second
)

var (
	// ErrHeld reports that the lease currently belongs to another owner.
	ErrHeld = errors.New("lease held by another owner")

	// ErrLost reports that this owner no longer owns the lease. The key may have
	// expired, been deleted, or been replaced by another owner.
	ErrLost = errors.New("lease lost")

	// ErrYield lets the leader voluntarily release the lease and retry later.
	// Long-running work can use this when another replica should get a chance to
	// make progress from a different network or process vantage point.
	ErrYield = errors.New("lease yielded")
)

type Logger interface {
	Debug(msg interface{}, keyvals ...interface{})
	Info(msg interface{}, keyvals ...interface{})
	Warn(msg interface{}, keyvals ...interface{})
	Error(msg interface{}, keyvals ...interface{})
}

type Options struct {
	Name string

	// OwnerID identifies one process/worker across renewals. A random owner is
	// generated when empty; tests may set it explicitly to make ownership stable.
	OwnerID string

	// Bucket is the KV bucket that stores the lease record. MEMORY_CACHE is the
	// default because leases are ephemeral and should not survive a full NATS
	// restart or backup/restore.
	Bucket string

	// TTL is the per-key expiry for the current lease record. RenewEvery must be
	// shorter than TTL so the owner has time to refresh before expiry.
	TTL        time.Duration
	RenewEvery time.Duration

	// RetryEvery controls how often non-leaders poll for ownership.
	RetryEvery time.Duration
	Logger     Logger
}

// Record is the JSON payload stored at lease.{name}. ExpiresAt is informational;
// NATS enforces expiry through the message TTL on the KV entry itself.
type Record struct {
	Name       string    `json:"name"`
	OwnerID    string    `json:"ownerId"`
	AcquiredAt time.Time `json:"acquiredAt"`
	RenewedAt  time.Time `json:"renewedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type Lease struct {
	js         jetstream.JetStream
	kv         jetstream.KeyValue
	name       string
	key        string
	ownerID    string
	bucket     string
	ttl        time.Duration
	renewEvery time.Duration
	retryEvery time.Duration
	logger     Logger
}

// New validates the lease configuration. It does not contact NATS; acquisition
// happens lazily in TryAcquire/Run.
func New(js jetstream.JetStream, kv jetstream.KeyValue, opts Options) (*Lease, error) {
	if js == nil {
		return nil, fmt.Errorf("lease JetStream handle is nil")
	}
	if kv == nil {
		return nil, fmt.Errorf("lease KV bucket is nil")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, fmt.Errorf("lease name is required")
	}
	ownerID := strings.TrimSpace(opts.OwnerID)
	if ownerID == "" {
		var err error
		ownerID, err = NewOwnerID()
		if err != nil {
			return nil, err
		}
	}
	bucket := strings.TrimSpace(opts.Bucket)
	if bucket == "" {
		bucket = "MEMORY_CACHE"
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	renewEvery := opts.RenewEvery
	if renewEvery <= 0 {
		renewEvery = defaultRenewEvery
	}
	retryEvery := opts.RetryEvery
	if retryEvery <= 0 {
		retryEvery = defaultRetryEvery
	}
	if renewEvery >= ttl {
		return nil, fmt.Errorf("lease renew interval must be shorter than lease TTL")
	}
	return &Lease{
		js:         js,
		kv:         kv,
		name:       name,
		key:        "lease." + name,
		ownerID:    ownerID,
		bucket:     bucket,
		ttl:        ttl,
		renewEvery: renewEvery,
		retryEvery: retryEvery,
		logger:     opts.Logger,
	}, nil
}

// NewOwnerID returns an opaque process identifier suitable for lease ownership.
func NewOwnerID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate lease owner id: %w", err)
	}
	return "owner-" + hex.EncodeToString(b[:]), nil
}

// Key returns the KV key used for this lease.
func (l *Lease) Key() string {
	return l.key
}

// OwnerID returns this process' owner identifier.
func (l *Lease) OwnerID() string {
	return l.ownerID
}

// TryAcquire attempts to create the lease record atomically. If this owner
// already owns the visible record, it refreshes that record and treats the lease
// as reacquired; this supports retry loops after transient renewal errors.
func (l *Lease) TryAcquire(ctx context.Context) (bool, error) {
	now := time.Now().UTC()
	data, err := l.marshalRecord(now, now)
	if err != nil {
		return false, err
	}
	if _, err := l.kv.Create(ctx, l.key, data, jetstream.KeyTTL(l.ttl)); err == nil {
		l.logDebug("lease acquired")
		return true, nil
	} else if !errors.Is(err, jetstream.ErrKeyExists) {
		return false, fmt.Errorf("create lease %s: %w", l.name, err)
	}

	entry, err := l.kv.Get(ctx, l.key)
	if err != nil {
		if isMissingKey(err) {
			return false, nil
		}
		return false, fmt.Errorf("read lease %s: %w", l.name, err)
	}
	record, err := DecodeRecord(entry.Value())
	if err != nil || record.OwnerID != l.ownerID {
		return false, nil
	}
	if err := l.renewAtRevision(ctx, entry.Revision(), record.AcquiredAt); err != nil {
		if errors.Is(err, ErrLost) {
			return false, nil
		}
		return false, err
	}
	l.logDebug("lease reacquired by existing owner")
	return true, nil
}

// Renew refreshes the lease TTL only if the current KV entry is still owned by
// this process. It returns ErrLost when the lease was deleted, expired, or taken
// over by another owner.
func (l *Lease) Renew(ctx context.Context) error {
	entry, record, err := l.currentOwnedEntry(ctx)
	if err != nil {
		return err
	}
	return l.renewAtRevision(ctx, entry.Revision(), record.AcquiredAt)
}

// Release deletes the lease only when this owner still owns the current KV
// revision. Releasing an already-lost lease is a successful no-op.
func (l *Lease) Release(ctx context.Context) error {
	entry, _, err := l.currentOwnedEntry(ctx)
	if err != nil {
		if errors.Is(err, ErrLost) {
			return nil
		}
		return err
	}
	if err := l.kv.Delete(ctx, l.key, jetstream.LastRevision(entry.Revision())); err != nil {
		if isMissingKey(err) || errors.Is(err, jetstream.ErrKeyExists) {
			return nil
		}
		return fmt.Errorf("release lease %s: %w", l.name, err)
	}
	l.logDebug("lease released")
	return nil
}

// Run acquires the lease, runs work as the leader, and renews until the work
// returns, the parent context ends, or the lease is lost. Returning ErrYield
// from work releases leadership and keeps the acquisition loop alive.
func (l *Lease) Run(ctx context.Context, work func(context.Context) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		acquired, err := l.TryAcquire(ctx)
		if err != nil {
			l.logWarn("lease acquisition failed", "error", err)
			if err := sleepContext(ctx, l.retryEvery); err != nil {
				return err
			}
			continue
		}
		if !acquired {
			if err := sleepContext(ctx, l.retryEvery); err != nil {
				return err
			}
			continue
		}
		if err := l.runAsLeader(ctx, work); err != nil {
			if errors.Is(err, ErrLost) {
				l.logWarn("lease lost")
				continue
			}
			if errors.Is(err, ErrYield) {
				l.logWarn("lease yielded")
				if err := sleepContext(ctx, l.retryEvery); err != nil {
					return err
				}
				continue
			}
			return err
		}
		return nil
	}
}

func (l *Lease) runAsLeader(ctx context.Context, work func(context.Context) error) error {
	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- work(leaderCtx)
	}()

	ticker := time.NewTicker(l.renewEvery)
	defer ticker.Stop()

	for {
		select {
		case err := <-errCh:
			l.releaseBestEffort()
			return err
		case <-ticker.C:
			if err := l.Renew(ctx); err != nil {
				cancel()
				<-errCh
				return ErrLost
			}
		case <-ctx.Done():
			cancel()
			<-errCh
			l.releaseBestEffort()
			return ctx.Err()
		}
	}
}

func (l *Lease) renewAtRevision(ctx context.Context, revision uint64, acquiredAt time.Time) error {
	now := time.Now().UTC()
	data, err := l.marshalRecord(acquiredAt, now)
	if err != nil {
		return err
	}
	// NATS KV supports per-key TTL on Create, but not on the high-level
	// revision-based Update call. Publish directly to the KV stream subject so
	// renewal remains both TTL-refreshing and conditional on the current subject
	// sequence. This intentionally relies on JetStream/KV internals; tests cover
	// the behavior because lease safety depends on it.
	_, err = l.js.Publish(
		ctx,
		"$KV."+l.bucket+"."+l.key,
		data,
		jetstream.WithExpectLastSequencePerSubject(revision),
		jetstream.WithMsgTTL(l.ttl),
	)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) || isMissingKey(err) {
			return ErrLost
		}
		return fmt.Errorf("renew lease %s: %w", l.name, err)
	}
	l.logDebug("lease renewed")
	return nil
}

func (l *Lease) currentOwnedEntry(ctx context.Context) (jetstream.KeyValueEntry, Record, error) {
	entry, err := l.kv.Get(ctx, l.key)
	if err != nil {
		if isMissingKey(err) {
			return nil, Record{}, ErrLost
		}
		return nil, Record{}, fmt.Errorf("read lease %s: %w", l.name, err)
	}
	record, err := DecodeRecord(entry.Value())
	if err != nil {
		return nil, Record{}, fmt.Errorf("decode lease %s: %w", l.name, err)
	}
	if record.OwnerID != l.ownerID {
		return nil, Record{}, ErrLost
	}
	return entry, record, nil
}

func (l *Lease) marshalRecord(acquiredAt, renewedAt time.Time) ([]byte, error) {
	record := Record{
		Name:       l.name,
		OwnerID:    l.ownerID,
		AcquiredAt: acquiredAt,
		RenewedAt:  renewedAt,
		ExpiresAt:  renewedAt.Add(l.ttl),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal lease %s: %w", l.name, err)
	}
	return data, nil
}

func DecodeRecord(data []byte) (Record, error) {
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (l *Lease) releaseBestEffort() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultReleaseWait)
	defer cancel()
	if err := l.Release(ctx); err != nil {
		l.logWarn("lease release failed", "error", err)
	}
}

func isMissingKey(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *Lease) logDebug(msg interface{}, keyvals ...interface{}) {
	if l.logger != nil {
		l.logger.Debug(msg, append([]interface{}{"lease", l.name, "owner_id", l.ownerID}, keyvals...)...)
	}
}

func (l *Lease) logWarn(msg interface{}, keyvals ...interface{}) {
	if l.logger != nil {
		l.logger.Warn(msg, append([]interface{}{"lease", l.name, "owner_id", l.ownerID}, keyvals...)...)
	}
}
