package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ApplicationCapability is a stable OAuth-scope-style operation identifier.
// A grant is only an upper bound: actor authority and resource context are
// evaluated separately for every operation.
type ApplicationCapability string

const (
	ApplicationCapabilityDMMessageRead ApplicationCapability = "dm.messages.read"
	ApplicationCapabilityMessageWrite  ApplicationCapability = "messages.write"
)

// ApplicationCapabilityDefinition is the server-owned disclosure metadata for
// one recognised capability.
type ApplicationCapabilityDefinition struct {
	ID          ApplicationCapability
	DisplayName string
	Description string
}

var applicationCapabilityCatalog = []ApplicationCapabilityDefinition{
	{ID: ApplicationCapabilityDMMessageRead, DisplayName: "Read direct messages", Description: "Read the complete history of direct-message conversations that a user explicitly starts with this bot."},
	{ID: ApplicationCapabilityMessageWrite, DisplayName: "Post messages", Description: "Post messages only in conversations and contexts where this bot has been explicitly invited."},
}

// ListApplicationCapabilities returns a detached, stable-order catalogue.
func ListApplicationCapabilities() []ApplicationCapabilityDefinition {
	return append([]ApplicationCapabilityDefinition(nil), applicationCapabilityCatalog...)
}

func applicationCapabilityKnown(capability ApplicationCapability) bool {
	return slices.ContainsFunc(applicationCapabilityCatalog, func(definition ApplicationCapabilityDefinition) bool {
		return definition.ID == capability
	})
}

func normalizeApplicationCapabilities(raw []string) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	capabilities := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" || !applicationCapabilityKnown(ApplicationCapability(value)) {
			return nil, fmt.Errorf("%w: unknown application capability %q", ErrInvalidArgument, value)
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("%w: duplicate application capability %q", ErrInvalidArgument, value)
		}
		seen[value] = struct{}{}
		capabilities = append(capabilities, value)
	}
	sort.Strings(capabilities)
	return capabilities, nil
}

// SetBotCapabilities replaces a bot's complete approved capability set. The
// management decision and mutation are authorization-fenced together.
func (c *ChattoCore) SetBotCapabilities(ctx context.Context, actorID, botID string, raw []string) (*corev1.User, error) {
	capabilities, err := normalizeApplicationCapabilities(raw)
	if err != nil {
		return nil, err
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_BotCapabilitiesSet{
		BotCapabilitiesSet: &corev1.BotCapabilitiesSetEvent{UserId: botID, CapabilityIds: capabilities},
	}})
	if _, err := c.appendUserEvent(ctx, botID, event, evtstream.UserAggregate(botID).AllEventsFilter(), func() error {
		return c.requireManageableBot(ctx, actorID, botID)
	}); err != nil {
		return nil, err
	}
	c.publishUserProfileUpdate(ctx, botID)
	return c.GetUser(ctx, botID)
}

// BotHasCapability reports whether the current projected bot profile contains
// a recognised approved capability. Unknown persisted identifiers fail closed.
func BotHasCapability(bot *corev1.User, capability ApplicationCapability) bool {
	return bot != nil && isBotAccount(bot) && applicationCapabilityKnown(capability) && slices.Contains(bot.GetBot().GetCapabilityIds(), string(capability))
}

// AuthorizeBotCapability applies the two account-level gates for a bot
// operation: the capability must be approved and both the bot and its human
// owner must still be active. Resource context is checked by the operation
// model after this succeeds.
func (c *ChattoCore) AuthorizeBotCapability(ctx context.Context, botID string, capability ApplicationCapability) (*corev1.User, *corev1.User, error) {
	if strings.TrimSpace(botID) == "" {
		return nil, nil, ErrNotAuthenticated
	}
	if err := c.userModel.waitForUsersCurrent(ctx, "bot capability authorization", evtstream.UserAggregate(botID).AllEventsFilter()); err != nil {
		return nil, nil, err
	}
	bot, err := c.GetUser(ctx, botID)
	if err != nil || !isBotAccount(bot) || bot.GetDeleted() || !BotHasCapability(bot, capability) {
		return nil, nil, ErrPermissionDenied
	}
	ownerID := bot.GetBot().GetOwnerId()
	if err := c.userModel.waitForUsersCurrent(ctx, "bot owner capability authorization", evtstream.UserAggregate(ownerID).AllEventsFilter()); err != nil {
		return nil, nil, err
	}
	owner, err := c.GetUser(ctx, ownerID)
	if err != nil || isBotAccount(owner) || owner.GetDeleted() {
		return nil, nil, ErrPermissionDenied
	}
	return bot, owner, nil
}
