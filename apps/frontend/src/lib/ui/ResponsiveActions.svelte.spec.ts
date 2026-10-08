import { afterEach, expect, it, vi } from 'vitest';
import { ResponsiveActions } from './ResponsiveActions.svelte';

const cleanups: (() => void)[] = [];

afterEach(() => {
  for (const cleanup of cleanups.splice(0).reverse()) cleanup();
});

function mount(width: string, breakpointRem = 20) {
  const scope = document.createElement('section');
  const element = document.createElement('div');
  element.style.cssText = `width: ${width}; box-sizing: border-box`;
  const inline = document.createElement('div');
  const action = document.createElement('button');
  const trigger = document.createElement('button');
  const fallback = document.createElement('button');
  inline.append(action);
  element.append(inline, trigger);
  scope.append(element, fallback);
  document.body.append(scope);
  let open = false;
  const dismissMenu = vi.fn(() => {
    open = false;
  });
  const state = new ResponsiveActions({
    breakpointRem,
    isMenuOpen: () => open,
    dismissMenu,
    focusFallback: () => fallback,
    focusScope: () => scope
  });
  cleanups.push(() => scope.remove());
  const stop = state.observe(element);
  if (stop) cleanups.push(stop);
  const stopInline = state.inline(inline);
  if (stopInline) cleanups.push(stopInline);
  state.trigger(trigger);
  return {
    state,
    scope,
    element,
    inline,
    action,
    trigger,
    fallback,
    dismissMenu,
    openMenu: () => {
      open = true;
    },
    stop
  };
}

it('uses the element border-box at the exact cutoff in a wider viewport', async () => {
  const { state, element } = mount('319px');
  expect(window.innerWidth).toBeGreaterThan(320);
  expect(state.compact).toBe(true);
  element.style.width = '320px';
  await expect.poll(() => state.compact).toBe(false);
  element.style.width = '319.5px';
  await expect.poll(() => state.compact).toBe(true);
});

it('supports a different cutoff and ignores transforms', async () => {
  const { state, element } = mount('383px', 24);
  expect(state.compact).toBe(true);
  element.style.transform = 'scale(0.5)';
  element.style.width = '384px';
  await expect.poll(() => state.compact).toBe(false);
});

it('counts padding and borders on a content-box container', async () => {
  const { state, element } = mount('296px');
  element.style.boxSizing = 'content-box';
  element.style.padding = '10px';
  element.style.border = '2px solid transparent';
  await expect.poll(() => state.compact).toBe(false);
  element.style.width = '295px';
  await expect.poll(() => state.compact).toBe(true);
});

it('dismisses an owned menu and restores trigger focus on a layout change', async () => {
  const { state, element, trigger, openMenu, dismissMenu } = mount('319px');
  openMenu();
  element.style.width = '320px';
  await expect.poll(() => state.compact).toBe(false);
  expect(dismissMenu).toHaveBeenCalledOnce();
  await expect.poll(() => document.activeElement).toBe(trigger);
});

it('moves focus from an inline action when the container becomes compact', async () => {
  const { state, element, action, trigger, dismissMenu } = mount('320px');
  action.focus();
  element.style.width = '319px';
  await expect.poll(() => state.compact).toBe(true);
  await expect.poll(() => document.activeElement).toBe(trigger);
  expect(dismissMenu).not.toHaveBeenCalled();
});

it('dismisses a removed container and uses the host focus fallback', async () => {
  const { element, fallback, openMenu, dismissMenu, stop } = mount('319px');
  openMenu();
  element.remove();
  stop?.();
  expect(dismissMenu).toHaveBeenCalledOnce();
  await expect.poll(() => document.activeElement).toBe(fallback);
});

it('preserves focus moved to a replacement control before dismissal finishes', async () => {
  const { state, fallback } = mount('319px');
  const restore = state.restoreFocus();
  fallback.focus();
  await restore;
  expect(document.activeElement).toBe(fallback);
});
