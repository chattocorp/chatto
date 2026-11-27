package email

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed templates/layout.html templates/layout.txt
var templateFS embed.FS

// The layouts parse at package initialization, so a broken template stops the
// process at startup instead of failing a delivery.
var (
	htmlLayout = htmltemplate.Must(htmltemplate.New("layout.html").Funcs(htmltemplate.FuncMap{
		// html/template removes comments, so the layout emits Outlook
		// conditional comments through these functions. Outlook for Windows
		// ignores max-width; the fixed-width table keeps the card at 520px.
		"msoOpen":  func() htmltemplate.HTML { return msoOpen },
		"msoClose": func() htmltemplate.HTML { return msoClose },
	}).ParseFS(templateFS, "templates/layout.html"))
	textLayout = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/layout.txt"))
)

// Outlook-only wrapper around the card. Only Outlook for Windows reads
// conditional comments.
const (
	msoOpen  htmltemplate.HTML = `<!--[if mso]><table role="presentation" width="520" align="center" cellpadding="0" cellspacing="0"><tr><td><![endif]-->`
	msoClose htmltemplate.HTML = `<!--[if mso]></td></tr></table><![endif]-->`
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

// Content is what a transactional email says, without its wording or layout.
// The email queue stores Content and renders it when it sends the email, so
// stored records hold only the recipient, the secret, and its lifetime. The
// unexported method limits Content to the kinds in this package.
type Content interface {
	layout(b Branding) layoutData
}

// RegistrationCode is the email that carries a registration code.
type RegistrationCode struct {
	Code string
	// TTL is the lifetime of the code, shown in the email.
	TTL time.Duration
}

// EmailVerificationCode is the email that verifies an address for an existing
// account.
type EmailVerificationCode struct {
	Code string
	// TTL is the lifetime of the code, shown in the email.
	TTL time.Duration
}

// PasswordReset is the email that carries a password reset link. The link is
// built from Branding.ServerURL when the email is rendered.
type PasswordReset struct {
	Token string
	// TTL is the lifetime of the link, shown in the email.
	TTL time.Duration
}

func (c RegistrationCode) layout(b Branding) layoutData {
	return layoutData{
		Subject:   fmt.Sprintf("%s is your code for %s", c.Code, b.ServerName),
		Preheader: "Enter this code to finish creating your account.",
		Heading:   fmt.Sprintf("Welcome to %s", b.ServerName),
		Intro:     "Enter this code to finish creating your account.",
		Code:      c.Code,
		Expiry:    fmt.Sprintf("This code expires in %s.", ExpirationText(c.TTL)),
		Footer:    fmt.Sprintf("Someone used this address to sign up on %s. If that wasn't you, ignore this email. No account will be created.", b.host()),
	}
}

func (c EmailVerificationCode) layout(b Branding) layoutData {
	return layoutData{
		Subject:   fmt.Sprintf("%s is your code for %s", c.Code, b.ServerName),
		Preheader: fmt.Sprintf("Enter this code to add this address to your %s account.", b.ServerName),
		Heading:   "Verify your email address",
		Intro:     fmt.Sprintf("Enter this code to add this address to your %s account.", b.ServerName),
		Code:      c.Code,
		Expiry:    fmt.Sprintf("This code expires in %s.", ExpirationText(c.TTL)),
		Footer:    fmt.Sprintf("Someone asked to add this address to an account on %s. If that wasn't you, ignore this email.", b.host()),
	}
}

func (c PasswordReset) layout(b Branding) layoutData {
	return layoutData{
		Subject:     fmt.Sprintf("Reset your %s password", b.ServerName),
		Preheader:   "Choose a new password for your account.",
		Heading:     "Reset your password",
		Intro:       fmt.Sprintf("We received a request to reset the password for your %s account. Choose a new one with the link below.", b.ServerName),
		ButtonLabel: "Set a new password",
		ButtonURL:   passwordResetURL(b.ServerURL, c.Token),
		Expiry:      fmt.Sprintf("This link expires in %s.", ExpirationText(c.TTL)),
		Footer:      fmt.Sprintf("Sent by %s. If you didn't ask for this, ignore this email. Your password stays the same.", b.host()),
	}
}

// passwordResetURL returns the frontend page that accepts a password reset
// token.
func passwordResetURL(serverURL, token string) string {
	return strings.TrimRight(serverURL, "/") + "/reset-password?token=" + url.QueryEscape(token)
}

// Render builds the complete message for content, with the wording and
// layout of this version and the given branding.
func Render(b Branding, to string, content Content) (Message, error) {
	if content == nil {
		return Message{}, errors.New("email content required")
	}
	data := content.layout(b)
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
		To:       to,
		FromName: b.ServerName,
		Subject:  data.Subject,
		Body:     text.String(),
		HTMLBody: html.String(),
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
