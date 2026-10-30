import '../../app.css';
import { cdp, page, userEvent } from 'vitest/browser';
import { afterEach, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import Frame from './Frame.svelte';

afterEach(async () => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
  await page.viewport(1280, 720);
});

it('updates control density when the primary input changes without remounting', async () => {
  const { container } = render(Frame, {
    children: testSnippet('<button class="menu-entry">Command</button>')
  });
  const command = container.querySelector<HTMLElement>('.menu-entry')!;
  for (const touch of [false, true, false]) {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
    await expect.poll(() => command.getBoundingClientRect().height).toBe(touch ? 44 : 32);
  }
});

it.each([false, true])(
  'keeps menu targets and hover controls accessible with touch=%s',
  async (touch) => {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
    const { container } = render(Frame, {
      children: testSnippet(
        '<div class="group/preview relative h-40 w-64"><button class="menu-entry">Command</button><div class="preview-toolbar"><button aria-label="Open attachment">Open</button></div></div>'
      )
    });
    const command = container.querySelector<HTMLElement>('.menu-entry')!;
    const action = container.querySelector<HTMLElement>('.preview-toolbar')!;
    for (const width of [320, 767, 768, 1280]) {
      await page.viewport(width, 800);
      expect(command.getBoundingClientRect().height).toBeGreaterThanOrEqual(touch ? 44 : 32);
      await userEvent.hover(command);
      await expect.poll(() => getComputedStyle(action).opacity).toBe('1');
      await userEvent.unhover(command);
    }
  }
);

it.each([false, true])(
  'keeps frame and timeline presentation in sync with touch=%s',
  async (touch) => {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
    const { container } = render(Frame, {
      children: testSnippet('<div class="message-row">Message content</div>')
    });
    const frame = container.firstElementChild!;
    const row = frame.firstElementChild!;
    for (const width of [375, 500, 767, 768, 1024, 375]) {
      await page.viewport(width, 800);
      const mobile = touch && width < 768;
      expect(getComputedStyle(frame).borderRadius).toBe(mobile ? '0px' : '16px');
      expect(getComputedStyle(frame).overflow).toBe(mobile ? 'visible' : 'hidden');
      expect(getComputedStyle(row).marginLeft).toBe(mobile ? '0px' : '8px');
      expect(
        getComputedStyle(document.documentElement).getPropertyValue('--app-header-height').trim()
      ).toBe(mobile ? '3.5rem' : '2.75rem');
    }
  }
);

it.each([false, true])(
  'keeps embed controls reachable with touch=%s at any width',
  async (touch) => {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
    expect(matchMedia('(any-pointer: coarse)').matches).toBe(touch);
    // Disabling touch does not imply hover support (for example, in headless CI).
    const hoverControls =
      !touch && matchMedia('(any-hover: hover) and (any-pointer: fine)').matches;
    const idleOpacity = hoverControls ? '0' : '1';
    const { container } = render(Frame, {
      children: testSnippet(
        '<div><div class="group/preview relative h-40 w-64"><div class="preview-toolbar"><button aria-label="Open attachment">Open</button></div></div><button data-testid="outside-preview">Outside preview</button></div>'
      )
    });
    const toolbar = container.querySelector<HTMLElement>('.preview-toolbar')!;
    const button = toolbar.querySelector<HTMLButtonElement>('button')!;
    const outside = container.querySelector<HTMLElement>('[data-testid="outside-preview"]')!;
    // Start focused so the test also proves it clears keyboard reveal state.
    button.focus();
    await expect.poll(() => getComputedStyle(toolbar).opacity).toBe('1');
    for (const width of [375, 1024]) {
      await page.viewport(width, 800);
      // Both pointer and keyboard focus can reveal the preview controls.
      await userEvent.hover(outside);
      outside.focus();
      expect(button.parentElement!.matches(':hover')).toBe(false);
      expect(button.parentElement!.matches(':focus-within')).toBe(false);
      await expect.poll(() => getComputedStyle(toolbar).opacity).toBe(idleOpacity);
      expect(getComputedStyle(toolbar).pointerEvents).toBe('none');
      expect(getComputedStyle(button).pointerEvents).toBe(hoverControls ? 'none' : 'auto');
      button.focus();
      await expect.poll(() => getComputedStyle(toolbar).opacity).toBe('1');
      expect(getComputedStyle(toolbar).pointerEvents).toBe('none');
      expect(getComputedStyle(button).pointerEvents).toBe('auto');
      outside.focus();
      await expect.poll(() => getComputedStyle(toolbar).opacity).toBe(idleOpacity);
    }
  }
);
