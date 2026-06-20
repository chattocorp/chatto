package graph

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (r *Resolver) requireHumanBotManager(ctx context.Context) (*corev1.User, error) {
	caller, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if core.IsBotUser(caller) {
		return nil, core.ErrBotActorNotAllowed
	}
	return caller, nil
}

func (r *Resolver) requireCanManageBot(ctx context.Context, botUserID string) (*corev1.User, error) {
	caller, err := r.requireHumanBotManager(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := r.core.GetUser(ctx, botUserID)
	if err != nil {
		return nil, err
	}
	if !core.IsBotUser(bot) {
		return nil, core.ErrPermissionDenied
	}
	if bot.GetBotOwnerId() == caller.Id {
		canCreate, err := r.core.CanCreateBot(ctx, caller.Id)
		if err != nil {
			return nil, err
		}
		if canCreate {
			return caller, nil
		}
	}
	canManage, err := r.core.CanManageBots(ctx, caller.Id)
	if err != nil {
		return nil, err
	}
	if !canManage {
		return nil, core.ErrPermissionDenied
	}
	return caller, nil
}

func botTokenExpiresAt(preset model.BotTokenExpiryPreset, custom *timestamppb.Timestamp) (*time.Time, error) {
	now := time.Now().UTC()
	var expires time.Time
	switch preset {
	case model.BotTokenExpiryPresetDays30:
		expires = now.Add(30 * 24 * time.Hour)
	case model.BotTokenExpiryPresetDays90:
		expires = now.Add(90 * 24 * time.Hour)
	case model.BotTokenExpiryPresetDays365:
		expires = now.Add(365 * 24 * time.Hour)
	case model.BotTokenExpiryPresetIndefinite:
		return nil, nil
	case model.BotTokenExpiryPresetCustom:
		if custom == nil {
			return nil, core.ErrBotTokenExpired
		}
		expires = custom.AsTime().UTC()
	default:
		return nil, core.ErrBotTokenExpired
	}
	if !expires.After(now) {
		return nil, core.ErrBotTokenExpired
	}
	return &expires, nil
}

func (r *Resolver) botTokenModel(ctx context.Context, meta core.BotTokenMetadata) (*model.BotToken, error) {
	bot, err := r.core.GetUser(ctx, meta.BotUserID)
	if err != nil {
		return nil, err
	}
	var createdBy *corev1.User
	if meta.CreatedBy != "" {
		createdBy, _ = r.core.GetUser(ctx, meta.CreatedBy)
	}
	var revokedBy *corev1.User
	if meta.RevokedBy != "" {
		revokedBy, _ = r.core.GetUser(ctx, meta.RevokedBy)
	}
	return &model.BotToken{
		ID:           meta.ID,
		Name:         meta.Name,
		Bot:          bot,
		CreatedBy:    createdBy,
		CreatedAt:    timestamppb.New(meta.CreatedAt),
		ExpiresAt:    timestampFromPtr(meta.ExpiresAt),
		LastUsedAt:   timestampFromPtr(meta.LastUsedAt),
		RevokedAt:    timestampFromPtr(meta.RevokedAt),
		RevokedBy:    revokedBy,
		RevokeReason: stringPtrOrNil(meta.RevokeReason),
	}, nil
}

func timestampFromPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
