package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

// displayText prevents terminal controls and bidi overrides in private content
// from affecting the surrounding interface. Clipboard actions use original text.
func displayText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
}

// reasoningLabel distinguishes absent telemetry from a reported zero.
func reasoningLabel(tokens int64, known, partial bool) string {
	if !known {
		return "reasoning n/a"
	}
	label := fmt.Sprintf("%d reasoning", tokens)
	if partial {
		label += " (partial)"
	}
	return label
}

// turnUsageSummary includes child and title calls without counting cached input
// twice. A plus marks an incomplete reported total; unknown is never zero.
func turnUsageSummary(t inspection.Turn) (string, string) {
	var tokens int64
	var cost float64
	known, missing, costKnown, costMissing := false, false, false, false
	for _, step := range t.Steps {
		u := step.Usage
		if step.Kind != "model" && step.Kind != "title" && !u.Known {
			continue
		}
		known = known || u.Known
		missing = missing || !u.Known
		tokens += u.Input + u.Output
		cost += u.Cost
		costKnown = costKnown || u.CostKnown
		costMissing = costMissing || u.CostMissing || !u.CostKnown
	}
	tokenText, costText := "— tok", "$—"
	if known {
		tokenText = fmt.Sprint(tokens)
		if tokens >= 1000000 {
			tokenText = fmt.Sprintf("%.1fM", float64(tokens)/1000000)
		} else if tokens >= 1000 {
			tokenText = fmt.Sprintf("%.1fk", float64(tokens)/1000)
		}
		if missing {
			tokenText += "+"
		}
		tokenText += " tok"
	}
	if costKnown {
		costText = fmt.Sprintf("$%.4f", cost)
		if costMissing {
			costText += "+"
		}
	}
	return tokenText, costText
}

func groupedTokens(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
