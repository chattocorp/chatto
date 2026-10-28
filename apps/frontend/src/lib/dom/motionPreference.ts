import type { Attachment } from 'svelte/attachments';

/** Automatic follows the OS; explicit choices override it for this browser. */
export type AnimationPreference = 'automatic' | 'enabled' | 'disabled';
const preferenceChanged = 'chatto:animation-preference';

/** Reject unknown stored values without disabling the system preference. */
export function isAnimationPreference(value: unknown): value is AnimationPreference {
  return value === 'automatic' || value === 'enabled' || value === 'disabled';
}

/**
 * A motion query scoped to its caller. Listeners cover both OS and app changes;
 * callers must remove them when their animation or mounted surface ends.
 */
export function motionPreference() {
  const system = window.matchMedia('(prefers-reduced-motion: reduce)');
  return {
    get matches(): boolean {
      const choice = document.documentElement.dataset.animations;
      return choice === 'disabled' || (choice !== 'enabled' && system.matches);
    },
    addEventListener(_type: 'change', listener: () => void) {
      system.addEventListener('change', listener);
      document.addEventListener(preferenceChanged, listener);
    },
    removeEventListener(_type: 'change', listener: () => void) {
      system.removeEventListener('change', listener);
      document.removeEventListener(preferenceChanged, listener);
    }
  };
}

/** Apply CSS state immediately, including before the app root has mounted. */
export function applyAnimationPreference(value: AnimationPreference): void {
  if (typeof document === 'undefined') return;
  document.documentElement.dataset.animations = isAnimationPreference(value) ? value : 'automatic';
  document.documentElement.dataset.motion = motionPreference().matches ? 'reduced' : 'full';
  document.dispatchEvent(new Event(preferenceChanged));
}

/** Keep CSS aligned with OS changes for the lifetime of the application root. */
export const watchMotionPreference: Attachment = () => {
  const motion = motionPreference();
  const sync = () => {
    document.documentElement.dataset.motion = motion.matches ? 'reduced' : 'full';
  };
  sync();
  motion.addEventListener('change', sync);
  return () => motion.removeEventListener('change', sync);
};
