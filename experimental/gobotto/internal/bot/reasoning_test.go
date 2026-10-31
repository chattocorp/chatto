package bot

import (
	"encoding/json"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

func TestReasoningTotalsPreservePartialAndLegacyUsage(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		usage := totals{Known: legacy}
		usage.addReasoning(agent.Usage{Reasoning: 25, ReasoningKnown: true})
		usage.Known = true
		if usage.Reasoning != 25 || !usage.ReasoningKnown || usage.ReasoningMissing != legacy {
			t.Fatal("incorrect initial reasoning total")
		}
		data, err := json.Marshal(usage)
		if err != nil {
			t.Fatal(err)
		}
		var restored totals
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		restored.addReasoning(agent.Usage{})
		restored.addReasoning(agent.Usage{Reasoning: 5, ReasoningKnown: true})
		if restored.Reasoning != 30 || !restored.ReasoningKnown || !restored.ReasoningMissing {
			t.Fatal("partial telemetry lost across restore")
		}
	}
}
