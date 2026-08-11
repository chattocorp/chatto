package connectapi

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNotificationThreadRootExcerpt(t *testing.T) {
	t.Run("collapses whitespace", func(t *testing.T) {
		if got := notificationThreadRootExcerpt("  First\n\tthread   context  "); got != "First thread context" {
			t.Fatalf("excerpt = %q, want collapsed whitespace", got)
		}
	})

	t.Run("truncates by Unicode code point", func(t *testing.T) {
		got := notificationThreadRootExcerpt(strings.Repeat("é", notificationThreadRootExcerptMaxRunes+1))
		if utf8.RuneCountInString(got) != notificationThreadRootExcerptMaxRunes || !strings.HasSuffix(got, "…") {
			t.Fatalf("excerpt rune count/suffix = %d/%q, want %d runes ending in ellipsis", utf8.RuneCountInString(got), got, notificationThreadRootExcerptMaxRunes)
		}
	})
}
