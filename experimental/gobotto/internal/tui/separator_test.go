package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSeparatorBackgroundHasNoNestedStyleResets(t *testing.T) {
	plain := " Turn 3" + strings.Repeat(" ", 30) + "DELIVERED "
	nested := "\x1b[36m Turn 3\x1b[0m" + strings.Repeat(" ", 30) + "\x1b[42mDELIVERED \x1b[0m"
	got := separatorBar(nested, 60, false)
	if got != separatorBar(plain, 60, false) {
		t.Fatal("nested styles split the background band")
	}
	if ansi.StringWidth(got) != 60 || ansi.Strip(got) != plain+strings.Repeat(" ", 60-len(plain)) {
		t.Fatal("background band does not cover the complete row")
	}
}
