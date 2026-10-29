package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	configv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/config/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
)

func setTestRegistrationAdmission(t *testing.T, c *core.ChattoCore, inviteOnly bool) {
	t.Helper()
	admission := evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_OPEN
	if inviteOnly {
		admission = evtv1.RegistrationAdmission_REGISTRATION_ADMISSION_INVITE_ONLY
	}
	if err := c.ConfigModel().SetServerConfig(t.Context(), "test", &configv1.ServerConfig{RegistrationAdmission: admission}); err != nil {
		t.Fatal(err)
	}
}

// acceptTestInvitation uses the browser cookie contract, including its CSRF header.
func acceptTestInvitation(t *testing.T, client *http.Client, baseURL, code, externalToken string) map[string]string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"code": code, "externalIdentityToken": externalToken})
	request, err := http.NewRequest(http.MethodPost, baseURL+"/auth/browser/registration-invitation", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Chatto-Authentication-Mode", "cookie")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]string
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result["admissionId"] == "" {
		t.Fatalf("accept invitation: status %d, %v", response.StatusCode, result)
	}
	return result
}

func TestRegistrationAdmissionIsCookieBoundAndKeepsTabSelection(t *testing.T) {
	ts, client, c, _ := setupTestHTTPServerWithMailerAuthConfig(t, config.AuthConfig{})
	setTestRegistrationAdmission(t, c, true)
	ctx := t.Context()
	admin, err := c.CreateUser(ctx, core.SystemActorID, "gate-admin", "Gate Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatal(err)
	}
	first, err := c.CreateInvitationCode(ctx, admin.Id, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.CreateInvitationCode(ctx, admin.Id, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	firstID := acceptTestInvitation(t, client, ts.URL, c.InvitationCode(first.ID), "")["admissionId"]
	secondID := acceptTestInvitation(t, client, ts.URL, c.InvitationCode(second.ID), "")["admissionId"]
	status := func(client *http.Client, id string) string {
		t.Helper()
		response, err := client.Get(ts.URL + "/auth/browser/registration-invitation?admission=" + url.QueryEscape(id))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result map[string]string
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result["admissionId"]
	}
	if status(client, firstID) != firstID || status(client, secondID) != secondID {
		t.Fatal("another tab replaced admission")
	}
	if status(http.DefaultClient, firstID) != "" || status(client, "forged-selector") != "" {
		t.Fatal("selector alone granted admission")
	}
	if _, err := c.RevokeInvitation(ctx, admin.Id, first.ID); err != nil {
		t.Fatal(err)
	}
	if status(client, firstID) != "" {
		t.Fatal("revoked selection fell back to another tab")
	}
	if status(client, secondID) != secondID {
		t.Fatal("revocation removed unrelated admission")
	}
	state, err := c.GetInvitation(ctx, admin.Id, second.ID)
	if err != nil || state.Used {
		t.Fatalf("validation consumed code: %v", err)
	}

	for _, tc := range []struct {
		contentType, origin string
		want                int
	}{
		{"application/json", "https://evil.example", http.StatusForbidden},
		{"application/x-www-form-urlencoded", "", http.StatusUnsupportedMediaType},
	} {
		request, _ := http.NewRequest(http.MethodPost, ts.URL+"/auth/browser/registration-invitation", bytes.NewBufferString(`{"code":"bad"}`))
		request.Header.Set("Content-Type", tc.contentType)
		request.Header.Set("Origin", tc.origin)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Fatalf("CSRF rejection = %d, want %d", response.StatusCode, tc.want)
		}
	}
}

func TestRegistrationAdmissionBindsSSOWithoutExtendingExpiry(t *testing.T) {
	ts, client, c, _ := setupTestHTTPServerWithMailerAuthConfig(t, config.AuthConfig{})
	setTestRegistrationAdmission(t, c, true)
	ctx := t.Context()
	admin, err := c.CreateUser(ctx, core.SystemActorID, "sso-gate-admin", "SSO Gate Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
		t.Fatal(err)
	}
	invitation, err := c.CreateInvitationCode(ctx, admin.Id, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().Add(-time.Minute)
	original, err := c.CreatePendingExternalIdentityCreateFlow(ctx, core.PendingExternalIdentityFlow{
		ProviderID: "test", ProviderType: config.AuthProviderTypeOpenIDConnect,
		Issuer: "https://id.example", Subject: "gate-subject", CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The same input remains valid after a lost response. Copies retain the
	// original identity, expiry, and invitation without consuming the code.
	for range 2 {
		bound := acceptTestInvitation(t, client, ts.URL, c.InvitationCode(invitation.ID), original)
		flow, err := c.GetPendingExternalIdentityCreateFlow(ctx, bound["externalIdentityToken"])
		if err != nil {
			t.Fatal(err)
		}
		if !flow.CreatedAt.Equal(createdAt) || flow.InvitationID != invitation.ID || flow.Subject != "gate-subject" {
			t.Fatal("SSO binding changed verified data or expiry")
		}
	}
	flow, err := c.GetPendingExternalIdentityCreateFlow(ctx, original)
	if err != nil || flow.InvitationID != "" {
		t.Fatalf("original flow changed: %v", err)
	}
	state, err := c.GetInvitation(ctx, admin.Id, invitation.ID)
	if err != nil || state.Used {
		t.Fatalf("SSO binding consumed code: %v", err)
	}
}
