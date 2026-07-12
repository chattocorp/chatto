package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestJMAPMailer_SendContext(t *testing.T) {
	var apiCalls int
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-1" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		switch r.URL.Path {
		case "/session":
			if r.Method != http.MethodGet {
				t.Errorf("session method = %s, want GET", r.Method)
			}
			writeJMAPJSON(t, w, map[string]any{
				"apiUrl": server.URL + "/api",
				"capabilities": map[string]any{
					jmapCoreCapability:       map[string]any{},
					jmapMailCapability:       map[string]any{},
					jmapSubmissionCapability: map[string]any{},
				},
				"accounts": map[string]any{
					"account-1": map[string]any{"accountCapabilities": map[string]any{
						jmapMailCapability:       map[string]any{},
						jmapSubmissionCapability: map[string]any{},
					}},
				},
				"primaryAccounts": map[string]string{jmapSubmissionCapability: "account-1"},
			})
		case "/api":
			apiCalls++
			var payload struct {
				MethodCalls [][]json.RawMessage `json:"methodCalls"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode JMAP request: %v", err)
			}
			switch apiCalls {
			case 1:
				if got := jmapCallName(t, payload.MethodCalls[0]); got != "Identity/get" {
					t.Errorf("first method = %q, want Identity/get", got)
				}
				if got := jmapCallName(t, payload.MethodCalls[1]); got != "Mailbox/get" {
					t.Errorf("second method = %q, want Mailbox/get", got)
				}
				writeJMAPJSON(t, w, map[string]any{"methodResponses": []any{
					[]any{"Identity/get", map[string]any{"list": []any{map[string]any{"id": "identity-1", "email": "sender@example.com"}}}, "identities"},
					[]any{"Mailbox/get", map[string]any{"list": []any{map[string]any{"id": "drafts-1", "role": "drafts"}}}, "mailboxes"},
				}})
			case 2:
				if got := jmapCallName(t, payload.MethodCalls[0]); got != "Email/set" {
					t.Errorf("first method = %q, want Email/set", got)
				}
				if got := jmapCallName(t, payload.MethodCalls[1]); got != "EmailSubmission/set" {
					t.Errorf("second method = %q, want EmailSubmission/set", got)
				}
				assertJMAPSubmissionRequest(t, payload.MethodCalls)
				writeJMAPJSON(t, w, map[string]any{"methodResponses": []any{
					[]any{"Email/set", map[string]any{"created": map[string]any{"email": map[string]any{"id": "email-1"}}}, "create-email"},
					[]any{"EmailSubmission/set", map[string]any{"created": map[string]any{"submission": map[string]any{"id": "submission-1"}}}, "submit-email"},
					[]any{"Email/set", map[string]any{"destroyed": []string{"email-1"}}, "submit-email"},
				}})
			default:
				t.Errorf("unexpected JMAP API request %d", apiCalls)
			}
		default:
			t.Errorf("unexpected JMAP request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	mailer := newJMAPMailer(config.JMAPConfig{
		SessionURL:  server.URL + "/session",
		AccessToken: "token-1",
		From:        "Chatto <sender@example.com>",
	}, server.Client())

	err := mailer.SendContext(context.Background(), Message{
		To:      "Recipient <recipient@example.com>",
		Subject: "Verification code",
		Body:    "123456",
	})
	if err != nil {
		t.Fatalf("SendContext() error = %v", err)
	}
	if apiCalls != 2 {
		t.Fatalf("JMAP API calls = %d, want 2", apiCalls)
	}
}

func TestJMAPMailer_SendContextRejectsUnsupportedServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJMAPJSON(t, w, map[string]any{
			"apiUrl": "https://jmap.test/api",
			"capabilities": map[string]any{
				jmapCoreCapability: map[string]any{},
				jmapMailCapability: map[string]any{},
			},
		})
	}))
	defer server.Close()

	mailer := newJMAPMailer(config.JMAPConfig{
		SessionURL:  server.URL,
		AccessToken: "token-1",
		From:        "sender@example.com",
	}, server.Client())
	err := mailer.Send(Message{To: "recipient@example.com"})
	if err == nil || !strings.Contains(err.Error(), "does not support mail submission") {
		t.Fatalf("Send() error = %v, want unsupported submission error", err)
	}
}

func TestJMAPMailer_IsEnabled(t *testing.T) {
	mailer := NewJMAPMailer(config.JMAPConfig{})
	if mailer.IsEnabled() {
		t.Fatal("IsEnabled() = true, want false for incomplete configuration")
	}
	if err := mailer.Send(Message{}); err != ErrEmailDisabled {
		t.Fatalf("Send() error = %v, want ErrEmailDisabled", err)
	}
}

func TestJMAPMailer_RejectsInsecureSessionURL(t *testing.T) {
	mailer := NewJMAPMailer(config.JMAPConfig{
		SessionURL:  "http://mail.example/.well-known/jmap",
		AccessToken: "token-1",
		From:        "sender@example.com",
	})
	err := mailer.Send(Message{To: "recipient@example.com"})
	if err == nil || !strings.Contains(err.Error(), "JMAP session URL must be an absolute HTTPS URL") {
		t.Fatalf("Send() error = %v, want insecure-session error", err)
	}
}

func TestJMAPMailer_RejectsInsecureAPIURL(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJMAPJSON(t, w, map[string]any{
			"apiUrl": "http://mail.example/api",
			"capabilities": map[string]any{
				jmapCoreCapability:       map[string]any{},
				jmapMailCapability:       map[string]any{},
				jmapSubmissionCapability: map[string]any{},
			},
		})
	}))
	defer server.Close()

	mailer := newJMAPMailer(config.JMAPConfig{
		SessionURL:  server.URL,
		AccessToken: "token-1",
		From:        "sender@example.com",
	}, server.Client())
	err := mailer.Send(Message{To: "recipient@example.com"})
	if err == nil || !strings.Contains(err.Error(), "JMAP session API URL must be an absolute HTTPS URL") {
		t.Fatalf("Send() error = %v, want insecure-API error", err)
	}
}

func TestJMAPMailer_TreatsDraftCleanupFailureAsSubmitted(t *testing.T) {
	var apiCalls int
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session":
			writeJMAPJSON(t, w, map[string]any{
				"apiUrl": server.URL + "/api",
				"capabilities": map[string]any{
					jmapCoreCapability:       map[string]any{},
					jmapMailCapability:       map[string]any{},
					jmapSubmissionCapability: map[string]any{},
				},
				"accounts": map[string]any{"account-1": map[string]any{"accountCapabilities": map[string]any{
					jmapMailCapability:       map[string]any{},
					jmapSubmissionCapability: map[string]any{},
				}}},
				"primaryAccounts": map[string]string{jmapSubmissionCapability: "account-1"},
			})
		case "/api":
			apiCalls++
			if apiCalls == 1 {
				writeJMAPJSON(t, w, map[string]any{"methodResponses": []any{
					[]any{"Identity/get", map[string]any{"list": []any{map[string]any{"id": "identity-1", "email": "sender@example.com"}}}, "identities"},
					[]any{"Mailbox/get", map[string]any{"list": []any{map[string]any{"id": "drafts-1", "role": "drafts"}}}, "mailboxes"},
				}})
				return
			}
			writeJMAPJSON(t, w, map[string]any{"methodResponses": []any{
				[]any{"Email/set", map[string]any{"created": map[string]any{"email": map[string]any{"id": "email-1"}}}, "create-email"},
				[]any{"EmailSubmission/set", map[string]any{"created": map[string]any{"submission": map[string]any{"id": "submission-1"}}}, "submit-email"},
				[]any{"Email/set", map[string]any{"notDestroyed": map[string]any{"email-1": map[string]any{"type": "forbidden"}}}, "submit-email"},
			}})
		default:
			t.Errorf("unexpected JMAP request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	mailer := newJMAPMailer(config.JMAPConfig{
		SessionURL:  server.URL + "/session",
		AccessToken: "token-1",
		From:        "sender@example.com",
	}, server.Client())
	if err := mailer.Send(Message{To: "recipient@example.com"}); err != nil {
		t.Fatalf("Send() error = %v, want accepted submission", err)
	}
}

func TestJMAPMailer_SelectIdentityUsesWildcardDomain(t *testing.T) {
	mailer := NewJMAPMailer(config.JMAPConfig{})
	identityID, err := mailer.selectIdentity([]jmapIdentity{{ID: "identity-1", Email: "*@example.com"}}, "sender@example.com")
	if err != nil {
		t.Fatalf("selectIdentity() error = %v", err)
	}
	if identityID != "identity-1" {
		t.Fatalf("selectIdentity() = %q, want identity-1", identityID)
	}
}

func TestJMAPResponse_RequiresDraftCleanup(t *testing.T) {
	response := jmapResponse{MethodResponses: [][]json.RawMessage{
		mustJMAPResponse(t, "Email/set", map[string]any{"created": map[string]any{"email": map[string]any{"id": "email-1"}}}, "create-email"),
		mustJMAPResponse(t, "EmailSubmission/set", map[string]any{"created": map[string]any{"submission": map[string]any{"id": "submission-1"}}}, "submit-email"),
		mustJMAPResponse(t, "Email/set", map[string]any{"notDestroyed": map[string]any{"email-1": map[string]any{"type": "forbidden"}}}, "submit-email"),
	}}

	if err := response.requireDestroyedEmail("submit-email", "email-1"); err == nil || !strings.Contains(err.Error(), "JMAP draft cleanup failed") {
		t.Fatalf("requireDestroyedEmail() error = %v, want cleanup failure", err)
	}
}

func mustJMAPResponse(t *testing.T, method string, arguments any, callID string) []json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal([3]any{method, arguments, callID})
	if err != nil {
		t.Fatalf("marshal JMAP response: %v", err)
	}
	var response []json.RawMessage
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("unmarshal JMAP response: %v", err)
	}
	return response
}

func writeJMAPJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode JMAP response: %v", err)
	}
}

func jmapCallName(t *testing.T, call []json.RawMessage) string {
	t.Helper()
	if len(call) != 3 {
		t.Fatalf("JMAP method call has %d fields, want 3", len(call))
	}
	var name string
	if err := json.Unmarshal(call[0], &name); err != nil {
		t.Fatalf("decode JMAP method name: %v", err)
	}
	return name
}

func assertJMAPSubmissionRequest(t *testing.T, calls [][]json.RawMessage) {
	t.Helper()
	var createEmail struct {
		AccountID string `json:"accountId"`
		Create    map[string]struct {
			MailboxIDs map[string]bool          `json:"mailboxIds"`
			Keywords   map[string]bool          `json:"keywords"`
			From       []jmapAddress            `json:"from"`
			To         []jmapAddress            `json:"to"`
			Subject    string                   `json:"subject"`
			BodyValues map[string]jmapBodyValue `json:"bodyValues"`
		} `json:"create"`
	}
	if err := json.Unmarshal(calls[0][1], &createEmail); err != nil {
		t.Fatalf("decode Email/set request: %v", err)
	}
	message := createEmail.Create["email"]
	if createEmail.AccountID != "account-1" || !message.MailboxIDs["drafts-1"] || !message.Keywords["$draft"] {
		t.Fatalf("Email/set did not create a draft in the configured account: %#v", createEmail)
	}
	if got := message.From[0].Email; got != "sender@example.com" {
		t.Errorf("from = %q, want sender@example.com", got)
	}
	if got := message.To[0].Email; got != "recipient@example.com" {
		t.Errorf("to = %q, want recipient@example.com", got)
	}
	if message.Subject != "Verification code" || message.BodyValues["text"].Value != "123456" {
		t.Errorf("unexpected email content: %#v", message)
	}

	var submit struct {
		AccountID             string                    `json:"accountId"`
		Create                map[string]jmapSubmission `json:"create"`
		OnSuccessDestroyEmail []string                  `json:"onSuccessDestroyEmail"`
	}
	if err := json.Unmarshal(calls[1][1], &submit); err != nil {
		t.Fatalf("decode EmailSubmission/set request: %v", err)
	}
	if submit.AccountID != "account-1" || submit.Create["submission"].IdentityID != "identity-1" || submit.Create["submission"].EmailID != "#email" {
		t.Errorf("unexpected EmailSubmission/set payload: %#v", submit)
	}
	if len(submit.OnSuccessDestroyEmail) != 1 || submit.OnSuccessDestroyEmail[0] != "#submission" {
		t.Errorf("onSuccessDestroyEmail = %#v, want [#submission]", submit.OnSuccessDestroyEmail)
	}
}
