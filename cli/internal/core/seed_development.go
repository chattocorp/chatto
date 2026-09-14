//go:build bootstrap || test_endpoints

package core

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"

	"github.com/brianvoe/gofakeit/v7"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// SeedVersion identifies the deterministic content and relationship generator.
const SeedVersion = "synthetic-v2-gofakeit-7.17.0"

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

// SeedRoom identifies a generated channel. All generated users are members.
type SeedRoom struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
		for _, user := range result.Users {
			if _, err := c.AddMember(ctx, SystemActorID, KindChannel, room.Id, user.ID); err != nil {
				return result, fmt.Errorf("seed room %d membership: %w", i+1, err)
			}
		}
	}
	roots := o.Messages - o.ThreadReplies
	for i := 0; i < o.Messages; i++ {
		room := result.Rooms[rng.IntN(len(result.Rooms))]
		author := result.Users[rng.IntN(len(result.Users))]
		threadRootID := ""
		if i >= roots {
			root := result.Messages[rng.IntN(roots)]
			room.ID, threadRootID = root.RoomID, root.ID
		}
		body := faker.Sentence()
		post, err := c.Messages().PostMessage(ctx, MessagePostInput{
			ActorID: author.ID, RoomID: room.ID, Body: body, ThreadRootEventID: threadRootID,
		})
		if err != nil {
			return result, fmt.Errorf("seed message %d: %w", i+1, err)
		}
		result.Messages = append(result.Messages, SeedMessage{ID: post.Event.Id, RoomID: room.ID,
			AuthorID: author.ID, Body: body, ThreadRootID: threadRootID})
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
