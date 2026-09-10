import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { initialPageReveal } from './initialPageReveal';

let root: HTMLDivElement;
let loading: HTMLDivElement;
let motion: EventTarget & { matches: boolean };
let cleanup: void | (() => void);

function appendLoadingShell() {
  const shell = document.createElement('div');
  shell.id = 'app-loading';
  document.body.append(shell);
  return shell;
}

beforeEach(() => {
  motion = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
  loading = appendLoadingShell();
  root = document.createElement('div');
  root.innerHTML = `<header>App</header><main><aside>Servers</aside><section data-page-reveal><h1 data-page-reveal>Title</h1><form data-page-reveal><input aria-label="Name"></form></section></main>`;
  document.body.append(root);
});

afterEach(() => {
  cleanup?.();
  cleanup = undefined;
  root.remove();
  vi.restoreAllMocks();
});

it('reveals separate sections without scaling nested sections twice', () => {
  cleanup = initialPageReveal(root);
  const animations = root.getAnimations({ subtree: true });
  const targets = animations.map((animation) => (animation.effect as KeyframeEffect).target);
  expect(targets).toEqual(Array.from(root.querySelectorAll('header, aside, h1, form')));
  expect(animations.map((animation) => animation.effect?.getTiming().delay)).toEqual([
    0, 90, 180, 270
  ]);
  expect(root.querySelector('section')?.getAnimations()).toHaveLength(0);
});

it('cross-fades and removes the startup shell', async () => {
  cleanup = initialPageReveal(root);

  expect(loading.isConnected).toBe(true);
  const [animation] = loading.getAnimations();
  expect(animation.effect?.getTiming()).toMatchObject({ duration: 540, easing: 'ease-out' });
  expect((animation.effect as KeyframeEffect).getKeyframes()).toMatchObject([
    { opacity: '1', transform: 'scale(1)' },
    { opacity: '0', transform: 'scale(1.02)' }
  ]);

  animation.finish();
  await vi.waitFor(() => expect(loading.isConnected).toBe(false));
});

it('does not animate content inserted by navigation or an asynchronous load', () => {
  cleanup = initialPageReveal(root);
  root.querySelector('main')!.innerHTML = '<section data-page-reveal>Next page</section>';
  expect(root.querySelector('section')?.getAnimations({ subtree: true })).toHaveLength(0);
});

it('keeps automatic form focus but reveals all controls before keyboard input', () => {
  root.querySelector('input')!.focus();
  cleanup = initialPageReveal(root);
  expect(root.getAnimations({ subtree: true }).length).toBeGreaterThan(0);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }));
  expect(root.getAnimations({ subtree: true })).toHaveLength(0);
  expect(loading.isConnected).toBe(false);
});

it('skips reduced motion and cancels when the preference changes', () => {
  motion.matches = true;
  cleanup = initialPageReveal(root);
  expect(root.getAnimations({ subtree: true })).toHaveLength(0);
  expect(loading.isConnected).toBe(false);
  motion.matches = false;
  loading = appendLoadingShell();
  cleanup = initialPageReveal(root);
  expect(root.getAnimations({ subtree: true }).length).toBeGreaterThan(0);
  motion.matches = true;
  motion.dispatchEvent(new Event('change'));
  expect(root.getAnimations({ subtree: true })).toHaveLength(0);
  expect(loading.isConnected).toBe(false);
});

it('removes animation effects after completion and on unmount', () => {
  cleanup = initialPageReveal(root);
  for (const animation of root.getAnimations({ subtree: true })) animation.finish();
  expect(getComputedStyle(root.querySelector('form')!).scale).toBe('none');
  cleanup?.();
  expect(root.getAnimations({ subtree: true })).toHaveLength(0);
});

it('reveals controls before pointer interaction', () => {
  cleanup = initialPageReveal(root);
  root.querySelector('input')!.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }));
  expect(root.getAnimations({ subtree: true })).toHaveLength(0);
  expect(loading.isConnected).toBe(false);
});

it('removes the startup shell and animations on unmount', () => {
  cleanup = initialPageReveal(root);
  cleanup?.();
  cleanup = undefined;

  expect(loading.isConnected).toBe(false);
  expect(root.getAnimations({ subtree: true })).toHaveLength(0);
});
