/**
 * Device-local "Not now" state of Chatto's invitation to enable push
 * notifications. Browsers stop showing a site's permission prompt for a while
 * after the user dismisses it several times, so Chatto asks only after the
 * user chooses Enable, and waits after Not now or a dismissed prompt.
 */

import { Codecs, globalSlot } from '@chatto/client/storage/slot';

/** How long Chatto waits after Not now or a dismissed browser prompt. */
export const PUSH_PROMPT_SNOOZE_MS = 14 * 24 * 60 * 60 * 1000;

const snoozedUntil = globalSlot('pushPromptSnoozedUntil', 0, Codecs.number({ min: 0 }));

/** Whether the invitation waits after an earlier Not now on this device. */
export function isPushPromptSnoozed(now = Date.now()): boolean {
  return now < snoozedUntil.get();
}

/** Hides the invitation on this device for `PUSH_PROMPT_SNOOZE_MS`. */
export function snoozePushPrompt(now = Date.now()): void {
  snoozedUntil.set(now + PUSH_PROMPT_SNOOZE_MS);
}
