package tui

import (
	"context"
	"errors"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// Emit sends a fixed operational event to the UI. Sending stops on cancellation.
type Emit func(Event)

type finished struct{ err error }

type shutdownKey struct{}

// ShutdownRequested closes when the user requests a graceful quit. Workers
// finish active work and return; context cancellation remains the force path.
// It returns nil outside Run.
func ShutdownRequested(ctx context.Context) <-chan struct{} {
	ch, _ := ctx.Value(shutdownKey{}).(<-chan struct{})
	return ch
}

// Run owns the terminal and worker lifetime. Normal quit requests graceful
// shutdown; Shift-Q cancels active work. The worker must honor both signals
// and finish all Emit calls before it returns.
func Run(ctx context.Context, work func(context.Context, Emit) error, options ...tea.ProgramOption) error {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	shutdown := make(chan struct{})
	workerCtx = context.WithValue(workerCtx, shutdownKey{}, (<-chan struct{})(shutdown))
	var requestOnce sync.Once
	events := make(chan tea.Msg, 128)
	done := make(chan error, 1)
	emit := func(event Event) {
		select {
		case events <- event:
		case <-workerCtx.Done():
		}
	}
	go func() {
		defer close(events)
		err := work(workerCtx, emit)
		select {
		case events <- finished{err}:
		case <-workerCtx.Done():
		}
		done <- err
	}()
	m := newModel(events)
	m.requestQuit = func() { requestOnce.Do(func() { close(shutdown) }) }
	m.switchMode(true)
	m.logsFocused = false
	options = append(options, tea.WithContext(ctx))
	_, uiErr := tea.NewProgram(m, options...).Run()
	cancel()
	workerErr := <-done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if uiErr != nil {
		return uiErr
	}
	// User-requested cancellation is a clean shutdown, not a runtime failure.
	if errors.Is(workerErr, context.Canceled) {
		return nil
	}
	return workerErr
}
