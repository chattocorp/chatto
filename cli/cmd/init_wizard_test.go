package cmd

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestInitWizardUsesTerminalDimensions(t *testing.T) {
	t.Parallel()
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 50}, {48, 16}} {
		opts := defaultInitOptions()
		port, smtpPort, confirmed := "4000", "587", false
		wizard := newInitWizard(&opts, &port, &smtpPort, &confirmed)
		wizard.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		for _, welcome := range []bool{true, false} {
			wizard.welcome = welcome
			view := wizard.View()
			if !view.AltScreen {
				t.Fatal("wizard must use the alternate screen")
			}
			if got := lipgloss.Height(view.Content); got != size.height {
				t.Fatalf("%dx%d welcome=%v: height=%d", size.width, size.height, welcome, got)
			}
			if got := lipgloss.Width(view.Content); got > size.width {
				t.Fatalf("%dx%d welcome=%v: width=%d", size.width, size.height, welcome, got)
			}
			if welcome && !strings.Contains(view.Content, "Let's get started") {
				t.Fatal("welcome must keep the start button visible")
			}
		}
	}
}

func TestInitWizardWelcomeAndSmallTerminal(t *testing.T) {
	t.Parallel()
	opts := defaultInitOptions()
	port, smtpPort, confirmed := "4000", "587", false
	wizard := newInitWizard(&opts, &port, &smtpPort, &confirmed)
	wizard.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if !wizard.welcome || confirmed {
		t.Fatal("welcome must wait for explicit Enter")
	}
	wizard.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	wizard.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !wizard.welcome || !strings.Contains(wizard.View().Content, "Resize") {
		t.Fatal("small terminals must show resize guidance without accepting hidden input")
	}
	wizard.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	wizard.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if wizard.welcome || confirmed {
		t.Fatal("Enter must start setup without approving file creation")
	}
	if value := wizard.form.GetFocusedField().GetValue(); value != "4000" {
		t.Fatalf("first question value = %v; want listen port", value)
	}
	wizard.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !wizard.cancelled || confirmed {
		t.Fatal("Ctrl+C must cancel without approving file creation")
	}
}
