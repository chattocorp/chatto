package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// initWizard owns terminal layout and lifecycle. Huh owns field navigation and
// validation. File creation stays outside both models, after explicit approval.
type initWizard struct {
	form                               *huh.Form
	configPath                         string
	width, height                      int
	welcome, cancelled, backward, dark bool
}

func newInitWizard(opts *initOptions, port, smtpPort *string, confirmed *bool) *initWizard {
	wizard := &initWizard{configPath: opts.ConfigPath, width: 80, height: 24, welcome: true, dark: true}
	wizard.form = newInitForm(opts, port, smtpPort, confirmed, &wizard.backward)
	wizard.form.WithTheme(huh.ThemeFunc(func(bool) *huh.Styles { return initTheme(wizard.dark) }))
	wizard.resize()
	return wizard
}

func (w *initWizard) Init() tea.Cmd {
	return tea.Batch(tea.RequestWindowSize, tea.RequestBackgroundColor)
}

func (w *initWizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width, w.height = msg.Width, msg.Height
		w.resize()
	case tea.BackgroundColorMsg:
		w.dark = msg.IsDark()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			w.cancelled = true
			return w, tea.Quit
		}
		// Do not change hidden answers while the terminal is too small to show them.
		if w.width < 48 || w.height < 16 {
			return w, nil
		}
		if w.welcome {
			if msg.String() == "enter" {
				w.welcome = false
				return w, w.form.Init()
			}
			return w, nil
		}
		w.backward = msg.String() == "up" || msg.String() == "shift+tab"
	}
	// Pass resize and palette changes through even on the welcome screen.
	_, cmd := w.form.Update(msg)
	if w.form.State == huh.StateCompleted {
		return w, tea.Quit
	}
	if w.form.State == huh.StateAborted {
		w.cancelled = true
		return w, tea.Quit
	}
	return w, cmd
}

func (w *initWizard) contentWidth() int  { return max(1, min(72, w.width-6)) }
func (w *initWizard) contentHeight() int { return max(1, w.height-6) }

func (w *initWizard) resize() {
	w.form.WithWidth(w.contentWidth()).WithHeight(w.contentHeight())
}

// View uses every terminal row: three header rows, a separator row, a flexible
// content area, and two footer rows. Long forms scroll within Huh's viewport.
func (w *initWizard) View() tea.View {
	accent, muted := initPalette(w.dark)
	title := lipgloss.NewStyle().Bold(true).Foreground(accent)
	quiet := lipgloss.NewStyle().Foreground(muted)
	width, height := w.contentWidth(), w.contentHeight()
	rule := quiet.Render(strings.Repeat("─", width))
	header := title.Render("chatto") + "  " + quiet.Render("/  Server setup") + "\n" +
		lipgloss.NewStyle().MaxWidth(width).Render(quiet.Render("Save to "+w.configPath)) + "\n" + rule
	body := w.form.View()
	footer := "↑/↓ move  ←/→ choose  Enter continue  Ctrl+C cancel"
	if w.welcome {
		buttonText := lipgloss.Color("#FFFFFF")
		if w.dark {
			buttonText = lipgloss.Color("#24123D")
		}
		button := lipgloss.NewStyle().Bold(true).Foreground(buttonText).Background(accent).Padding(0, 2)
		card := title.Render("Welcome to Chatto.") + "\n\n" +
			"A place for your people to talk, share, and belong.\n\n" +
			"We'll walk through:\n\n" +
			"  1  Server, sign-in, and storage\n" +
			"  2  Search, calls, and email — if you want them\n" +
			"  3  A review before anything is saved\n\n" +
			quiet.Render("The defaults work for a local server.\nYou can go back and change your answers.") + "\n\n" +
			button.Render("Let's get started  →")
		if height < 18 || width < 60 {
			card = title.Render("Welcome to Chatto.") + "\n\n" +
				"Set up your server, choose features,\nand review before saving.\n\n" +
				button.Render("Let's get started  →")
		}
		card = lipgloss.NewStyle().Width(width).Render(card)
		body = strings.Repeat("\n", max(0, (height-lipgloss.Height(card))/2)) + card
		footer = "Enter get started  ·  Ctrl+C cancel"
	}
	if w.width < 48 || w.height < 16 {
		view := tea.NewView(fmt.Sprintf("Chatto setup\n\nResize to at least 48 × 16.\nCurrent size: %d × %d\n\nCtrl+C to cancel.", w.width, w.height))
		view.AltScreen = true
		return view
	}
	if !w.welcome && width < 60 {
		footer = "↑↓ move · ←→ choose · Enter next · ^C quit"
	}
	body = lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(body)
	content := header + "\n\n" + body + "\n" + rule + "\n" + quiet.Render(footer)
	view := tea.NewView(lipgloss.PlaceHorizontal(w.width, lipgloss.Center, lipgloss.NewStyle().Width(width).Align(lipgloss.Left).Render(content)))
	view.AltScreen = true
	return view
}
