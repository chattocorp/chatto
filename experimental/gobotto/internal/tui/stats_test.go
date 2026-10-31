package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

func TestStatsDashboardLayout(t *testing.T) {
	turn := inspection.Turn{Number: 1, State: "delivered", Model: "z-ai/glm-5.3-flash", Wait: 2 * time.Second,
		Steps: []inspection.Step{{State: "rejected"}, {State: "complete"}}}
	stats := turnStats{input: groupedTokens(17201), output: "263", cache: "12800 cached (74%)", cacheValue: "74%", cost: "$0.000983", costDetail: "Reported by provider", reasoning: "81 reasoning", calls: 6}
	for _, width := range []int{120, 94, 70, 30} {
		view := renderTurnStats(turn, 7113*time.Millisecond, stats, width)
		if lipgloss.Width(view) > width {
			t.Fatalf("dashboard exceeds width %d", width)
		}
		for _, want := range []string{"INPUT TOKENS", "OUTPUT TOKENS", "CACHE HIT RATE", "TURN COST", "17,201", "$0.000983", "rejected", "delivered"} {
			if !strings.Contains(view, want) {
				t.Fatalf("dashboard at width %d lost %q", width, want)
			}
		}
		if width >= 94 && lipgloss.Height(view) != 6 {
			t.Fatalf("wide dashboard needs six rows, got %d", lipgloss.Height(view))
		}
	}
}
