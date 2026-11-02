package core

import "testing"

func TestSupplementalEmoji(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"🫩", "🙂‍↕️", "🪾", "🐦‍🔥", "🪏", "🫟"} {
		name, err := HistoricalReactionName(input)
		if err != nil || emojiNameToUnicode[name] != input {
			t.Errorf("supplemental emoji did not round-trip: %q, %v", input, err)
		}
	}
}

func TestHistoricalSkinToneEmoji(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"👍🏽", "👩🏾‍⚕️", "👩🏻‍🤝‍👨🏿", "thumbsup::skin-tone-4", "+1::skin-tone-4"} {
		name, err := HistoricalReactionName(input)
		if err != nil {
			t.Fatalf("HistoricalReactionName(%q): %v", input, err)
		}
		if IsValidReactionName(input) {
			if name != input {
				t.Errorf("name changed: %q -> %q", input, name)
			}
		} else if emojiNameToUnicode[name] != input {
			t.Errorf("Unicode changed: %q -> %q", input, emojiNameToUnicode[name])
		}
	}
}
