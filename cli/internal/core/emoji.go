package core

import "strings"

var emojiUnicodeSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(emojiNameToUnicode))
	for _, emoji := range emojiNameToUnicode {
		set[emoji] = struct{}{}
	}
	return set
}()

// IsValidUnicodeEmoji reports whether emoji is one complete emoji from the
// bundled gemoji and Unicode skin-tone datasets. This includes flags
// and family sequences, while rejecting arbitrary text and multiple adjacent
// emoji.
func IsValidUnicodeEmoji(emoji string) bool {
	_, ok := emojiUnicodeSet[emoji]
	return ok
}

// IsValidReactionName accepts 1–128 ASCII letters, digits, underscores, plus
// signs, or hyphens. Names are case-sensitive and need not be in the bundled
// catalogue. Slack-style skin-tone suffixes are accepted only for complete
// sequences in the shared catalogue. Validation preserves stored identity.
func IsValidReactionName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	if strings.Contains(name, "::") {
		_, ok := emojiNameToUnicode[name]
		return ok
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '+' || c == '-') {
			return false
		}
	}
	return true
}

// resolveEmojiInput accepts reaction names without surrounding colons, not Unicode emoji.
// Unknown names remain usable before a client can render their emoji.
func resolveEmojiInput(input string) (string, error) {
	if IsValidReactionName(input) {
		return input, nil
	}
	return "", invalidArgument("invalid reaction name")
}
