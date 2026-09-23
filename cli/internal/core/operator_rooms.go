package core

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// ErrRoomSourceConflict means an operator source key already identifies a room
// created with different input. The key remains bound to its first room.
var ErrRoomSourceConflict = errors.New("room source key already used with different input")

// OperatorRoomCreateInput is a local, system-attributed channel-room request.
// Source and SourceID are optional together; they identify one source room.
type OperatorRoomCreateInput struct {
	Name        string
	Description string
	GroupID     string
	Source      string
	SourceID    string
}

type roomCreationSource struct {
	keyHash     string
	requestHash string
}

// CreateOperatorRoom creates a channel with normal room lifecycle events. A
// source-bound request commits its key with RoomCreated, so exact retries
// return the first creation result after a lost response, restart, or replay.
func (c *ChattoCore) CreateOperatorRoom(ctx context.Context, input OperatorRoomCreateInput) (*evtv1.Room, error) {
	if (input.Source == "") != (input.SourceID == "") {
		return nil, invalidArgument("source and source_id must be supplied together")
	}
	if input.Source != "" && (strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.SourceID) == "") {
		return nil, invalidArgument("source and source_id must contain visible text")
	}
	if len(input.Source) > 256 || len(input.SourceID) > 1024 {
		return nil, invalidArgument("source or source_id is too long")
	}
	var source *roomCreationSource
	if input.Source != "" {
		source = &roomCreationSource{
			keyHash:     operatorRoomDigest("room-source-v1", input.Source, input.SourceID),
			requestHash: operatorRoomDigest("room-create-input-v1", input.Name, input.Description, input.GroupID),
		}
		// Catch a prior commit before resolving today's default group. The OCC
		// loop checks the same claim again to cover concurrent replicas.
		if err := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); err != nil {
			return nil, err
		}
		snapshot := c.roomModel.creationClaimSnapshot(input.Name, "", source.keyHash)
		if snapshot.sourceClaim != nil {
			return c.operatorRoomClaimResult(ctx, source, snapshot.sourceClaim)
		}
	}
	if err := validateRoomNameAndDescription(input.Name, input.Description); err != nil {
		return nil, err
	}
	room, err := c.createRoom(ctx, SystemActorID, KindChannel, input.GroupID, input.Name, input.Description, source)
	if err == nil || source == nil {
		return room, err
	}
	// A room can commit while this caller receives an uncertain publish error
	// or loses a group/name race. Check the durable claim before reporting the
	// error; a later retry remains safe if this read also fails.
	if waitErr := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); waitErr == nil {
		snapshot := c.roomModel.creationClaimSnapshot(input.Name, "", source.keyHash)
		if snapshot.sourceClaim != nil {
			return c.operatorRoomClaimResult(ctx, source, snapshot.sourceClaim)
		}
	}
	return nil, err
}

func (c *ChattoCore) operatorRoomClaimResult(ctx context.Context, source *roomCreationSource, claim *roomSourceClaim) (*evtv1.Room, error) {
	if claim.requestHash != source.requestHash {
		return nil, ErrRoomSourceConflict
	}
	// The room and group placement commit in one batch but use separate
	// projections. Wait for placement before a retry returns the room ID.
	if claim.createdRoom.GetGroupId() != "" {
		if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
			return nil, err
		}
	}
	return claim.createdRoom, nil
}

func operatorRoomDigest(domain string, parts ...string) string {
	data := make([]byte, 0, 64)
	for _, value := range append([]string{domain}, parts...) {
		data = binary.AppendUvarint(data, uint64(len(value)))
		data = append(data, value...)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
