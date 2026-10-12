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
	accounts      roomCommandAccounts
	readState     *ReadStateModel
	calls         *CallModel
	notifications *NotificationMaterializer
	logger        *log.Logger
}

// RoomLayoutModel owns room-group and sidebar mutations. It keeps authorization,
// layout conflict checks, atomic event batches and projection waits together.
type RoomLayoutModel struct {
	rooms     *RoomModel
	publisher *evtstream.Publisher
	policy    *AuthorizationModel
	logger    *log.Logger
}

// RoomMemberships returns the owner of membership reads and trusted mutations.
// Public room mutations use RoomCommands to enforce operation authorization.
func (c *ChattoCore) RoomMemberships() *RoomMembershipModel { return c.roomMemberships }

// RoomLayout returns the owner of group and sidebar layout operations.
// Transports use its Admin methods for request-time authorization.
func (c *ChattoCore) RoomLayout() *RoomLayoutModel { return c.roomLayout }
