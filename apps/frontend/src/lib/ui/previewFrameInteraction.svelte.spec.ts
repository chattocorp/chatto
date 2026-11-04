import '../../app.css';
import { afterEach, expect, it, vi } from 'vitest';
import { cdp, page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import PreviewFrameInteractionHarness from './PreviewFrameInteractionHarness.svelte';

afterEach(async () => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
  await page.viewport(1280, 720);
});

async function holdContent(container: HTMLElement) {
  const content = container.querySelector<HTMLButtonElement>('.preview-frame-body button')!;
  content.focus();
  content.dispatchEvent(
    new PointerEvent('pointerdown', {
      bubbles: true,
      pointerType: 'touch',
      isPrimary: true,
      pointerId: 1
    })
  );
  await expect.poll(() => document.querySelector('dialog[open]')).not.toBeNull();
  content.dispatchEvent(
    new PointerEvent('pointerup', {
      bubbles: true,
      pointerType: 'touch',
      pointerId: 1
    })
  );
  content.click(); // The synthesized release must not open the viewer.
  return content;
}

it('reveals the desktop rim on hover and keyboard focus without covering content', async () => {
  const view = render(PreviewFrameInteractionHarness);
  const frame = view.container.querySelector<HTMLElement>('.preview-action-frame')!;
  const rail = view.container.querySelector<HTMLElement>('.preview-frame-actions')!;
  const content = view.container.querySelector<HTMLElement>('.preview-frame-body')!;
  const pause = view.container.querySelector<HTMLButtonElement>('[aria-label="Pause animation"]')!;
  const outside = view.getByRole('button', { name: 'Outside preview' });
  const hover = matchMedia('(any-hover: hover) and (any-pointer: fine)').matches;
  await outside.click();
  await expect.poll(() => rail.getBoundingClientRect().width).toBe(hover ? 0 : 32);
  if (hover) {
    expect(getComputedStyle(rail).pointerEvents).toBe('none');
    await userEvent.hover(frame);
    await expect.poll(() => rail.getBoundingClientRect().width).toBe(32);
    await userEvent.hover(outside);
    await expect.poll(() => rail.getBoundingClientRect().width).toBe(0);
  }
  pause.focus();
  await expect.poll(() => rail.getBoundingClientRect().width).toBe(32);
  expect(content.getBoundingClientRect().right).toBeLessThanOrEqual(
    pause.getBoundingClientRect().left
  );
  await userEvent.keyboard('{Enter}');
  await expect
    .element(view.getByRole('button', { name: 'Pause animation' }))
    .toHaveAttribute('aria-pressed', 'true');
});

it('uses the same actions in a touch sheet, closes on activation, and restores focus', async () => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
  await page.viewport(375, 600);
  const onaction = vi.fn();
  const oncontent = vi.fn();
  const view = render(PreviewFrameInteractionHarness, { onaction, oncontent });
  const content = view.getByRole('button', { name: 'Open preview', exact: true });
  expect(view.container.querySelector('[aria-label="More actions"]')).toBeNull();
  expect(view.container.querySelector('.preview-frame-actions')).toBeNull();
  const frameBounds = view.container
    .querySelector('.preview-action-frame')!
    .getBoundingClientRect();
  const bodyBounds = view.container.querySelector('.preview-frame-body')!.getBoundingClientRect();
  expect(frameBounds.right - bodyBounds.right).toBe(5);
  await holdContent(view.container);
  const pause = page.getByRole('button', { name: 'Pause animation', exact: true });
  await expect.element(pause).toBeVisible();
  await expect.element(pause).toHaveAttribute('aria-pressed', 'false');
  await expect.element(pause).toHaveAttribute('aria-describedby', 'preview-description');
  await expect.element(page.getByRole('button', { name: 'Unavailable action' })).toBeDisabled();
  expect(oncontent).not.toHaveBeenCalled();
  await pause.click();
  await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
  expect(onaction).toHaveBeenCalledOnce();
  await expect.element(content).toHaveFocus();
  await holdContent(view.container);
  await expect
    .element(page.getByRole('button', { name: 'Pause animation', exact: true }))
    .toHaveAttribute('aria-pressed', 'true');
  await userEvent.keyboard('{Escape}');
  await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
  await expect.element(content).toHaveFocus();
});

