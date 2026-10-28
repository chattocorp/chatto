/** Browser-local default for new emoji selections, shared across servers. */
import { Codecs, globalSlot } from '@chatto/client/storage/slot';
import { emojiTones, type EmojiTone } from '$lib/emojiTones';

const slot = globalSlot<EmojiTone>(
  'emojiSkinTone',
  0,
  Codecs.json((value): value is EmojiTone => emojiTones.some((option) => option.tone === value))
);

/** Owns the reactive preference and saves explicit changes without an effect. */
export class EmojiPreferences {
  #tone = $state<EmojiTone>(slot.get());

  get tone(): EmojiTone {
    return this.#tone;
  }

  set tone(value: EmojiTone) {
    this.#tone = value;
    slot.set(value);
  }
}

/** Shared by all picker and autocomplete instances in the browser app. */
export const emojiPreferences = new EmojiPreferences();
