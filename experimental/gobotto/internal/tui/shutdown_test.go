package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestQuitWaitsAndOffersForceKey(t *testing.T) {
	m := newModel(nil)
	requests := 0
	m.requestQuit = func() { requests++ }
	for range 2 {
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
		if cmd != nil {
			t.Fatal("normal quit closed UI before worker finished")
		}
	}
	if requests != 1 || !strings.Contains(m.View().Content, "Waiting for active turn") || !strings.Contains(m.View().Content, "Shift-Q") {
		t.Fatal("missing graceful shutdown state")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'Q'})
	if cmd == nil {
		t.Fatal("Shift-Q did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Shift-Q did not issue Quit")
	}
	_, cmd = m.Update(finished{})
	if cmd == nil {
		t.Fatal("finished worker did not close waiting UI")
	}
}

func TestGracefulQuitSignalsWorkerWithoutCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	requested := false
	err := Run(ctx, func(ctx context.Context, emit Emit) error {
		select {
		case <-ShutdownRequested(ctx):
			requested = true
		case <-ctx.Done():
			t.Error("normal quit cancelled worker")
			return ctx.Err()
		}
		if ctx.Err() != nil {
			t.Error("worker context cancelled while finishing")
		}
		return nil
	}, tea.WithInput(strings.NewReader("q")), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	if err != nil || !requested {
		t.Fatal("graceful shutdown failed", err)
	}
}
