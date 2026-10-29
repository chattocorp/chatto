/** Shared presentation choices for the picker and composer qualifier. */
import { resolveEmoji } from '@chatto/client/messaging/emojiShortcodes';

export const emojiTones = [
  { tone: 0, label: 'tone_default' },
  { tone: 2, label: 'tone_light' },
  { tone: 3, label: 'tone_medium_light' },
  { tone: 4, label: 'tone_medium' },
  { tone: 5, label: 'tone_medium_dark' },
  { tone: 6, label: 'tone_dark' }
] as const;

/** Tone number saved by the bundled frontend; zero means default yellow. */
export type EmojiTone = (typeof emojiTones)[number]['tone'];

/** Return the base name when the shared catalogue provides tone variants. */
export function emojiToneBase(name: string): string | null {
  const base = name.split('::')[0];
  return resolveEmoji(`${base}::skin-tone-2`).kind === 'unicode' ? base : null;
}