it('discards an open sheet when the primary input changes', async () => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
  const view = render(PreviewFrameInteractionHarness);
  await holdContent(view.container);
  await expect.poll(() => document.querySelector('dialog[open]')).not.toBeNull();
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
  await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
  expect(document.querySelector('dialog[open]')).toBeNull();
  expect(view.container.querySelector('[aria-label="More actions"]')).toBeNull();
});

it('disables the reveal transition for reduced motion', () => {
  const root = document.documentElement;
  const previous = root.dataset.motion;
  const view = render(PreviewFrameInteractionHarness);
  const rail = view.container.querySelector<HTMLElement>('.preview-frame-actions')!;
  try {
    root.dataset.motion = 'full';
    expect(parseFloat(getComputedStyle(rail).transitionDuration)).toBeGreaterThan(0);
    root.dataset.motion = 'reduced';
    expect(parseFloat(getComputedStyle(rail).transitionDuration)).toBe(0);
  } finally {
    if (previous === undefined) delete root.dataset.motion;
    else root.dataset.motion = previous;
  }
});

it('uses one sheet for a held touch and suppresses the release click', async () => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
  const onaction = vi.fn();
  const view = render(PreviewFrameInteractionHarness, { onaction });
  const trigger = view.container.querySelector<HTMLButtonElement>('.preview-frame-body button')!;
  trigger.dispatchEvent(
    new PointerEvent('pointerdown', {
      bubbles: true,
      pointerType: 'touch',
      isPrimary: true,
      pointerId: 1
    })
  );
  await expect.poll(() => document.querySelectorAll('dialog[open]').length).toBe(1);
  const nativeMenu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
  trigger.dispatchEvent(nativeMenu);
  expect(nativeMenu.defaultPrevented).toBe(true);
  trigger.dispatchEvent(
    new PointerEvent('pointerup', { bubbles: true, pointerType: 'touch', pointerId: 1 })
  );
  const action = document.querySelector<HTMLButtonElement>('dialog .menu-entry')!;
  action.click();
  expect(onaction).not.toHaveBeenCalled();
  expect(document.querySelectorAll('dialog[open]')).toHaveLength(1);
});

it('keeps ordinary taps and scrolling native on touch previews', async () => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
  const oncontent = vi.fn();
  const view = render(PreviewFrameInteractionHarness, { oncontent });
  const content = view.container.querySelector<HTMLButtonElement>('.preview-frame-body button')!;
  await view.getByRole('button', { name: 'Open preview', exact: true }).click();
  expect(oncontent).toHaveBeenCalledOnce();
  content.dispatchEvent(
    new PointerEvent('pointerdown', {
      bubbles: true,
      pointerType: 'touch',
      isPrimary: true,
      pointerId: 1,
      clientX: 20,
      clientY: 20
    })
  );
  content.dispatchEvent(
    new PointerEvent('pointermove', {
      bubbles: true,
      pointerType: 'touch',
      pointerId: 1,
      clientX: 20,
      clientY: 40
    })
  );
  await new Promise((resolve) => setTimeout(resolve, 600));
  expect(document.querySelector('dialog[open]')).toBeNull();
  content.dispatchEvent(
    new PointerEvent('pointerup', { bubbles: true, pointerType: 'touch', pointerId: 1 })
  );
});

it('claims an early native touch menu before descendant and message handlers', async () => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
  const view = render(PreviewFrameInteractionHarness);
  const content = view.container.querySelector<HTMLButtonElement>('.preview-frame-body button')!;
  const childMenu = vi.fn();
  const parentMenu = vi.fn();
  content.addEventListener('contextmenu', childMenu);
  view.container.addEventListener('contextmenu', parentMenu);
  content.dispatchEvent(
    new PointerEvent('pointerdown', {
      bubbles: true,
      pointerType: 'touch',
      isPrimary: true,
      pointerId: 1
    })
  );
  const nativeMenu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
  content.dispatchEvent(nativeMenu);
  await expect.poll(() => document.querySelectorAll('dialog[open]').length).toBe(1);
  expect(nativeMenu.defaultPrevented).toBe(true);
  expect(childMenu).not.toHaveBeenCalled();
  expect(parentMenu).not.toHaveBeenCalled();
  content.dispatchEvent(
    new PointerEvent('pointerup', { bubbles: true, pointerType: 'touch', pointerId: 1 })
  );
});
