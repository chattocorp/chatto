package core

// IsValidEmojiName checks if a name is a known emoji shortcode.
// Uses the generated emojiNameToUnicode map (from gemoji).
func IsValidEmojiName(name string) bool {
	_, ok := emojiNameToUnicode[name]
	return ok
}

// resolveEmojiInput validates that the input is a known emoji shortcode name.
// The API only accepts shortcode names (e.g., "thumbsup"), not Unicode emoji.
func resolveEmojiInput(input string) (string, error) {
	if IsValidEmojiName(input) {
		return input, nil
	}
	return "", invalidArgument("unsupported emoji")
}
