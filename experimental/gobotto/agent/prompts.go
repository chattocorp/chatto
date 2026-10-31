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

func systemMessage(identity Identity, operatorPrompt string, ownerUserIDs ...string) Message {
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
	return textMessage("system", strings.Join(parts, "\n\n"))
}
