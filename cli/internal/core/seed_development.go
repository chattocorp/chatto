//go:build bootstrap || test_endpoints

package core

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/brianvoe/gofakeit/v7"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// SeedVersion identifies the deterministic content and relationship generator.
const SeedVersion = "synthetic-v3-gofakeit-7.17.0"

// SeedOptions selects an additive synthetic dataset. Messages is the total
// number of posts, including ThreadReplies. Seed controls content and relations,
// not generated IDs or timestamps. Existing names receive numeric suffixes.
type SeedOptions struct {
	Seed          int64 `json:"seed"`
	Users         int   `json:"users"`
	Rooms         int   `json:"rooms"`
	Messages      int   `json:"messages"`
	ThreadReplies int   `json:"threadReplies"`
}

// SeedUser identifies a passwordless synthetic account in generation order.
type SeedUser struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
}

// SeedRoom identifies a generated channel and its current generated members.
type SeedRoom struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// MemberIDs lists generated users still in the room, in join order.
	MemberIDs []string `json:"memberIds"`
}

// SeedMessage identifies a committed post. ThreadRootID is empty for root posts.
type SeedMessage struct {
	ID           string `json:"id"`
	RoomID       string `json:"roomId"`
	AuthorID     string `json:"authorId"`
	Body         string `json:"body"`
	ThreadRootID string `json:"threadRootId,omitempty"`
}

// SeedResult lists resources whose commands completed successfully. On failure
// it is partial: the failing command may also have committed before its read
// barrier failed. Seeding does not roll back earlier commands.
type SeedResult struct {
	Version  string        `json:"version"`
	Seed     int64         `json:"seed"`
	Users    []SeedUser    `json:"users"`
	Rooms    []SeedRoom    `json:"rooms"`
	Messages []SeedMessage `json:"messages"`
}

var seedNameSeparators = regexp.MustCompile(`[^a-z0-9]+`)

// Validate rejects the complete request before any durable writes occur.
func (o SeedOptions) Validate() error {
	if o.Users < 1 || o.Users > 5000 {
		return invalidArgument("users must be between 1 and 5000")
	}
	if o.Rooms < 1 || o.Rooms > 100 {
		return invalidArgument("rooms must be between 1 and 100")
	}
	if o.Users*o.Rooms > 25000 {
		return invalidArgument("users multiplied by rooms must not exceed 25000 memberships")
	}
	if o.Messages < 0 || o.Messages > 100000 {
		return invalidArgument("messages must be between 0 and 100000")
	}
	if o.ThreadReplies < 0 || o.ThreadReplies > o.Messages {
		return invalidArgument("thread replies must be between zero and messages")
	}
	if o.ThreadReplies > 0 && o.Messages == o.ThreadReplies {
		return invalidArgument("thread replies require at least one root message")
	}
	return nil
}

