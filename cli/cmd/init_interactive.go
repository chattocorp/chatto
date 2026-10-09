package cmd

import (
	"errors"
	"fmt"
	"image/color"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsauth"
)

// initOptions contains the choices shared by scripted and interactive setup.
// SMTP credentials stay in memory until the private config file is written.
type initOptions struct {
	ConfigPath         string
	Port               int
	PublicURL          string
	DirectRegistration bool
	DirectLogin        bool
	WithSSO            bool
	SSO                config.AuthProviderConfig
	SSOAutoProvision   bool
	WithLiveKit        bool
	WithSearch         bool
	SMTP               config.SMTPConfig
	EmbeddedNATS       bool
	NATSClient         config.NATSClientConfig
}

func defaultInitOptions() initOptions {
	return initOptions{
		ConfigPath: "chatto.toml", Port: 4000, PublicURL: "http://localhost:4000",
		DirectRegistration: true,
		DirectLogin:        true,
		SSO:                config.AuthProviderConfig{ID: "sso", Type: config.AuthProviderTypeOpenIDConnect, Label: "Single sign-on"},
		EmbeddedNATS:       true,
		NATSClient:         config.NATSClientConfig{URL: "nats://localhost:4222", AuthMethod: natsauth.AuthToken},
		SMTP:               config.SMTPConfig{Port: 587, TLS: config.SMTPTLSMandatory},
	}
}

// promptInitOptions requires a terminal instead of silently consuming piped input.
// No files or external connections are created while the form is running.
func promptInitOptions(cmd *cobra.Command, opts *initOptions) (bool, error) {
	if err := validateInitPath(opts.ConfigPath); err != nil {
		return false, fmt.Errorf("config %q: %w", opts.ConfigPath, err)
	}
	input, inputOK := cmd.InOrStdin().(*os.File)
	output, outputOK := cmd.OutOrStdout().(*os.File)
	if !inputOK || !outputOK || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return false, errors.New("interactive setup requires a terminal; run chatto init without --interactive for scripted setup")
	}
	port, smtpPort := strconv.Itoa(opts.Port), strconv.Itoa(opts.SMTP.Port)
	// A blank URL follows the selected local port. An explicit URL remains unchanged.
	if opts.PublicURL == fmt.Sprintf("http://localhost:%d", opts.Port) {
		opts.PublicURL = ""
	}
	confirmed := false
	wizard := newInitWizard(opts, &port, &smtpPort, &confirmed)
	if _, err := tea.NewProgram(wizard, tea.WithInput(input), tea.WithOutput(output), tea.WithContext(cmd.Context())).Run(); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return false, nil
		}
		return false, fmt.Errorf("interactive setup: %w", err)
	}
	if wizard.cancelled {
		return false, nil
	}
	if !confirmed {
		return false, nil
	}
	opts.Port, _ = strconv.Atoi(port) // The form validates both port fields.
	opts.SMTP.Port, _ = strconv.Atoi(smtpPort)
	opts.PublicURL = initPublicURL(opts.PublicURL, port)
	return true, nil
}

