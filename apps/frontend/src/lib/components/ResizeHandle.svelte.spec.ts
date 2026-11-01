import '../../app.css';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import ResizeHandle from './ResizeHandle.svelte';

describe('ResizeHandle', () => {
  it.each([
    { edge: 'end' as const, edgeClass: 'end-0' },
    { edge: 'start' as const, edgeClass: 'start-0' }
  ])('keeps the $edge hit target inside its owning sidebar', async ({ edge, edgeClass }) => {
    await page.viewport(800, 600);
    const { container } = render(ResizeHandle, {
      width: 256,
      min: 192,
      max: 384,
      controls: 'pane',
      edge,
      onResize: vi.fn()
    });
    container.id = 'pane';

    const handle = page.getByRole('separator', { name: 'Resize' });
    await expect.element(handle).toHaveClass(edgeClass);
    await expect.element(handle).toHaveClass('w-2');
    await expect.element(handle).toHaveClass('h-full');
    await expect.element(handle).toHaveClass('pointer-events-auto');
    await expect.element(handle).toHaveAttribute('aria-orientation', 'vertical');
    await expect.element(handle).toHaveAttribute('aria-controls', container.id);
    await expect.element(handle).toHaveAttribute('tabindex', '0');
    await expect.element(handle).toHaveAttribute('aria-valuemin', '192');
    await expect.element(handle).toHaveAttribute('aria-valuemax', '384');
    await expect.element(handle).toHaveAttribute('aria-valuenow', '256');

    const wrapper = page.getByTestId('resize-handle');
    await expect.element(wrapper).toHaveClass(edgeClass);
    await expect.element(wrapper).toHaveClass('w-2');
    await expect.element(wrapper).toHaveClass('pointer-events-none');

    const line = wrapper.getByTestId('resize-handle-line');
    await expect.element(line).toHaveClass(edgeClass);
    await expect.element(line).toHaveClass('w-px');
  });

  it.each([
    { edge: 'end' as const, dir: 'ltr' },
    { edge: 'start' as const, dir: 'ltr' },
    { edge: 'end' as const, dir: 'rtl' },
    { edge: 'start' as const, dir: 'rtl' }
  ])('leaves midpoint row actions clickable at the $edge edge in $dir', async ({ edge, dir }) => {
    await page.viewport(800, 600);
    const { container } = render(ResizeHandle, {
      width: 256,
      min: 192,
      max: 384,
      controls: 'pane',
      edge,
      onResize: vi.fn()
    });
    container.id = 'pane';
    container.dir = dir;
    Object.assign(container.style, { position: 'relative', width: '256px', height: '240px' });

    const badge = document.createElement('button');
    badge.textContent = 'Notifications';
    Object.assign(badge.style, {
      position: 'absolute',
      insetInlineEnd: edge === 'end' ? '8px' : 'auto',
      insetInlineStart: edge === 'start' ? '8px' : 'auto',
      top: '108px',
      width: '24px',
      height: '24px'
    });
    const onClick = vi.fn();
    badge.onclick = onClick;
    container.append(badge);

    const handle = page.getByRole('separator', { name: 'Resize' }).element();
    const rect = container.getBoundingClientRect();
    const rightEdge = (edge === 'end') === (dir === 'ltr');
    const x = (inset: number) => (rightEdge ? rect.right - inset : rect.left + inset);

    // The midpoint must have the same hit area as the rest of the separator.
    for (const y of [rect.top + 32, rect.top + 120, rect.bottom - 32]) {
      expect(document.elementFromPoint(x(4), y)).toBe(handle);
      expect(document.elementFromPoint(x(12), y)).not.toBe(handle);
    }
    expect(document.elementFromPoint(x(20), rect.top + 120)).toBe(badge);
    await page.getByRole('button', { name: 'Notifications' }).click();
    expect(onClick).toHaveBeenCalledOnce();
  });

  it('supports resize keyboard controls', async () => {
    const onResize = vi.fn();
    const { container } = render(ResizeHandle, {
      width: 256,
      min: 192,
      max: 384,
      controls: 'pane',
      onResize
    });
    container.id = 'pane';

    const handle = container.querySelector('[role="separator"]') as HTMLElement;
    for (const key of ['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown']) {
      handle.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }));
    }

    expect(onResize).toHaveBeenNthCalledWith(1, 264);
    expect(onResize).toHaveBeenNthCalledWith(2, 248);
    expect(onResize).toHaveBeenNthCalledWith(3, 288);
    expect(onResize).toHaveBeenNthCalledWith(4, 224);
  });

  it('finishes a drag when pointerup reaches the window outside the handle', () => {
    const onResize = vi.fn();
    const { container } = render(ResizeHandle, {
      width: 256,
      min: 192,
      max: 384,
      controls: 'pane',
      onResize
    });
    container.id = 'pane';

    const handle = container.querySelector('[role="separator"]') as HTMLElement;
    vi.spyOn(handle, 'setPointerCapture').mockImplementation(() => undefined);
    vi.spyOn(handle, 'hasPointerCapture').mockReturnValue(true);
    const releasePointerCapture = vi
      .spyOn(handle, 'releasePointerCapture')
      .mockImplementation(() => undefined);

    handle.dispatchEvent(
      new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerId: 7, clientX: 100 })
    );
    window.dispatchEvent(
      new PointerEvent('pointermove', { bubbles: true, buttons: 1, pointerId: 7, clientX: 140 })
    );
    window.dispatchEvent(
      new PointerEvent('pointerup', { bubbles: true, button: 0, pointerId: 7, clientX: 140 })
    );

    expect(onResize).toHaveBeenCalledWith(296);
    expect(document.body.dataset.resizingSidebar).toBeUndefined();
    expect(releasePointerCapture).toHaveBeenCalledWith(7);
  });
});
