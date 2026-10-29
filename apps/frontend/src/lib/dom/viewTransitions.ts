import { tick } from 'svelte';
import { motionPreference } from './motionPreference';

// One document can show one transition. Keep its lifetime here so navigation
// and appearance controls cannot leave each other's snapshot on screen.
let active: ViewTransition | undefined;
let themeRevision = 0;

function motionAvailable(): boolean {
  return (
    typeof document.startViewTransition === 'function' &&
    !motionPreference().matches &&
    document.visibilityState === 'visible'
  );
}

function track(transition: ViewTransition): void {
  active = transition;
  const reducedMotion = motionPreference();
  const stop = () => {
    if (reducedMotion.matches) transition.skipTransition();
  };
  reducedMotion.addEventListener('change', stop);
  // Skipped snapshots reject ready; the DOM update must still complete.
  void transition.ready.catch(() => {});
  void transition.finished
    .catch(() => {})
    .finally(() => {
      reducedMotion.removeEventListener('change', stop);
      if (active !== transition) return;
      active = undefined;
      delete document.documentElement.dataset.viewMotion;
      document.documentElement.style.removeProperty('--theme-x');
      document.documentElement.style.removeProperty('--theme-y');
      document.documentElement.style.removeProperty('--theme-radius');
    });
}

/** Navigation stays immediate and clears any theme snapshot still on screen. */
export function stopViewTransition(): void {
  active?.skipTransition();
}

/** Reveal an explicit theme choice from its control; keyboard input uses its centre. */
export function transitionTheme(event: MouseEvent, update: () => void): void {
  const revision = ++themeRevision;
  if (active) {
    active.skipTransition();
    // A skipped transition still runs its update. Apply the last choice after
    // that update, so rapid clicks cannot restore an earlier theme.
    void active.updateCallbackDone
      .catch(() => {})
      .then(() => {
        if (revision === themeRevision) update();
      });
    return;
  }
  if (!motionAvailable()) {
    update();
    return;
  }
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();
  const x = event.detail ? event.clientX : bounds.left + bounds.width / 2;
  const y = event.detail ? event.clientY : bounds.top + bounds.height / 2;
  const radius = Math.hypot(Math.max(x, innerWidth - x), Math.max(y, innerHeight - y));
  const root = document.documentElement;
  root.dataset.viewMotion = 'theme';
  root.style.setProperty('--theme-x', `${x}px`);
  root.style.setProperty('--theme-y', `${y}px`);
  root.style.setProperty('--theme-radius', `${radius}px`);
  track(
    document.startViewTransition(async () => {
      if (revision === themeRevision) update();
      await tick();
    })
  );
}
