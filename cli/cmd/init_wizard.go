package cmd

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"hmans.de/chatto/internal/config"
)

type initNATSMode string

const (
	initNATSEmbedded initNATSMode = "embedded"
	initNATSExternal initNATSMode = "external"
)

type initAnswers struct {
	PublicURL           string
	ListenPort          string
	NATSMode            initNATSMode
	NATSReplicas        int
	EmbeddedDataDir     string
	ExternalNATSURL     string
	NATSAuthMethod      config.NATSAuthMethod
	NATSToken           string
	NATSUsername        string
	NATSPassword        string
	NATSCredentialsFile string
	NATSNKeySeed        string
	Confirmed           bool
}

type initWizardOptions struct {
	input      io.Reader
	output     io.Writer
	accessible bool
	configPath string
}

func defaultInitAnswers() initAnswers {
	return initAnswers{
		PublicURL:       "http://localhost:4000",
		ListenPort:      "4000",
		NATSMode:        initNATSEmbedded,
		NATSReplicas:    1,
		EmbeddedDataDir: "./data",
		ExternalNATSURL: "nats://localhost:4222",
		NATSAuthMethod:  config.NATSAuthCredentials,
		Confirmed:       true,
	}
}

func runInitWizard(answers *initAnswers, opts initWizardOptions) error {
	if opts.accessible {
		return runAccessibleInitWizard(answers, opts)
	}

	embedded := initEmbeddedNATSGroup(answers).WithHideFunc(func() bool {
		return answers.NATSMode != initNATSEmbedded
	})
	external := initExternalNATSGroup(answers).WithHideFunc(func() bool {
		return answers.NATSMode != initNATSExternal
	})
	credentials := initNATSCredentialsGroup(answers, config.NATSAuthCredentials).WithHideFunc(func() bool {
		return answers.NATSMode != initNATSExternal || answers.NATSAuthMethod != config.NATSAuthCredentials
	})
	token := initNATSCredentialsGroup(answers, config.NATSAuthToken).WithHideFunc(func() bool {
		return answers.NATSMode != initNATSExternal || answers.NATSAuthMethod != config.NATSAuthToken
	})
	userpass := initNATSCredentialsGroup(answers, config.NATSAuthUserPass).WithHideFunc(func() bool {
		return answers.NATSMode != initNATSExternal || answers.NATSAuthMethod != config.NATSAuthUserPass
	})
	nkey := initNATSCredentialsGroup(answers, config.NATSAuthNKey).WithHideFunc(func() bool {
		return answers.NATSMode != initNATSExternal || answers.NATSAuthMethod != config.NATSAuthNKey
	})

	return newInitForm(opts,
		initWelcomeGroup(),
		initFrontDoorGroup(answers),
		initNATSModeGroup(answers),
		embedded,
		external,
		credentials,
		token,
		userpass,
		nkey,
		initReviewGroup(answers, opts.configPath, true),
	).Run()
}

func runAccessibleInitWizard(answers *initAnswers, opts initWizardOptions) error {
	if err := newInitForm(opts,
		initWelcomeGroup(),
		initFrontDoorGroup(answers),
		initNATSModeGroup(answers),
	).Run(); err != nil {
		return err
	}

	if answers.NATSMode == initNATSEmbedded {
		if err := newInitForm(opts, initEmbeddedNATSGroup(answers)).Run(); err != nil {
			return err
		}
	} else {
		if err := newInitForm(opts, initExternalNATSGroup(answers)).Run(); err != nil {
			return err
		}
		if answers.NATSAuthMethod != config.NATSAuthNone {
			if err := newInitForm(opts, initNATSCredentialsGroup(answers, answers.NATSAuthMethod)).Run(); err != nil {
				return err
			}
		}
	}

	return newInitForm(opts, initReviewGroup(answers, opts.configPath, false)).Run()
}

func newInitForm(opts initWizardOptions, groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithTheme(chattoInitTheme{}).
		WithAccessible(opts.accessible).
		WithInput(opts.input).
		WithOutput(opts.output).
		WithShowHelp(true).
		WithShowErrors(true)
}

func initWelcomeGroup() *huh.Group {
	return huh.NewGroup(
		huh.NewNote().
			Title("╭─ chatto init ─╮\n╰─ a new conversation starts here").
			Description("Let’s find the front door, tune the engine room, and mint the secrets.\nNothing is written until you approve the launch card.").
			Next(true).
			NextLabel("Let’s do this →"),
	).Title("Welcome")
}

