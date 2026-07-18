package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type initWizardStage uint8

const (
	initWizardIntro initWizardStage = iota
	initWizardForm
	initWizardMaxFormWidth = 88
	initWizardFrameDelay   = 65 * time.Millisecond
)

const initWizardLogo = ` ██████╗██╗  ██╗ █████╗ ████████╗████████╗ ██████╗
██╔════╝██║  ██║██╔══██╗╚══██╔══╝╚══██╔══╝██╔═══██╗
██║     ███████║███████║   ██║      ██║   ██║   ██║
██║     ██╔══██║██╔══██║   ██║      ██║   ██║   ██║
╚██████╗██║  ██║██║  ██║   ██║      ██║   ╚██████╔╝
 ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝      ╚═╝    ╚═════╝`

type initWizardTickMsg struct{}

type initWizardModel struct {
	form       *huh.Form
	stage      initWizardStage
	introFrame int
	width      int
	height     int
	hasDark    bool
}

func newInitWizardModel(form *huh.Form) *initWizardModel {
	return &initWizardModel{
		form:       form,
		stage:      initWizardIntro,
		introFrame: 1,
	}
}

func runInteractiveInitWizard(form *huh.Form, opts initWizardOptions) error {
	programOptions := make([]tea.ProgramOption, 0, 2)
	if opts.input != nil {
		programOptions = append(programOptions, tea.WithInput(opts.input))
	}
	if opts.output != nil {
		programOptions = append(programOptions, tea.WithOutput(opts.output))
	}

	finalModel, err := tea.NewProgram(newInitWizardModel(form), programOptions...).Run()
	if errors.Is(err, tea.ErrInterrupted) {
		return huh.ErrUserAborted
	}
	if err != nil {
		return fmt.Errorf("run terminal wizard: %w", err)
	}
	if finalModel.(*initWizardModel).form.State == huh.StateAborted {
		return huh.ErrUserAborted
	}
	return nil
}

func (m *initWizardModel) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tea.RequestWindowSize, initWizardTick())
}

func (m *initWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.stage == initWizardForm {
			return m.resizeForm()
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.hasDark = msg.IsDark()
		// Prime Huh's adaptive theme before the form becomes visible.
		updated, cmd := m.form.Update(msg)
		m.form = updated.(*huh.Form)
		return m, cmd
	case initWizardTickMsg:
		if m.stage != initWizardIntro || m.introFrame >= initWizardIntroFrames() {
			return m, nil
		}
		m.introFrame++
		return m, initWizardTick()
	case tea.KeyPressMsg:
		if m.stage == initWizardIntro {
			switch msg.String() {
			case "ctrl+c", "esc":
				return m, tea.Interrupt
			case "enter", "space":
				return m.startForm()
			default:
				return m, nil
			}
		}
	}

	if m.stage == initWizardForm {
		return m.updateForm(msg)
	}
	return m, nil
}

func (m *initWizardModel) startForm() (tea.Model, tea.Cmd) {
	m.stage = initWizardForm
	cmds := []tea.Cmd{m.form.Init()}
	if m.width > 0 && m.height > 0 {
		_, cmd := m.resizeForm()
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *initWizardModel) resizeForm() (tea.Model, tea.Cmd) {
	size := m.formWindowSize()
	m.form.WithWidth(size.Width)
	return m.updateForm(size)
}

func (m *initWizardModel) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.form.Update(msg)
	m.form = updated.(*huh.Form)
	switch m.form.State {
	case huh.StateCompleted:
		return m, tea.Quit
	case huh.StateAborted:
		return m, tea.Interrupt
	default:
		return m, cmd
	}
}

func (m *initWizardModel) formWindowSize() tea.WindowSizeMsg {
	width := max(m.width-8, initFormMinWidth)
	width = min(width, initWizardMaxFormWidth)
	return tea.WindowSizeMsg{Width: width, Height: max(m.height-4, 1)}
}

func (m *initWizardModel) View() tea.View {
	var content string
	if m.stage == initWizardIntro {
		content = initWizardIntroView(m.width, m.introFrame, m.hasDark)
	} else {
		content = m.form.View()
	}
	content = initWizardCenteredView(m.width, m.height, content)
	view := tea.NewView(content)
	view.AltScreen = true
	view.ReportFocus = true
	return view
}

// initWizardCenteredView centers a block without filling the entire terminal
// with trailing whitespace. Writing through the bottom-right cell can make
// some terminals scroll an alternate screen and corrupt subsequent repaints.
func initWizardCenteredView(width, height int, content string) string {
	if width <= 0 || height <= 0 {
		return content
	}
	contentWidth, contentHeight := lipgloss.Size(content)
	left := max((width-contentWidth)/2, 0)
	top := max((height-contentHeight)/2, 0)
	prefix := strings.Repeat(" ", left)
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Repeat("\n", top) + strings.Join(lines, "\n")
}

func initWizardTick() tea.Cmd {
	return tea.Tick(initWizardFrameDelay, func(time.Time) tea.Msg {
		return initWizardTickMsg{}
	})
}

func initWizardIntroFrames() int {
	return len(strings.Split(initWizardLogo, "\n")) + 2
}

func initWizardIntroView(width, frame int, isDark bool) string {
	logoLines := strings.Split(initWizardLogo, "\n")
	largeLogoWidth := lipgloss.Width(initWizardLogo)
	if width <= 0 || width < largeLogoWidth+4 {
		logoLines = []string{"chatto"}
	}

	visibleLogoLines := min(frame, len(logoLines))
	logoWidth := 0
	for _, line := range logoLines {
		logoWidth = max(logoWidth, lipgloss.Width(line))
	}
	choose := lipgloss.LightDark(isDark)
	logoStyle := lipgloss.NewStyle().Foreground(choose(lipgloss.Color("#7C3AED"), lipgloss.Color("#C4B5FD"))).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(choose(lipgloss.Color("#64748B"), lipgloss.Color("#94A3B8")))
	accentStyle := lipgloss.NewStyle().Foreground(choose(lipgloss.Color("#0891B2"), lipgloss.Color("#67E8F9"))).Bold(true)
	lines := make([]string, 0, len(logoLines)+3)
	for i, line := range logoLines {
		line += strings.Repeat(" ", logoWidth-lipgloss.Width(line))
		if i < visibleLogoLines {
			lines = append(lines, logoStyle.Render(line))
		} else {
			lines = append(lines, strings.Repeat(" ", logoWidth))
		}
	}
	lines = append(lines, "")
	if frame > len(logoLines) {
		lines = append(lines, mutedStyle.Render("Chat Just Got Real"))
	} else {
		lines = append(lines, strings.Repeat(" ", lipgloss.Width("Chat Just Got Real")))
	}
	if frame > len(logoLines)+1 {
		lines = append(lines, accentStyle.Render("enter to begin  •  ctrl+c to bail out"))
	} else {
		lines = append(lines, strings.Repeat(" ", lipgloss.Width("enter to begin  •  ctrl+c to bail out")))
	}
	return lipgloss.JoinVertical(lipgloss.Center, lines...)
}