// SeedData creates synthetic accounts, channels, and messages through normal
// domain services. It is available only in development and test builds. All
// serving projections are current on success. Existing data is never cleared.
// Logins and room names are made unique through their normal OCC-protected
// creation operations. Failed runs are not resumable; another call adds another
// dataset. Only the returned manifest associates resources with a seeding run.
func (c *ChattoCore) SeedData(ctx context.Context, o SeedOptions) (*SeedResult, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	result := &SeedResult{Version: SeedVersion, Seed: o.Seed,
		Users: []SeedUser{}, Rooms: []SeedRoom{}, Messages: []SeedMessage{}}
	source := rand.NewPCG(uint64(o.Seed), 0x63686174746f)
	rng := rand.New(source)
	// New(0) in gofakeit chooses a crypto-random seed. An explicit source keeps
	// zero reproducible too. Each invocation owns its source; no global RNG state.
	faker := gofakeit.NewFaker(source, false)
	for i := 0; i < o.Users; i++ {
		displayName := faker.Name()
		user, err := c.createSeedUser(ctx, seedName(faker.Username()), displayName)
		if err != nil {
			return result, fmt.Errorf("seed user %d: %w", i+1, err)
		}
		result.Users = append(result.Users, SeedUser{ID: user.Id, Login: user.Login, DisplayName: user.DisplayName})
	}
	for i := 0; i < o.Rooms; i++ {
		room, err := c.createSeedRoom(ctx, seedName(faker.Hobby()), faker.Sentence())
		if err != nil {
			return result, fmt.Errorf("seed room %d: %w", i+1, err)
		}
		result.Rooms = append(result.Rooms, SeedRoom{ID: room.Id, Name: room.Name})
	}
	// Keep one member in each room so a room always has a valid author. Other
	// memberships have independent join/leave positions in the message sequence.
	type membershipChange struct {
		at, room, user int
		leave          bool
	}
	changes := []membershipChange{}
	roomIndexes := map[string]int{}
	for r, room := range result.Rooms {
		roomIndexes[room.ID] = r
		anchor := r % o.Users
		changes = append(changes, membershipChange{room: r, user: anchor})
		popularity := 0.25 + rng.Float64()*0.5
		for u := range result.Users {
			if u == anchor || rng.Float64() >= popularity {
				continue
			}
			joinAt := rng.IntN(max(1, o.Messages))
			changes = append(changes, membershipChange{at: joinAt, room: r, user: u})
			if rng.IntN(3) == 0 {
				leaveAt := joinAt + 1 + rng.IntN(max(1, o.Messages-joinAt))
				changes = append(changes, membershipChange{at: leaveAt, room: r, user: u, leave: true})
			}
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].at < changes[j].at })
	nextChange := 0
	applyMemberships := func(at int) error {
		for nextChange < len(changes) && changes[nextChange].at <= at {
			change := changes[nextChange]
			room := &result.Rooms[change.room]
			user := result.Users[change.user]
			if change.leave {
				if err := c.LeaveRoom(ctx, user.ID, KindChannel, user.ID, room.ID); err != nil {
					return err
				}
				room.MemberIDs = slices.DeleteFunc(room.MemberIDs, func(id string) bool { return id == user.ID })
			} else {
				if _, err := c.JoinRoom(ctx, user.ID, KindChannel, user.ID, room.ID); err != nil {
					return err
				}
				room.MemberIDs = append(room.MemberIDs, user.ID)
			}
			nextChange++
		}
		return nil
	}

	roots := o.Messages - o.ThreadReplies
	for i := 0; i < o.Messages; i++ {
		if err := applyMemberships(i); err != nil {
			return result, fmt.Errorf("seed membership before message %d: %w", i+1, err)
		}
		room := result.Rooms[rng.IntN(len(result.Rooms))]
		threadRootID := ""
		if i >= roots {
			root := result.Messages[rng.IntN(roots)]
			room, threadRootID = result.Rooms[roomIndexes[root.RoomID]], root.ID
		}
		authorID := room.MemberIDs[rng.IntN(len(room.MemberIDs))]
		body := faker.Sentence()
		post, err := c.Messages().PostMessage(ctx, MessagePostInput{
			ActorID: authorID, RoomID: room.ID, Body: body, ThreadRootEventID: threadRootID,
		})
		if err != nil {
			return result, fmt.Errorf("seed message %d: %w", i+1, err)
		}
		result.Messages = append(result.Messages, SeedMessage{ID: post.Event.Id, RoomID: room.ID,
			AuthorID: authorID, Body: body, ThreadRootID: threadRootID})
	}
	// A zero-message dataset still applies its joins and optional departures.
	if err := applyMemberships(max(1, o.Messages)); err != nil {
		return result, fmt.Errorf("seed final memberships: %w", err)
	}
	if err := c.WaitForProjectionsCurrent(ctx); err != nil {
		return result, fmt.Errorf("wait for seed projections: %w", err)
	}
	return result, nil
}

// seedName adapts library-generated text to Chatto's login/channel alphabet.
// Leave room for numeric collision suffixes within the 32-character login cap.
func seedName(value string) string {
	name := strings.Trim(seedNameSeparators.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if len(name) > 24 {
		name = strings.TrimRight(name[:24], "-")
	}
	if len(name) < 2 {
		return "chat"
	}
	return name
}

func seedNameCandidate(base string, attempt int) string {
	if attempt == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, attempt+1)
}

func (c *ChattoCore) createSeedUser(ctx context.Context, base, displayName string) (*evtv1.User, error) {
	for attempt := 0; attempt < 10000; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		user, err := c.CreateUser(ctx, SystemActorID, seedNameCandidate(base, attempt), displayName, "")
		if !errors.Is(err, ErrLoginAlreadyTaken) && !errors.Is(err, ErrUsernameBlocked) {
			return user, err
		}
	}
	return nil, fmt.Errorf("could not allocate a unique permitted synthetic login")
}

func (c *ChattoCore) createSeedRoom(ctx context.Context, base, description string) (*evtv1.Room, error) {
	for attempt := 0; attempt < 10000; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", seedNameCandidate(base, attempt), description)
		if !errors.Is(err, ErrRoomNameExists) {
			return room, err
		}
	}
	return nil, fmt.Errorf("could not allocate a unique synthetic channel name")
}