func initFrontDoorGroup(answers *initAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("Where will people open Chatto?").
			Description("The public URL used in links and browser connections.").
			Placeholder("https://chat.example.com").
			Value(&answers.PublicURL).
			Validate(validatePublicURL),
		huh.NewInput().
			Title("Which local port should Chatto listen on?").
			Description("A reverse proxy may expose a different public port.").
			Value(&answers.ListenPort).
			Validate(validatePort),
	).Title("The front door").Description("Give browsers an address and the server a place to listen.")
}

func initNATSModeGroup(answers *initAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[initNATSMode]().
			Title("Where should Chatto remember everything?").
			Description("Embedded NATS is the easy, batteries-included choice.").
			Options(
				huh.NewOption("Embedded NATS  ·  recommended", initNATSEmbedded),
				huh.NewOption("External NATS  ·  existing server or cluster", initNATSExternal),
			).
			Value(&answers.NATSMode),
	).Title("The engine room").Description("Every conversation needs somewhere to remember itself.")
}

func initEmbeddedNATSGroup(answers *initAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("Where should embedded NATS keep its data?").
			Description("Use a persistent path in production.").
			Value(&answers.EmbeddedDataDir).
			Validate(validateNotBlank("data directory")),
	).Title("A room for the memories")
}

func initExternalNATSGroup(answers *initAnswers) *huh.Group {
	return huh.NewGroup(
		huh.NewInput().
			Title("How can Chatto reach NATS?").
			Description("Comma-separated URLs are accepted for cluster failover.").
			Placeholder("nats://nats-1:4222,nats://nats-2:4222").
			Value(&answers.ExternalNATSURL).
			Validate(validateNATSURLs),
		huh.NewSelect[int]().
			Title("How many JetStream replicas?").
			Description("This must fit the size of your NATS cluster.").
			Options(
				huh.NewOption("1  ·  single node", 1),
				huh.NewOption("3  ·  resilient cluster", 3),
				huh.NewOption("5  ·  larger resilient cluster", 5),
			).
			Value(&answers.NATSReplicas),
		huh.NewSelect[config.NATSAuthMethod]().
			Title("How does NATS authenticate Chatto?").
			Options(
				huh.NewOption("Credentials file  ·  recommended", config.NATSAuthCredentials),
				huh.NewOption("Token", config.NATSAuthToken),
				huh.NewOption("Username and password", config.NATSAuthUserPass),
				huh.NewOption("NKey seed", config.NATSAuthNKey),
				huh.NewOption("No authentication", config.NATSAuthNone),
			).
			Value(&answers.NATSAuthMethod),
	).Title("Connect the antenna")
}

func initNATSCredentialsGroup(answers *initAnswers, method config.NATSAuthMethod) *huh.Group {
	var fields []huh.Field
	switch method {
	case config.NATSAuthCredentials:
		fields = append(fields, huh.NewInput().
			Title("Credentials file").
			Description("Path to the NATS .creds file mounted for Chatto.").
			Placeholder("/run/secrets/chatto.creds").
			Value(&answers.NATSCredentialsFile).
			Validate(validateNotBlank("credentials file")))
	case config.NATSAuthToken:
		fields = append(fields, huh.NewInput().
			Title("NATS token").
			Description("The token is written to chatto.toml, which is created with mode 0600.").
			Password(true).
			Value(&answers.NATSToken).
			Validate(validateNotBlank("token")))
	case config.NATSAuthUserPass:
		fields = append(fields,
			huh.NewInput().
				Title("NATS username").
				Value(&answers.NATSUsername).
				Validate(validateNotBlank("username")),
			huh.NewInput().
				Title("NATS password").
				Password(true).
				Value(&answers.NATSPassword).
				Validate(validateNotBlank("password")),
		)
	case config.NATSAuthNKey:
		fields = append(fields, huh.NewInput().
			Title("NKey seed").
			Description("The seed is written to chatto.toml, which is created with mode 0600.").
			Password(true).
			Value(&answers.NATSNKeySeed).
			Validate(validateNotBlank("NKey seed")))
	}
	return huh.NewGroup(fields...).Title("NATS credentials")
}

