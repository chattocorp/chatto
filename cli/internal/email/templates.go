package email

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	texttemplate "text/template"
	"time"
)

//go:embed templates/layout.html templates/layout.txt
var templateFS embed.FS

// The layouts parse at package initialization, so a broken template stops the
// process at startup instead of failing a delivery.
var (
	htmlLayout = htmltemplate.Must(htmltemplate.ParseFS(templateFS, "templates/layout.html"))
	textLayout = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/layout.txt"))
)

// Accent holds the two accent shades of the HTML layout. Values must be CSS
// hex colors.
type Accent struct {
	// Strong colors the button background, the code, and links. White text on
	// it must be readable.
	Strong string
	// Tint is the light background behind a one-time code.
	Tint string
}

// DefaultAccent is the default accent of the bundled frontend (Tailwind
// sky-700 and sky-50). Servers do not have their own accent color yet.
var DefaultAccent = Accent{Strong: "#0369a1", Tint: "#f0f9ff"}

// Branding identifies the server that sends a transactional email.
type Branding struct {
	// ServerName is the effective display name of the server.
	ServerName string
	// ServerURL is the public web server URL. The footer shows its host so that
	// the recipient can tell which server sent the message.
	ServerURL string
	// Accent colors the HTML layout. The zero value selects DefaultAccent.
	Accent Accent
}

// host returns the host of ServerURL, or the server name when the URL has no
// host.
func (b Branding) host() string {
	if u, err := url.Parse(b.ServerURL); err == nil && u.Host != "" {
		return u.Host
	}
	return b.ServerName
}

// layoutData fills both layouts. A message shows either Code or ButtonURL.
type layoutData struct {
	Subject     string
	Preheader   string
	ServerName  string
	Heading     string
	Intro       string
	Code        string
	ButtonLabel string
	ButtonURL   string
	Expiry      string
	Footer      string
	Accent      Accent
}

// RegistrationCode builds the email that carries a registration code.
func RegistrationCode(b Branding, to, code string, ttl time.Duration, expiresAt time.Time) (Message, error) {
	return render(b, to, expiresAt, layoutData{
		Subject:   fmt.Sprintf("%s is your code for %s", code, b.ServerName),
		Preheader: "Enter this code to finish creating your account.",
		Heading:   fmt.Sprintf("Welcome to %s", b.ServerName),
		Intro:     "Enter this code to finish creating your account.",
		Code:      code,
		Expiry:    fmt.Sprintf("This code expires in %s.", ExpirationText(ttl)),
		Footer:    fmt.Sprintf("Someone used this address to sign up on %s. If that wasn't you, ignore this email. No account will be created.", b.host()),
	})
}

// EmailVerificationCode builds the email that verifies an address for an
// existing account.
func EmailVerificationCode(b Branding, to, code string, ttl time.Duration, expiresAt time.Time) (Message, error) {
	return render(b, to, expiresAt, layoutData{
		Subject:   fmt.Sprintf("%s is your code for %s", code, b.ServerName),
		Preheader: fmt.Sprintf("Enter this code to add this address to your %s account.", b.ServerName),
		Heading:   "Verify your email address",
		Intro:     fmt.Sprintf("Enter this code to add this address to your %s account.", b.ServerName),
		Code:      code,
		Expiry:    fmt.Sprintf("This code expires in %s.", ExpirationText(ttl)),
		Footer:    fmt.Sprintf("Someone asked to add this address to an account on %s. If that wasn't you, ignore this email.", b.host()),
	})
}

// PasswordReset builds the email that carries a password reset link.
func PasswordReset(b Branding, to, resetURL string, ttl time.Duration, expiresAt time.Time) (Message, error) {
	return render(b, to, expiresAt, layoutData{
		Subject:     fmt.Sprintf("Reset your %s password", b.ServerName),
		Preheader:   "Choose a new password for your account.",
		Heading:     "Reset your password",
		Intro:       fmt.Sprintf("We received a request to reset the password for your %s account. Choose a new one with the link below.", b.ServerName),
		ButtonLabel: "Set a new password",
		ButtonURL:   resetURL,
		Expiry:      fmt.Sprintf("This link expires in %s.", ExpirationText(ttl)),
		Footer:      fmt.Sprintf("Sent by %s. If you didn't ask for this, ignore this email. Your password stays the same.", b.host()),
	})
}

func render(b Branding, to string, expiresAt time.Time, data layoutData) (Message, error) {
	data.ServerName = b.ServerName
	data.Accent = b.Accent
	if data.Accent == (Accent{}) {
		data.Accent = DefaultAccent
	}
	var html, text bytes.Buffer
	if err := htmlLayout.Execute(&html, data); err != nil {
		return Message{}, fmt.Errorf("render HTML email: %w", err)
	}
	if err := textLayout.Execute(&text, data); err != nil {
		return Message{}, fmt.Errorf("render text email: %w", err)
	}
	return Message{
		ExpiresAt: expiresAt,
		To:        to,
		FromName:  b.ServerName,
		Subject:   data.Subject,
		Body:      text.String(),
		HTMLBody:  html.String(),
	}, nil
}

// ExpirationText formats a lifetime in whole hours, minutes, or seconds, for
// example "1 hour" or "30 minutes".
func ExpirationText(ttl time.Duration) string {
	switch {
	case ttl%time.Hour == 0:
		return pluralDuration(int(ttl/time.Hour), "hour")
	case ttl%time.Minute == 0:
		return pluralDuration(int(ttl/time.Minute), "minute")
	case ttl%time.Second == 0:
		return pluralDuration(int(ttl/time.Second), "second")
	default:
		return ttl.String()
	}
}

func pluralDuration(value int, unit string) string {
	if value == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", value, unit)
}
