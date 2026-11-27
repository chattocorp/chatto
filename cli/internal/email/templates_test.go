package email

import (
	"strings"
	"testing"
	"time"
)

func TestTemplates(t *testing.T) {
	branding := Branding{ServerName: "Acme <Community>", ServerURL: "https://chat.acme.example"}
	expiresAt := time.Now().Add(time.Hour)
	resetURL := "https://chat.acme.example/reset-password?token=abc&x=1"

	tests := []struct {
		name        string
		build       func() (Message, error)
		subject     string
		textHas     []string
		htmlHas     []string
		htmlLacks   []string
		accentColor string
	}{
		{
			name: "registration code",
			build: func() (Message, error) {
				return RegistrationCode(branding, "a@example.test", "482913", 15*time.Minute, expiresAt)
			},
			subject: "482913 is your code for Acme <Community>",
			textHas: []string{"Welcome to Acme <Community>", "    482913\n", "expires in 15 minutes", "sign up on chat.acme.example"},
			htmlHas: []string{"Welcome to Acme &lt;Community&gt;", "482913", "background-color:#f0f9ff", "color:#0369a1"},
			// The server name is escaped everywhere in the HTML part.
			htmlLacks: []string{"<Community>"},
		},
		{
			name: "email verification code",
			build: func() (Message, error) {
				return EmailVerificationCode(branding, "a@example.test", "731058", time.Hour, expiresAt)
			},
			subject: "731058 is your code for Acme <Community>",
			textHas: []string{"Verify your email address", "731058", "expires in 1 hour"},
			htmlHas: []string{"Verify your email address", "731058"},
		},
		{
			name: "password reset",
			build: func() (Message, error) {
				return PasswordReset(branding, "a@example.test", resetURL, time.Hour, expiresAt)
			},
			subject: "Reset your Acme <Community> password",
			textHas: []string{"Set a new password:\n" + resetURL + "\n", "expires in 1 hour"},
			htmlHas: []string{`href="https://chat.acme.example/reset-password?token=abc&amp;x=1"`, "background-color:#0369a1", "Set a new password"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := tt.build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if msg.Subject != tt.subject {
				t.Errorf("subject = %q, want %q", msg.Subject, tt.subject)
			}
			if msg.To != "a@example.test" || msg.FromName != branding.ServerName || !msg.ExpiresAt.Equal(expiresAt) {
				t.Errorf("envelope = %q/%q/%v", msg.To, msg.FromName, msg.ExpiresAt)
			}
			for _, want := range tt.textHas {
				if !strings.Contains(msg.Body, want) {
					t.Errorf("text body lacks %q:\n%s", want, msg.Body)
				}
			}
			for _, want := range tt.htmlHas {
				if !strings.Contains(msg.HTMLBody, want) {
					t.Errorf("HTML body lacks %q", want)
				}
			}
			for _, unwanted := range tt.htmlLacks {
				if strings.Contains(msg.HTMLBody, unwanted) {
					t.Errorf("HTML body contains %q", unwanted)
				}
			}
			for _, want := range []string{"mso-hide:all", "<!--[if mso]><table", "<![endif]-->"} {
				if !strings.Contains(strings.ReplaceAll(msg.HTMLBody, ": ", ":"), want) {
					t.Errorf("HTML body lacks Outlook handling %q", want)
				}
			}
			if strings.Contains(msg.HTMLBody, "ZgotmplZ") {
				t.Error("HTML template rejected a value")
			}
		})
	}
}

func TestTemplatesUseCustomAccent(t *testing.T) {
	branding := Branding{ServerName: "Acme", Accent: Accent{Strong: "#115e59", Tint: "#f0fdfa"}}
	msg, err := RegistrationCode(branding, "a@example.test", "123456", time.Minute, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.HTMLBody, "color:#115e59") || !strings.Contains(msg.HTMLBody, "background-color:#f0fdfa") {
		t.Error("HTML body does not use the custom accent")
	}
	// Without a usable URL, the footer names the server.
	if !strings.Contains(msg.Body, "sign up on Acme.") {
		t.Errorf("text footer does not fall back to the server name:\n%s", msg.Body)
	}
}

func TestExpirationText(t *testing.T) {
	tests := map[time.Duration]string{
		time.Hour:               "1 hour",
		2 * time.Hour:           "2 hours",
		time.Minute:             "1 minute",
		30 * time.Minute:        "30 minutes",
		90 * time.Second:        "90 seconds",
		1500 * time.Millisecond: "1.5s",
	}
	for ttl, want := range tests {
		if got := ExpirationText(ttl); got != want {
			t.Errorf("ExpirationText(%v) = %q, want %q", ttl, got, want)
		}
	}
}

func TestSenderAddress(t *testing.T) {
	tests := []struct {
		configured, fallback, wantName string
	}{
		{"noreply@example.test", "Acme", "Acme"},
		{"Chatto Mail <noreply@example.test>", "Acme", "Chatto Mail"},
		{"noreply@example.test", "", ""},
	}
	for _, tt := range tests {
		from, err := senderAddress(tt.configured, tt.fallback)
		if err != nil {
			t.Fatalf("senderAddress(%q): %v", tt.configured, err)
		}
		if from.Name != tt.wantName || from.Address != "noreply@example.test" {
			t.Errorf("senderAddress(%q, %q) = %q <%s>", tt.configured, tt.fallback, from.Name, from.Address)
		}
	}
}
