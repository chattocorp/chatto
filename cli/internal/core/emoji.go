package core

var emojiUnicodeSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(emojiNameToUnicode))
	for _, emoji := range emojiNameToUnicode {
		set[emoji] = struct{}{}
	}
	return set
}()

// IsValidEmojiName checks if a name is a known emoji shortcode.
// Uses the generated emojiNameToUnicode map (from gemoji).
func IsValidEmojiName(name string) bool {
	_, ok := emojiNameToUnicode[name]
	return ok
}

// IsValidUnicodeEmoji reports whether emoji is one complete emoji from the
// bundled gemoji dataset. This includes multi-code-point emoji such as flags
// and family sequences, while rejecting arbitrary text and multiple adjacent
// emoji.
func IsValidUnicodeEmoji(emoji string) bool {
	_, ok := emojiUnicodeSet[emoji]
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
