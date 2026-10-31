package agent

import (
	_ "embed"
	"encoding/json"
	"slices"
	"strings"
)

// Internal prompts are embedded so an installed binary does not depend on the
// source tree. Operator configuration supplies a separate, replaceable section.
//
//go:embed prompts/chatto.txt
var chattoPrompt string

//go:embed prompts/behavior.txt
var behaviorPrompt string

//go:embed prompts/means.txt
var meansPrompt string

func systemMessage(identity Identity, operatorPrompt string, ownerUserIDs ...string) Message {
	return systemMessageForMemory(identity, operatorPrompt, meansPrompt, ownerUserIDs...)
}

func systemMessageForMemory(identity Identity, operatorPrompt, memoryPrompt string, ownerUserIDs ...string) Message {
	parts := []string{strings.TrimSpace(chattoPrompt), strings.TrimSpace(behaviorPrompt)}
	if strings.TrimSpace(operatorPrompt) != "" {
		parts = append(parts, "Operator instructions:\n"+operatorPrompt)
	}
	if identity.ID != "" {
		data, _ := json.Marshal(identity)
		parts = append(parts, "Authenticated Chatto identity (JSON): "+string(data))
	}
	owners := append([]string{}, ownerUserIDs...)
	slices.Sort(owners)
	owners = slices.Compact(owners)
	data, _ := json.Marshal(owners)
	parts = append(parts, "Current configured owner user IDs (JSON): "+string(data))
	if strings.TrimSpace(memoryPrompt) != "" {
		parts = append(parts, strings.TrimSpace(memoryPrompt))
	}
	return textMessage("system", strings.Join(parts, "\n\n"))
}