func initReviewGroup(answers *initAnswers, configPath string, dynamic bool) *huh.Group {
	review := huh.NewNote().Title("Launch card")
	if dynamic {
		review = review.DescriptionFunc(func() string {
			return initAnswersSummary(*answers, configPath)
		}, answers)
	} else {
		// Huh's accessible renderer does not evaluate dynamic descriptions.
		// Accessible mode reaches this staged form only after all answers exist.
		review = review.Description(initAnswersSummary(*answers, configPath))
	}

	return huh.NewGroup(
		review,
		huh.NewConfirm().
			Title("Create this configuration?").
			Affirmative("Write configuration").
			Negative("Not yet").
			Value(&answers.Confirmed),
	).Title("Ready when you are")
}

func initAnswersSummary(answers initAnswers, configPath string) string {
	storage := fmt.Sprintf("Embedded NATS · %s", strings.TrimSpace(answers.EmbeddedDataDir))
	if answers.NATSMode == initNATSExternal {
		storage = fmt.Sprintf("External NATS · %s · %d replica(s) · %s auth",
			strings.TrimSpace(answers.ExternalNATSURL), answers.NATSReplicas, answers.NATSAuthMethod)
	}
	return fmt.Sprintf("Public URL   %s\nListen port  %s\nStorage      %s\nWrite        %s\n\nFresh secrets will be generated now.",
		strings.TrimSpace(answers.PublicURL), answers.ListenPort, storage, configPath)
}

func validatePublicURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("enter an absolute http:// or https:// URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("use an origin without credentials, a path, query, or fragment")
	}
	return nil
}

func validatePort(value string) error {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("enter a port from 1 to 65535")
	}
	return nil
}

func validateNATSURLs(value string) error {
	parts := strings.Split(strings.TrimSpace(value), ",")
	if len(parts) == 0 {
		return fmt.Errorf("enter at least one NATS URL")
	}
	for _, part := range parts {
		parsed, err := url.Parse(strings.TrimSpace(part))
		if err != nil || parsed.Host == "" {
			return fmt.Errorf("enter complete comma-separated NATS URLs")
		}
		switch parsed.Scheme {
		case "nats", "tls", "ws", "wss":
		default:
			return fmt.Errorf("NATS URLs must use nats://, tls://, ws://, or wss://")
		}
	}
	return nil
}

func validateNotBlank(name string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be empty", name)
		}
		return nil
	}
}

type chattoInitTheme struct{}

func (chattoInitTheme) Theme(isDark bool) *huh.Styles {
	styles := huh.ThemeBase(isDark)
	choose := lipgloss.LightDark(isDark)
	indigo := choose(lipgloss.Color("#4F46E5"), lipgloss.Color("#A5B4FC"))
	violet := choose(lipgloss.Color("#7C3AED"), lipgloss.Color("#C4B5FD"))
	cyan := choose(lipgloss.Color("#0891B2"), lipgloss.Color("#67E8F9"))
	muted := choose(lipgloss.Color("#64748B"), lipgloss.Color("#94A3B8"))
	errorColor := choose(lipgloss.Color("#DC2626"), lipgloss.Color("#FCA5A5"))

	styles.Group.Title = styles.Group.Title.Foreground(violet).Bold(true)
	styles.Group.Description = styles.Group.Description.Foreground(muted)
	styles.Focused.Title = styles.Focused.Title.Foreground(indigo).Bold(true)
	styles.Focused.Description = styles.Focused.Description.Foreground(muted)
	styles.Focused.SelectSelector = styles.Focused.SelectSelector.Foreground(cyan).SetString("› ")
	styles.Focused.FocusedButton = styles.Focused.FocusedButton.Foreground(lipgloss.Color("#FFFFFF")).Background(violet).Bold(true)
	styles.Focused.NoteTitle = styles.Focused.NoteTitle.Foreground(violet).Bold(true)
	styles.Focused.Next = styles.Focused.Next.Foreground(cyan).Bold(true)
	styles.Focused.ErrorIndicator = styles.Focused.ErrorIndicator.Foreground(errorColor)
	styles.Focused.ErrorMessage = styles.Focused.ErrorMessage.Foreground(errorColor)
	styles.Blurred.Title = styles.Blurred.Title.Foreground(muted)
	styles.Help.ShortKey = styles.Help.ShortKey.Foreground(cyan)
	styles.Help.ShortDesc = styles.Help.ShortDesc.Foreground(muted)
	return styles
}
