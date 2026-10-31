package bot

import (
	"context"
	"sync"
	"time"
)

// startTyping runs best-effort live updates independently of model work. Its
// idempotent stop function cancels any in-flight request and joins the worker.
// Each request is bounded; failures never fail a turn or enter its transcript.
func startTyping(parent context.Context, client Client, key threadKey) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			requestCtx, cancelRequest := context.WithTimeout(ctx, 2*time.Second)
			_ = client.RefreshTypingIndicator(requestCtx, key.room, key.root)
			cancelRequest()
			// Start the delay after completion to avoid bursts after slow requests.
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done }) }
}
