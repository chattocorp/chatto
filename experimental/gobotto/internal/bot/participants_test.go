package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type profileClient struct {
	*fakeClient
	lookup func([]string) (map[string]chatto.Profile, error)
}

func (c *profileClient) Profiles(_ context.Context, ids []string) (map[string]chatto.Profile, error) {
	return c.lookup(ids)
}

func TestParticipantProfileProjectionAndBatching(t *testing.T) {
	ids := []string{"", "person", "person"}
	for i := range 100 {
		ids = append(ids, fmt.Sprintf("user-%03d", i))
	}
	calls := 0
	client := &profileClient{fakeClient: newFake(), lookup: func(ids []string) (map[string]chatto.Profile, error) {
		calls++
		if len(ids) > 100 {
			t.Fatal("SDK batch limit exceeded")
		}
		result := map[string]chatto.Profile{}
		for _, id := range ids {
			result[id] = chatto.Profile{ID: id, Login: "handle", DisplayName: "Display Name", AvatarURL: "private-avatar-url"}
		}
		result["unrequested"] = chatto.Profile{ID: "unrequested", Login: "other"}
		return result, nil
	}}
	profiles := resolveParticipants(context.Background(), client, ids)
	encoded, _ := json.Marshal(profiles)
	if len(profiles) != 101 || calls != 2 || strings.Contains(string(encoded), "private-avatar-url") || strings.Contains(string(encoded), "unrequested") {
		t.Fatal("incorrect participant projection")
	}
	if profiles[0].Username != "handle" || profiles[0].DisplayName != "Display Name" || !profiles[0].Resolved {
		t.Fatal("verified names missing")
	}
}

func TestParticipantContextRefreshAndLookupFailure(t *testing.T) {
	lookupErr := false
	login := "first_handle"
	client := &profileClient{fakeClient: newFake(), lookup: func(ids []string) (map[string]chatto.Profile, error) {
		if lookupErr {
			return nil, errors.New("private error")
		}
		return map[string]chatto.Profile{"human": {ID: "human", Login: login, DisplayName: "Person Name"}}, nil
	}}
	c := &conversation{key: threadKey{"room", "root"}}
	c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
		var content string
		if err := json.Unmarshal(messages[len(messages)-1]["content"], &content); err != nil {
			t.Fatal(err)
		}
		var batch struct {
			Participants []participantProfile `json:"participants"`
			Authors      []triggerAuthor      `json:"triggerAuthors"`
		}
		if err := json.Unmarshal([]byte(content), &batch); err != nil {
			t.Fatal(err)
		}
		if len(batch.Participants) != 1 || batch.Participants[0].UserID != "human" || batch.Authors[0].IsOwner {
			t.Fatal("incorrect identity or authority")
		}
		profile := batch.Participants[0]
		if lookupErr {
			if profile.Resolved || profile.Username != "" || profile.DisplayName != "" {
				t.Fatal("failed lookup reused stale names")
			}
		} else if !profile.Resolved || profile.Username != login || profile.DisplayName != "Person Name" {
			t.Fatal("current profile not supplied")
		}
		return answer(), nil
	}
	for i := range 3 {
		login = fmt.Sprintf("handle_%d", i)
		lookupErr = i == 2
		if err := respond(context.Background(), client, c, []chatto.MessageEvent{input(fmt.Sprintf("m%d", i), "hello", true)}, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestContextDeltasResetAfterFailureAndTranscriptReplacement(t *testing.T) {
	c := &conversation{key: threadKey{"room", "root"}}
	client := newFake()
	for i, wantContext := range []bool{true, false, false, true, true} {
		if i == 4 {
			c.model = nil
		}
		c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
			var content string
			_ = json.Unmarshal(messages[len(messages)-1]["content"], &content)
			var input map[string]json.RawMessage
			if err := json.Unmarshal([]byte(content), &input); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"participants", "brainCatalog"} {
				if _, present := input[field]; present != wantContext {
					t.Fatalf("turn %d: %s present=%t want=%t", i, field, present, wantContext)
				}
			}
			if len(input["triggerAuthors"]) == 0 {
				t.Fatal("authority metadata omitted")
			}
			if i == 2 {
				return nil, errors.New("model failed")
			}
			return answer(), nil
		}
		err := respond(context.Background(), client, c, []chatto.MessageEvent{input(fmt.Sprintf("m%d", i), "hello", true)}, nil)
		if (err != nil) != (i == 2) {
			t.Fatal(err)
		}
	}
}