// newInitForm keeps conditional questions and the final review in one navigable form.
func newInitForm(opts *initOptions, port, smtpPort *string, confirmed *bool, backward *bool) *huh.Form {
	keys := initKeyMap()
	// Huh validates fields on blur, including when leaving a group backwards.
	// Let users return to earlier questions without completing the current one.
	// Forward navigation still validates every field before reaching the review.
	validate := func(check func(string) error) func(string) error {
		return func(value string) error {
			if *backward {
				return nil
			}
			return check(value)
		}
	}
	return huh.NewForm(
		newInitPage(
			huh.NewInput().Title("Server port").Description("Chatto accepts HTTP connections on this port. Keep 4000 unless another service already uses it.").Value(port).Validate(validate(validateInitPort)),
			huh.NewInput().Title("Public URL").
				Description("The address people use to open Chatto, also used for links and sign-in redirects. Leave blank for local use, or enter an address such as https://chat.example.com.").
				PlaceholderFunc(func() string { return "http://localhost:" + *port }, port).Value(&opts.PublicURL).Validate(validate(validateInitURL)),
		).Title("Server").Description("For a local server, keep the defaults. For internet access, an HTTPS reverse proxy forwards requests from your public address to the port below. Configure the proxy and DNS separately."),
		newInitPage(
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Open registration").
				Description("Let people create password accounts. SSO has its own setting.").
				Affirmative("Open").Negative("Closed").Value(&opts.DirectRegistration),
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Password login").Description("Keep enabled for owner setup. You can switch to SSO-only later.").Affirmative("Enabled").Negative("SSO only").Value(&opts.DirectLogin),
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Single sign-on").Description("Use an OpenID Connect identity provider.").Affirmative("Configure SSO").Negative("Not now").Value(&opts.WithSSO),
		).Title("Access").Description("Choose how people join and sign in."),
		newInitPage(
			huh.NewInput().Title("Issuer URL").Description("The issuer from your OpenID Connect provider, including its realm or tenant path.").Placeholder("https://id.example.com/realms/community").Value(&opts.SSO.IssuerURL).Validate(validate(validateInitIssuer)),
			huh.NewInput().Title("Sign-in button label").Value(&opts.SSO.Label).Validate(validate(initRequired)),
			huh.NewNote().DescriptionFunc(func() string {
				return "Register this redirect URI with your provider:\n" + initSSOCallback(*opts, *port)
			}, struct {
				Options *initOptions
				Port    *string
			}{opts, port}),
		).Title("SSO · Provider").Description("Your provider receives sign-in requests and user IP addresses. Chatto receives the provider identity.").WithHideFunc(func() bool { return !opts.WithSSO }),
		newInitPage(
			huh.NewInput().Title("Client ID").Value(&opts.SSO.ClientID).Validate(validate(initRequired)),
			huh.NewInput().Title("Client secret").Description("Leave blank only if your provider uses a public OIDC client.").EchoMode(huh.EchoModePassword).Value(&opts.SSO.ClientSecret),
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Allow new accounts through SSO?").Description("Unlinked identities may create an account after confirmation. When off, users must first link SSO to an existing account.").Affirmative("Allow").Negative("Linked accounts only").Value(&opts.SSOAutoProvision),
		).Title("SSO · Client").WithHideFunc(func() bool { return !opts.WithSSO }),
		newInitPage(
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Where should Chatto store its data?").
				Description("Embedded is the easiest way to start: Chatto manages NATS and saves data in ./data. Choose External if you already run a NATS server with JetStream.").
				Affirmative("Embedded").Negative("External").Value(&opts.EmbeddedNATS),
		).Title("Storage"),
		newInitPage(
			huh.NewInput().Title("NATS server URL").Description("Enable JetStream on this server. Separate multiple URLs with commas. Use tls:// for an encrypted connection.").Value(&opts.NATSClient.URL).Validate(validate(validateInitNATSURL)),
			huh.NewSelect[natsauth.AuthMethod]().Title("Authentication").Options(
				huh.NewOption("Token", natsauth.AuthToken),
				huh.NewOption("User/password", natsauth.AuthUserPass),
				huh.NewOption("Credentials file", natsauth.AuthCredentials),
				huh.NewOption("NKey", natsauth.AuthNKey),
				huh.NewOption("None", natsauth.AuthNone),
			).Inline(true).Value(&opts.NATSClient.AuthMethod),
		).Title("NATS connection").WithHideFunc(func() bool { return opts.EmbeddedNATS }),
		newInitPage(
			huh.NewInput().Title("NATS token").EchoMode(huh.EchoModePassword).Value(&opts.NATSClient.Token).Validate(validate(initRequired)),
		).Title("NATS authentication").WithHideFunc(func() bool { return opts.EmbeddedNATS || opts.NATSClient.AuthMethod != natsauth.AuthToken }),
		newInitPage(
			huh.NewInput().Title("NATS username").Value(&opts.NATSClient.Username).Validate(validate(initRequired)),
			huh.NewInput().Title("NATS password").EchoMode(huh.EchoModePassword).Value(&opts.NATSClient.Password),
		).Title("NATS authentication").WithHideFunc(func() bool { return opts.EmbeddedNATS || opts.NATSClient.AuthMethod != natsauth.AuthUserPass }),
		newInitPage(
			huh.NewInput().Title("NATS credentials file").Description("Path to your .creds file, relative to the directory where you run Chatto.").Value(&opts.NATSClient.CredentialsFile).Validate(validate(initRequired)),
		).Title("NATS authentication").WithHideFunc(func() bool { return opts.EmbeddedNATS || opts.NATSClient.AuthMethod != natsauth.AuthCredentials }),
		newInitPage(
			huh.NewInput().Title("NATS NKey seed").EchoMode(huh.EchoModePassword).Value(&opts.NATSClient.NKeySeed).Validate(validate(initRequired)),
		).Title("NATS authentication").WithHideFunc(func() bool { return opts.EmbeddedNATS || opts.NATSClient.AuthMethod != natsauth.AuthNKey }),
		newInitPage(
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Message search").
				Description("Runs inside Chatto. Protect ./data/search: it contains decrypted messages.").
				Affirmative("Enabled").Negative("Off").Value(&opts.WithSearch),
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Voice and video").
				Description("Creates livekit.yaml for a separate local server. Participants connect to LiveKit directly, sharing their IP addresses.").
				Affirmative("Add LiveKit").Negative("Off").Value(&opts.WithLiveKit),
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Outgoing email").
				Description("For verification and password resets. Your SMTP provider receives recipients and email content.").
				Affirmative("Configure SMTP").Negative("Later").Value(&opts.SMTP.Enabled),
		).Title("Features"),
		newInitPage(
			huh.NewInput().Title("SMTP server").Placeholder("smtp.example.com").Value(&opts.SMTP.Host).Validate(validate(initRequired)),
			huh.NewInput().Title("SMTP port").Value(smtpPort).Validate(validate(validateInitPort)),
			huh.NewSelect[config.SMTPTLSPolicy]().Title("Connection security").
				Description("STARTTLS usually uses port 587; TLS uses port 465.").
				Options(huh.NewOption("STARTTLS", config.SMTPTLSMandatory), huh.NewOption("TLS", config.SMTPTLSImplicit)).Inline(true).Value(&opts.SMTP.TLS),
		).Title("Email connection").WithHideFunc(func() bool { return !opts.SMTP.Enabled }),
		newInitPage(
			huh.NewInput().Title("Sender address").Placeholder("noreply@example.com").Value(&opts.SMTP.From).Validate(validate(validateInitSender)),
			huh.NewInput().Title("SMTP username").Description("Leave blank if your provider does not require authentication.").Value(&opts.SMTP.Username),
			huh.NewInput().Title("SMTP password").EchoMode(huh.EchoModePassword).Value(&opts.SMTP.Password),
		).Title("Email identity").WithHideFunc(func() bool { return !opts.SMTP.Enabled }),
		huh.NewGroup(
			huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Title("Create these configuration files?").DescriptionFunc(func() string {
				return initReview(*opts, *port)
			}, struct {
				Options *initOptions
				Port    *string
			}{opts, port}).Affirmative("Create files").Negative("Cancel").Value(confirmed).
				Validate(func(yes bool) error {
					if *backward || !yes {
						return nil
					}
					if err := validateInitAnswers(*opts, *port, *smtpPort); err != nil {
						return err
					}
					if err := validateInitPath(opts.ConfigPath); err != nil {
						return err
					}
					if opts.WithLiveKit {
						path := filepath.Join(filepath.Dir(opts.ConfigPath), "livekit.yaml")
						if filepath.Clean(opts.ConfigPath) == path {
							return errors.New("choose a Chatto config filename other than livekit.yaml")
						}
						return validateInitPath(path)
					}
					return nil
				}),
		).Title("Review"),
	).WithKeyMap(keys).WithShowHelp(false)
}

