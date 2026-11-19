package main

import (
	"errors"
	"testing"

	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestChattoCheckpointPublishesOnlyAcceptedBoundaries(t *testing.T) {
	current := "old"
	reject := true
	checkpoint := chattoCheckpoint(func(cursor string, event *chatto.MessageEvent) error {
		if reject {
			return errors.New("inbox unavailable")
		}
		return nil
	}, func(cursor string) { current = cursor })
	if err := checkpoint("new", &chatto.MessageEvent{}); err == nil || current != "old" {
		t.Fatal("failed checkpoint advanced MCP boundary")
	}
	reject = false
	if err := checkpoint("new", &chatto.MessageEvent{}); err != nil || current != "new" {
		t.Fatal("accepted checkpoint did not advance MCP boundary")
	}
	if err := checkpoint("", &chatto.MessageEvent{RecoveryGap: true}); err != nil || current != "" {
		t.Fatal("recovery gap kept stale MCP boundary")
	}
}
