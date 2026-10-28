import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import {
  applyAnimationPreference,
  motionPreference,
  watchMotionPreference
} from './motionPreference';
import { fadePageContent } from './pageFade';
import '../../app.css';

let system: EventTarget & { matches: boolean };
let stopWatch: (() => void) | void;
let stopFade: (() => void) | undefined;
let pane: HTMLDivElement;
beforeEach(() => {
  system = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(system as MediaQueryList);
  pane = document.createElement('div');
  pane.dataset.navigationContent = '';
  pane.className = 'copy-feedback';
  pane.dataset.copied = 'true';
  pane.innerHTML = '<span data-copy-check>Copied</span>';
  document.body.append(pane);
  applyAnimationPreference('automatic');
  stopWatch = watchMotionPreference(document.documentElement);
});
afterEach(() => {
  stopFade?.();
  stopWatch?.();
  pane.remove();
  applyAnimationPreference('automatic');
  vi.restoreAllMocks();
});

it('follows OS changes in automatic mode for CSS and JavaScript', () => {
  const query = motionPreference();
  expect(query.matches).toBe(false);
  system.matches = true;
  system.dispatchEvent(new Event('change'));
  expect(query.matches).toBe(true);
  expect(document.documentElement.dataset.motion).toBe('reduced');
  expect(getComputedStyle(pane.firstElementChild!).animationName).toBe('none');
  stopFade = fadePageContent();
  expect(pane.getAnimations()).toHaveLength(0);
});

it('overrides reduced motion when enabled and stops active motion when disabled', () => {
  system.matches = true;
  applyAnimationPreference('enabled');
  expect(motionPreference().matches).toBe(false);
  expect(getComputedStyle(pane.firstElementChild!).animationName).toBe('copy-confirmation');
  stopFade = fadePageContent();
  expect(pane.getAnimations()).toHaveLength(1);
  applyAnimationPreference('disabled');
  expect(pane.getAnimations()).toHaveLength(0);
  expect(getComputedStyle(pane.firstElementChild!).animationName).toBe('none');
  system.matches = false;
  system.dispatchEvent(new Event('change'));
  expect(motionPreference().matches).toBe(true);
});

it('releases app and OS listeners when the owner detaches', () => {
  const query = motionPreference();
  const changed = vi.fn();
  query.addEventListener('change', changed);
  applyAnimationPreference('disabled');
  expect(changed).toHaveBeenCalledOnce();
  query.removeEventListener('change', changed);
  applyAnimationPreference('enabled');
  system.dispatchEvent(new Event('change'));
  expect(changed).toHaveBeenCalledOnce();
});