// newInitPage adds an explicit page boundary. Arrow keys move between questions;
// Enter on Continue advances to the next relevant page.
func newInitPage(fields ...huh.Field) *huh.Group {
	return huh.NewGroup(append(fields, huh.NewConfirm().WithButtonAlignment(lipgloss.Left).Affirmative("Continue →").Negative("").Value(new(true)))...)
}

// initKeyMap reserves vertical arrows for question navigation. Inline choices use
// horizontal arrows, so the same movement works on every screen. Submission
// keeps its separate Enter binding; moving down cannot write configuration files.
func initKeyMap() *huh.KeyMap {
	keys := huh.NewDefaultKeyMap()
	for _, binding := range []*key.Binding{&keys.Input.Prev, &keys.Confirm.Prev, &keys.Select.Prev} {
		binding.SetKeys("up", "shift+tab")
		binding.SetHelp("↑/shift+tab", "back")
	}
	for _, binding := range []*key.Binding{&keys.Input.Next, &keys.Confirm.Next, &keys.Select.Next} {
		binding.SetKeys("down", "tab", "enter")
		binding.SetHelp("↓/tab", "next")
	}
	return keys
}

// initTheme keeps the form readable on both light and dark terminal backgrounds.
func initTheme(dark bool) *huh.Styles {
	styles := huh.ThemeBase(dark)
	accent, muted := initPalette(dark)
	title := lipgloss.NewStyle().Bold(true)
	description := lipgloss.NewStyle().Foreground(muted)
	styles.Group.Title = title.Foreground(accent)
	styles.Group.Description = description.MarginBottom(2)
	styles.FieldSeparator = lipgloss.NewStyle().SetString("\n\n")
	styles.Focused.Base = styles.Focused.Base.BorderForeground(accent)
	styles.Focused.Title = title.Foreground(accent)
	styles.Focused.Description = description
	styles.Focused.TextInput.Prompt = lipgloss.NewStyle().Foreground(accent)
	styles.Focused.TextInput.Placeholder = description
	styles.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(accent)
	styles.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75"))
	styles.Focused.ErrorIndicator = styles.Focused.ErrorMessage.SetString(" !")
	styles.Focused.FocusedButton = lipgloss.NewStyle().Foreground(lipgloss.Color("#101820")).Background(accent).Bold(true).Padding(0, 1).MarginRight(1)
	if !dark {
		styles.Focused.FocusedButton = styles.Focused.FocusedButton.Foreground(lipgloss.Color("#FFFFFF"))
	}
	styles.Focused.BlurredButton = lipgloss.NewStyle().Foreground(muted).Padding(0, 1).MarginRight(1)
	styles.Focused.SelectSelector = lipgloss.NewStyle().Foreground(accent).SetString("› ")
	styles.Blurred = styles.Focused
	styles.Blurred.Base = styles.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	styles.Blurred.Title = title
	// Only the active field gets a filled button. Selection is not keyboard focus.
	styles.Blurred.FocusedButton = lipgloss.NewStyle().Bold(true).Underline(true).Padding(0, 1).MarginRight(1)
	return styles
}

