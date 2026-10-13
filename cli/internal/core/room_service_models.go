package core

import (
	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
)

// RoomMembershipModel owns explicit and effective membership reads, membership
// writes and moderation. Its batches preserve call and notification boundaries;
// the collaborating domains retain ownership of their external effects.
type RoomMembershipModel struct {
	rooms         *RoomModel
	users         *UserModel
	publisher     *evtstream.Publisher
	policy        *AuthorizationModel
	readState     *ReadStateModel
	calls         *CallModel
	notifications *NotificationMaterializer
	logger        *log.Logger
}

// newRoomMembershipModel returns a RoomMembershipModel with all collaborators.
// Production wiring constructs it after the authorization, read-state, call
// and notification models, so no dependency is assigned later.
func newRoomMembershipModel(
	rooms *RoomModel,
	users *UserModel,
	publisher *evtstream.Publisher,
	policy *AuthorizationModel,
	readState *ReadStateModel,
	calls *CallModel,
	notifications *NotificationMaterializer,
	logger *log.Logger,
) *RoomMembershipModel {
	return &RoomMembershipModel{
		rooms: rooms, users: users, publisher: publisher, policy: policy,
		readState: readState, calls: calls, notifications: notifications, logger: logger,
	}
}

// RoomLayoutModel owns room-group and sidebar mutations. It keeps authorization,
// layout conflict checks, atomic event batches and projection waits together.
type RoomLayoutModel struct {
	rooms     *RoomModel
	publisher *evtstream.Publisher
	policy    *AuthorizationModel
	logger    *log.Logger
}

// newRoomLayoutModel returns a RoomLayoutModel with all collaborators.
func newRoomLayoutModel(rooms *RoomModel, publisher *evtstream.Publisher, policy *AuthorizationModel, logger *log.Logger) *RoomLayoutModel {
	return &RoomLayoutModel{rooms: rooms, publisher: publisher, policy: policy, logger: logger}
}

// RoomMemberships returns the owner of membership reads and trusted mutations.
// Public room mutations use RoomCommands to enforce operation authorization.
func (c *ChattoCore) RoomMemberships() *RoomMembershipModel { return c.roomMemberships }

// RoomLayout returns the owner of group and sidebar layout operations.
// Transports use its Admin methods for request-time authorization.
func (c *ChattoCore) RoomLayout() *RoomLayoutModel { return c.roomLayout }
