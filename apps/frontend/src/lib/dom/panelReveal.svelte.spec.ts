import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createPanelReveal } from './panelReveal';

let element: HTMLDivElement;
let cleanup: void | (() => void);
let motion: EventTarget & { matches: boolean };

beforeEach(() => {
  element = document.createElement('div');
  document.body.append(element);
  motion = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
});

afterEach(() => {
  cleanup?.();
  cleanup = undefined;
  element.remove();
  vi.restoreAllMocks();
});

it('reveals a changed panel without replacing DOM or animating initial content', () => {
  let panel = 'members';
  const reveal = createPanelReveal(() => ({ scope: 'room-one', panel }));
  element.innerHTML = '<input value="draft">';
  const input = element.firstElementChild;
  cleanup = reveal(element);
  expect(element.getAnimations()).toHaveLength(0);
  panel = 'files';
  cleanup = reveal(element);
  expect(element.getAnimations()).toHaveLength(1);
  expect(element.firstElementChild).toBe(input);
  cleanup?.();
  cleanup = reveal(element);
  expect(element.getAnimations()).toHaveLength(0);
});

it('does not animate content when switching rooms', () => {
  let scope = 'room-one';
  let panel = 'members';
  const reveal = createPanelReveal(() => ({ scope, panel }));
  reveal(element);
  scope = 'room-two';
  panel = 'files';
  cleanup = reveal(element);
  expect(element.getAnimations()).toHaveLength(0);
});

it('respects reduced motion and stops an animation if the preference changes', () => {
  let panel = 'members';
  const reveal = createPanelReveal(() => ({ scope: 'room-one', panel }));
  reveal(element);
  motion.matches = true;
  panel = 'files';
  cleanup = reveal(element);
  expect(element.getAnimations()).toHaveLength(0);
  motion.matches = false;
  panel = 'search';
  cleanup = reveal(element);
  expect(element.getAnimations()).toHaveLength(1);
  motion.matches = true;
  motion.dispatchEvent(new Event('change'));
  expect(element.getAnimations()).toHaveLength(0);
});

it('mirrors the small panel movement in right-to-left layouts', () => {
  element.dir = 'rtl';
  let panel = 'members';
  const reveal = createPanelReveal(() => ({ scope: 'room-one', panel }));
  reveal(element);
  panel = 'profile:one';
  cleanup = reveal(element);
  const frames = (element.getAnimations()[0].effect as KeyframeEffect).getKeyframes();
  expect(frames[0].transform).toBe('translateX(-8px)');
});
