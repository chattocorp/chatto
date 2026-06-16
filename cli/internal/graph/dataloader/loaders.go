package dataloader

import (
	"context"
	"sync"

	"github.com/vikstrous/dataloadgen"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Loaders holds all dataloaders for a single request.
// Each request should get fresh loaders via NewLoaders().
type Loaders struct {
	core           *core.ChattoCore
	UserLoader     *dataloadgen.Loader[string, *corev1.User]
	ReactionLoader *dataloadgen.Loader[ReactionKey, []core.ReactionSummary]

	// messageBodyCache caches message body lookups within a single request.
	// Key format: "spaceId:messageBodyKey"
	// This prevents redundant KV lookups when Body, Attachments, and UpdatedAt
	// resolvers all need the same MessageBody.
	messageBodyCache sync.Map
	// notificationCache caches pending notifications within a request so
	// Server.viewerNotifications and every Room field share one scan.
	notificationCache sync.Map
}

// NewLoaders creates fresh loaders for a new request.
// The core dependency is used for batch fetching.
func NewLoaders(c *core.ChattoCore) *Loaders {
	return &Loaders{
		core:           c,
		UserLoader:     newUserLoader(c),
		ReactionLoader: newReactionLoader(c),
	}
}

// GetUser loads a user by ID, batching with other GetUser calls in the same request.
func (l *Loaders) GetUser(ctx context.Context, userID string) (*corev1.User, error) {
	return l.UserLoader.Load(ctx, userID)
}

// GetUsers loads multiple users by ID, batching efficiently.
func (l *Loaders) GetUsers(ctx context.Context, userIDs []string) ([]*corev1.User, error) {
	return l.UserLoader.LoadAll(ctx, userIDs)
}

// GetReactions loads reactions for a message, batching with other GetReactions calls in the same request.
// All messages in the batch are fetched with a single ListKeysFiltered call.
func (l *Loaders) GetReactions(ctx context.Context, eventID string) ([]core.ReactionSummary, error) {
	return l.ReactionLoader.Load(ctx, ReactionKey{EventID: eventID})
}

// messageBodyCacheEntry stores a cached message body result.
// We cache both the result and error to avoid retrying failed lookups.
type messageBodyCacheEntry struct {
	body *core.DecryptedMessageBody
	err  error
}

// GetMessageBody retrieves a message body, caching the result within the request.
// This prevents redundant KV lookups when Body, Attachments, and UpdatedAt
// resolvers all need the same MessageBody for a single message.
func (l *Loaders) GetMessageBody(ctx context.Context, kind core.RoomKind, messageBodyKey string) (*core.DecryptedMessageBody, error) {
	cacheKey := string(kind) + ":" + messageBodyKey

	// Check cache first
	if cached, ok := l.messageBodyCache.Load(cacheKey); ok {
		entry := cached.(messageBodyCacheEntry)
		return entry.body, entry.err
	}

	// Not cached, fetch from core
	body, err := l.core.GetFullMessageBody(ctx, kind, messageBodyKey)

	// Cache the result (even on error to avoid retrying)
	l.messageBodyCache.Store(cacheKey, messageBodyCacheEntry{body: body, err: err})

	return body, err
}

type notificationCacheEntry struct {
	notifications []*corev1.Notification
	err           error
}

// GetNotifications retrieves the user's pending notifications, caching the
// ordered list within a single GraphQL request.
func (l *Loaders) GetNotifications(ctx context.Context, userID string) ([]*corev1.Notification, error) {
	if cached, ok := l.notificationCache.Load(userID); ok {
		entry := cached.(notificationCacheEntry)
		return entry.notifications, entry.err
	}

	notifications, err := l.core.GetNotifications(ctx, userID)
	l.notificationCache.Store(userID, notificationCacheEntry{notifications: notifications, err: err})
	return notifications, err
}
