import { expoOut } from 'svelte/easing';
import { userPreferences } from '$lib/state/userPreferences.svelte';

export const COMPACT_MOTION_DURATION_MS = 180;
const PANE_MOTION_DURATION_MS = 240;

/** Shared exponential transition timing for spatial UI movement. */
export function expoOutTransition(duration = PANE_MOTION_DURATION_MS) {
  return {
    duration: userPreferences.reducedMotion ? 0 : duration,
    easing: expoOut
  };
}
