package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
)

func TestOperatorUserIdentityRecoversChangedIssuer(t *testing.T) {
	env := newOperatorCLITestEnvWithConfig(t, config.ChattoConfig{Auth: config.AuthConfig{
		DirectLogin: new(false), Providers: []config.AuthProviderConfig{{
			ID: "company", Type: "oidc", IssuerURL: "https://new.example.com",
		}},
	}})
	user, err := env.core.CreateUser(env.ctx, core.SystemActorID, "issuer-recovery", "Issuer Recovery", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.core.LinkExternalIdentity(env.ctx, "company", "oidc", "https://old.example.com", "subject-1", user.Id); err != nil {
		t.Fatal(err)
	}
	old, err := env.core.ExternalIdentitiesForUser(env.ctx, user.Id)
	if err != nil || len(old) != 1 {
		t.Fatalf("old identities = %v, %v", old, err)
	}
	token, err := env.core.CreateAuthToken(env.ctx, user.Id)
	if err != nil {
		t.Fatal(err)
	}
	listed := env.run(t, "operator", "user", "identity", "list", user.Id)
	if !strings.Contains(listed, `issuer="https://old.example.com"`) || !strings.Contains(listed, "login_available=false") {
		t.Fatalf("stale identity output = %q", listed)
	}
	linked := env.run(t, "operator", "user", "identity", "link", user.Id, "--provider", "company", "--subject", "subject-1", "--json")
	var response struct {
		Identity struct {
			Issuer         string `json:"issuer"`
			SubjectHash    string `json:"subjectHash"`
			LoginAvailable bool   `json:"loginAvailable"`
		} `json:"identity"`
	}
	if err := json.Unmarshal([]byte(linked), &response); err != nil {
		t.Fatal(err)
	}
	if response.Identity.Issuer != "https://new.example.com" || !response.Identity.LoginAvailable || response.Identity.SubjectHash == old[0].SubjectHash {
		t.Fatalf("linked identity = %+v", response.Identity)
	}
	// A repeated request after an uncertain response must not append another fact.
	env.run(t, "operator", "user", "identity", "link", user.Id, "--provider", "company", "--subject", "subject-1")
	env.run(t, "operator", "user", "identity", "unlink", user.Id, old[0].SubjectHash)
	if _, err := env.core.ValidateAuthToken(env.ctx, token); err != nil {
		t.Fatalf("existing session after recovery: %v", err)
	}
	// Use the ordinary identity login lookup to prove the original account owns
	// the new provider identity, without a password or an email claim.
	owner, err := env.core.GetUserByExternalIdentity(env.ctx, "https://new.example.com", "subject-1")
	if err != nil || owner == nil || owner.Id != user.Id {
		t.Fatalf("new identity owner = %v, %v", owner, err)
	}
	if _, err := env.execute(t, "operator", "user", "identity", "unlink", user.Id, response.Identity.SubjectHash); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("last method removal = %v", err)
	}
	if _, err := env.execute(t, "operator", "user", "identity", "unlink", user.Id, old[0].SubjectHash); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("repeated removal = %v", err)
	}
	for _, eventType := range []string{evtstream.EventUserExternalIdentityLinked, evtstream.EventUserExternalIdentityUnlinked} {
		events, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.UserAggregate(user.Id).Subject(eventType))
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if eventType == evtstream.EventUserExternalIdentityLinked {
			want = 2 // original link plus recovery; repeat adds no fact
		}
		if len(events) != want || events[len(events)-1].GetActorId() != core.SystemActorID {
			t.Fatalf("%s events count/actor = %v", eventType, events)
		}
	}
}

func TestOperatorUserIdentityRejectsInvalidRequestsAndConflicts(t *testing.T) {
	env := newOperatorCLITestEnvWithConfig(t, config.ChattoConfig{Auth: config.AuthConfig{
		Providers: []config.AuthProviderConfig{{ID: "github", Type: "github"}},
	}})
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "identity-owner", "Identity Owner", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "identity-other", "Identity Other", "")
	if err != nil {
		t.Fatal(err)
	}
	env.run(t, "operator", "user", "identity", "link", owner.Id, "--provider", "github", "--subject", "1234")
	for _, tc := range []struct {
		name string
		args []string
		code connect.Code
	}{
		{"conflict", []string{"link", other.Id, "--provider", "github", "--subject", "1234"}, connect.CodeAlreadyExists},
		{"unknown provider", []string{"link", other.Id, "--provider", "missing", "--subject", "different"}, connect.CodeNotFound},
		{"missing user", []string{"link", "missing", "--provider", "github", "--subject", "different"}, connect.CodeNotFound},
		{"missing list user", []string{"list", "missing"}, connect.CodeNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.execute(t, append([]string{"operator", "user", "identity"}, tc.args...)...)
			if connect.CodeOf(err) != tc.code {
				t.Fatalf("error = %v, want %v", err, tc.code)
			}
		})
	}
	for _, args := range [][]string{
		{"link", other.Id, "--provider", "github"},
		{"link", other.Id, "--subject", "1234"},
		{"link", other.Id, "--provider", "github", "--subject", " "},
		{"list", ""}, {"unlink", other.Id, ""},
	} {
		if _, err := env.execute(t, append([]string{"operator", "user", "identity"}, args...)...); err == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
	if identities, err := env.core.ExternalIdentitiesForUser(env.ctx, other.Id); err != nil || len(identities) != 0 {
		t.Fatalf("rejected requests changed user identities: %v, %v", identities, err)
	}
	// Subjects are opaque. Preserve whitespace and escape terminal control
	// characters in explicit operator output instead of normalizing the key.
	subject := " spaced-subject\n"
	out := env.run(t, "operator", "user", "identity", "link", owner.Id, "--provider", "github", "--subject", subject)
	if !strings.Contains(out, `issuer="github"`) || !strings.Contains(out, `subject=" spaced-subject\n"`) || strings.Count(out, "\n") != 1 {
		t.Fatalf("opaque subject output = %q", out)
	}
	found, err := env.core.GetUserByExternalIdentity(env.ctx, "github", subject)
	if err != nil || found == nil || found.Id != owner.Id {
		t.Fatalf("exact subject owner = %v, %v", found, err)
	}
	if normalized, err := env.core.GetUserByExternalIdentity(env.ctx, "github", strings.TrimSpace(subject)); err != nil || normalized != nil {
		t.Fatalf("normalized subject unexpectedly linked = %v, %v", normalized, err)
	}
}
