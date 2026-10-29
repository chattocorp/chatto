package core

// HistoricalReactionName preserves valid reaction names and converts supported
// Unicode emoji from existing archives with the historical selection rule.
func HistoricalReactionName(input string) (string, error) {
	if IsValidReactionName(input) {
		return input, nil
	}
	return HistoricalEmojiName(input)
}

// HistoricalEmojiName selects a stable bundled shortcode without changing the Unicode
// sequence. Unknown sequences fail preflight rather than lose a reaction.
func HistoricalEmojiName(unicode string) (string, error) {
	name := ""
	for candidate, value := range emojiNameToUnicode {
		if value == unicode && (name == "" || len(candidate) < len(name) || (len(candidate) == len(name) && candidate < name)) {
			name = candidate
		}
	}
	if name == "" {
		return "", invalidArgument("unsupported Unicode emoji")
	}
	return name, nil
}
