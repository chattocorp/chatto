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
        '<div class="group/preview relative h-40 w-64"><button class="menu-entry">Command</button><div class="preview-actions"><button aria-label="Open attachment">Open</button></div></div>'
      )
    });
    const command = container.querySelector<HTMLElement>('.menu-entry')!;
    const action = container.querySelector<HTMLElement>('.preview-actions')!;
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
  'shows preview actions on keyboard focus, and always for touch, with touch=%s',
  async (touch) => {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
    expect(matchMedia('(any-pointer: coarse)').matches).toBe(touch);
    // A mouse hides the actions at rest; touch input always shows them. CI's
    // headless browser reports no hover, so there they always show.
    const hiddenAtRest = !touch && matchMedia('(any-hover: hover) and (any-pointer: fine)').matches;
    const { container } = render(Frame, {
      children: testSnippet(
        '<div><div class="group/preview relative h-40 w-64"><div class="preview-actions"><button aria-label="Open attachment">Open</button></div></div><button data-testid="outside-preview">Outside preview</button></div>'
      )
    });
    const actions = container.querySelector<HTMLElement>('.preview-actions')!;
    const button = actions.querySelector<HTMLButtonElement>('button')!;
    const outside = container.querySelector<HTMLElement>('[data-testid="outside-preview"]')!;
    for (const width of [375, 1024]) {
      await page.viewport(width, 800);
      await userEvent.hover(outside);
      outside.focus();
      await expect.poll(() => getComputedStyle(actions).opacity).toBe(hiddenAtRest ? '0' : '1');
      // Hidden actions stay reachable for pointer and keyboard input.
      expect(getComputedStyle(button).pointerEvents).toBe('auto');
      await userEvent.keyboard('{Shift>}{Tab}{/Shift}');
      expect(document.activeElement).toBe(button);
      await expect.poll(() => getComputedStyle(actions).opacity).toBe('1');
    }
  }
);
