package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

func TestInitWizardUsesTerminalDimensions(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 50}, {48, 16}} {
		opts := defaultInitOptions()
		port, smtpPort := "4000", "587"
		wizard := newInitWizard(&opts, &port, &smtpPort)
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
	opts := defaultInitOptions()
	port, smtpPort := "4000", "587"
	wizard := newInitWizard(&opts, &port, &smtpPort)
	wizard.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if !wizard.welcome || wizard.approved() {
		t.Fatal("welcome must wait for explicit Enter")
	}
	wizard.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	wizard.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !wizard.welcome || !strings.Contains(wizard.View().Content, "Resize") {
		t.Fatal("small terminals must show resize guidance without accepting hidden input")
	}
	wizard.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	wizard.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if wizard.welcome || wizard.approved() {
		t.Fatal("Enter must start setup without approving file creation")
	}
	if value := wizard.form.GetFocusedField().GetValue(); value != "4000" {
		t.Fatalf("first question value = %v; want listen port", value)
	}
	wizard.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !wizard.cancelled || wizard.approved() {
		t.Fatal("Ctrl+C must cancel without approving file creation")
	}
}

func TestInitWizardReview(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel bool
	}{
		{name: "Enter creates files"},
		{name: "select Cancel then Enter", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultInitOptions()
			opts.ConfigPath = filepath.Join(t.TempDir(), "chatto.toml")
			port, smtpPort := "4000", "587"
			w := newInitWizard(&opts, &port, &smtpPort)
			w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			for w.form.GetFocusedField().GetKey() != "createFiles" {
				w.Update(w.form.NextGroup()())
				if w.form.State == huh.StateCompleted {
					t.Fatal("form completed without a review")
				}
			}
			if w.approved() {
				t.Fatal("reaching the review must not approve file creation")
			}
			if tc.cancel {
				w.Update(tea.KeyPressMsg{Code: tea.KeyRight})
			}
			_, cmd := w.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			for cmd != nil && w.form.State != huh.StateCompleted {
				_, cmd = w.Update(cmd())
			}
			if w.form.State != huh.StateCompleted || w.approved() == tc.cancel {
				t.Fatalf("form state = %v, approved = %v", w.form.State, w.approved())
			}
		})
	}
}