// initPalette keeps the shell and form focus colors consistent.
func initPalette(dark bool) (accent, muted color.Color) {
	if dark {
		return lipgloss.Color("#7DD3C7"), lipgloss.Color("#A0A7B4")
	}
	return lipgloss.Color("#176B63"), lipgloss.Color("#596375")
}

func initReview(opts initOptions, port string) string {
	registration := "Closed"
	if opts.DirectRegistration {
		registration = "Open"
	}
	storage := "Embedded · ./data"
	if !opts.EmbeddedNATS {
		storage = "External · " + string(opts.NATSClient.AuthMethod) + " authentication"
	}
	lines := []string{
		"Storage       " + storage,
		"Server URL    " + initPublicURL(opts.PublicURL, port),
		"Listen port   " + port,
		"Registration  " + registration,
		"Password login " + initFeatureLabel(opts.DirectLogin),
		"SSO           " + initFeatureLabel(opts.WithSSO),
		"Search        " + initFeatureLabel(opts.WithSearch),
		"Email         " + initFeatureLabel(opts.SMTP.Enabled),
		"Calls         " + initFeatureLabel(opts.WithLiveKit),
		"",
		"Files to create",
		"  " + opts.ConfigPath,
	}
	if opts.WithLiveKit {
		lines = append(lines, "  "+filepath.Join(filepath.Dir(opts.ConfigPath), "livekit.yaml"))
	}
	if !opts.DirectLogin {
		lines = append(lines, "", "First-run setup needs auth.direct_login = true.", "Create your owner and link SSO before disabling it.")
	}
	return strings.Join(lines, "\n") + "\n\n↑ to edit your answers · Ctrl+C to cancel\nNothing is written until you choose Create files."
}

