import { beforeEach, describe, expect, it } from 'vitest';
import { EmojiPreferences } from './emojiPreferences.svelte';

describe('EmojiPreferences', () => {
  beforeEach(() => localStorage.clear());

  it('defaults to yellow and restores a saved choice in a fresh instance', () => {
    const preferences = new EmojiPreferences();
    expect(preferences.tone).toBe(0);
    preferences.tone = 4;
    expect(new EmojiPreferences().tone).toBe(4);
    preferences.tone = 0;
    expect(new EmojiPreferences().tone).toBe(0);
  });

  it.each(['null', '7', '1', '2.5', '"4"', 'broken'])(
    'ignores corrupt or unsupported values: %s',
    (value) => {
      localStorage.setItem('chatto:emojiSkinTone', value);
      expect(new EmojiPreferences().tone).toBe(0);
    }
  );
});
