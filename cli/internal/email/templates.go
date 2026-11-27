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

	"github.com/chattocorp/chatto/cli/internal/i18n"
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
	// Lang is the BCP 47 tag of the email's language.
	Lang string
	// Dir is "rtl" or "ltr". Align is the matching start side for clients
	// that ignore the dir attribute.
	Dir            string
	Align          string
	Subject        string
	Preheader      string
	ServerName     string
	Heading        string
	Intro          string
	Code           string
	ButtonLabel    string
	ButtonURL      string
	ButtonFallback string
	Expiry         string
	Footer         string
	Accent         Accent
}

// Content is what a transactional email says, without its wording or layout.
// The email queue stores Content and renders it when it sends the email, so
// stored records hold only the recipient, the secret, and its lifetime. The
// unexported method limits Content to the kinds in this package.
type Content interface {
	layout(b Branding, t i18n.Translator) layoutData
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

func (c RegistrationCode) layout(b Branding, t i18n.Translator) layoutData {
	args := i18n.Args{"code": c.Code, "server": b.ServerName, "host": b.host()}
	intro := t.T("email.registration.intro", args)
	return layoutData{
		Subject:   t.T("email.registration.subject", args),
		Preheader: intro,
		Heading:   t.T("email.registration.heading", args),
		Intro:     intro,
		Code:      c.Code,
		Expiry:    t.T("email.common.code_expiry", i18n.Args{"duration": durationText(t, c.TTL)}),
		Footer:    t.T("email.registration.footer", args),
	}
}

func (c EmailVerificationCode) layout(b Branding, t i18n.Translator) layoutData {
	args := i18n.Args{"code": c.Code, "server": b.ServerName, "host": b.host()}
	intro := t.T("email.verification.intro", args)
	return layoutData{
		Subject:   t.T("email.verification.subject", args),
		Preheader: intro,
		Heading:   t.T("email.verification.heading", args),
		Intro:     intro,
		Code:      c.Code,
		Expiry:    t.T("email.common.code_expiry", i18n.Args{"duration": durationText(t, c.TTL)}),
		Footer:    t.T("email.verification.footer", args),
	}
}

func (c PasswordReset) layout(b Branding, t i18n.Translator) layoutData {
	args := i18n.Args{"server": b.ServerName, "host": b.host()}
	return layoutData{
		Subject:        t.T("email.password_reset.subject", args),
		Preheader:      t.T("email.password_reset.preheader", args),
		Heading:        t.T("email.password_reset.heading", args),
		Intro:          t.T("email.password_reset.intro", args),
		ButtonLabel:    t.T("email.password_reset.button", args),
		ButtonURL:      passwordResetURL(b.ServerURL, c.Token),
		ButtonFallback: t.T("email.common.button_fallback", nil),
		Expiry:         t.T("email.common.link_expiry", i18n.Args{"duration": durationText(t, c.TTL)}),
		Footer:         t.T("email.password_reset.footer", args),
	}
}

// passwordResetURL returns the frontend page that accepts a password reset
// token.
func passwordResetURL(serverURL, token string) string {
	return strings.TrimRight(serverURL, "/") + "/reset-password?token=" + url.QueryEscape(token)
}

// Render builds the complete message for content, with the wording and
// layout of this version, the given branding, and the language of locale.
// An empty or unsupported locale uses the base language (en-GB).
func Render(b Branding, locale, to string, content Content) (Message, error) {
	if content == nil {
		return Message{}, errors.New("email content required")
	}
	t := i18n.Default().Translator(locale)
	data := content.layout(b, t)
	data.Lang = t.Locale()
	data.Dir = t.Dir()
	data.Align = "left"
	if data.Dir == "rtl" {
		data.Align = "right"
	}
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

// durationText formats a lifetime in whole hours, minutes, or seconds, for
// example "1 hour" or "30 minutes", in the translator's language.
func durationText(t i18n.Translator, ttl time.Duration) string {
	switch {
	case ttl%time.Hour == 0:
		return t.N("email.common.duration.hours", int(ttl/time.Hour), nil)
	case ttl%time.Minute == 0:
		return t.N("email.common.duration.minutes", int(ttl/time.Minute), nil)
	case ttl%time.Second == 0:
		return t.N("email.common.duration.seconds", int(ttl/time.Second), nil)
	default:
		return ttl.String()
	}
}