func initFeatureLabel(value bool) string {
	if value {
		return "Enabled"
	}
	return "Off"
}

func initPublicURL(value, port string) string {
	if value == "" {
		return "http://localhost:" + port
	}
	return strings.TrimRight(value, "/")
}

// validateInitAnswers checks the complete draft again before file creation,
// including fields the user left while navigating backwards.
func validateInitAnswers(opts initOptions, port, smtpPort string) error {
	if err := validateInitAuth(opts); err != nil {
		return err
	}
	if err := validateInitPort(port); err != nil {
		return fmt.Errorf("listen port: %w", err)
	}
	if err := validateInitURL(opts.PublicURL); err != nil {
		return err
	}
	if !opts.EmbeddedNATS {
		if err := validateInitNATSClient(opts.NATSClient); err != nil {
			return err
		}
	}
	if opts.SMTP.Enabled {
		if err := initRequired(opts.SMTP.Host); err != nil {
			return fmt.Errorf("SMTP server: %w", err)
		}
		if err := validateInitPort(smtpPort); err != nil {
			return fmt.Errorf("SMTP port: %w", err)
		}
		return validateInitSender(opts.SMTP.From)
	}
	return nil
}

func validateInitSender(value string) error {
	if _, err := mail.ParseAddress(value); err != nil {
		return errors.New("enter a valid sender email address")
	}
	return nil
}

func initRequired(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("please enter a value")
	}
	return nil
}

func validateInitPort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("choose a port between 1 and 65535")
	}
	return nil
}

func validateInitURL(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("use an HTTP or HTTPS origin without credentials, a path, a query, or a fragment")
	}
	if u.Port() != "" {
		return validateInitPort(u.Port())
	}
	return nil
}

func validateInitPath(path string) error {
	if err := initRequired(path); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("that file already exists; choose a new filename")
	} else if !os.IsNotExist(err) {
		return errors.New("cannot access that config path")
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || !info.IsDir() {
		return errors.New("choose a file in an existing directory")
	}
	return nil
}

func printInitNextSteps(cmd *cobra.Command, opts initOptions) {
	heading := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A6E3A1"))
	cmd.Println(heading.Render("All set. Your community starts here."))
	cmd.Printf("\nStart Chatto:\n  chatto run --config %s\n", initShellQuote(opts.ConfigPath))
	if opts.WithLiveKit {
		cmd.Printf("\nIn another terminal, start LiveKit:\n  livekit-server --config %s\n", initShellQuote(filepath.Join(filepath.Dir(opts.ConfigPath), "livekit.yaml")))
	}
	if opts.WithSSO {
		cmd.Printf("\nRegister this redirect URI with your SSO provider:\n  %s\n", initSSOCallback(opts, strconv.Itoa(opts.Port)))
	}
	if !opts.DirectLogin {
		cmd.Println("\nBefore first-run setup, temporarily set auth.direct_login = true in the config.")
		cmd.Println("Create your owner account, sign in, and link SSO. Then restore false and restart Chatto.")
	}
	cmd.Printf("\nOpen %s to name your server and create your owner account.\n", opts.PublicURL)
	cmd.Println("Keep access to the server restricted until you complete that step.")
}

// initShellQuote preserves paths with spaces or shell metacharacters in copyable commands.
func initShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
