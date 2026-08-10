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
    render(ResizeHandle, {
      width: 256,
      min: 192,
      max: 384,
      edge,
      onResize: vi.fn()
    });

    const handle = page.getByRole('slider', { name: 'Resize' });
    await expect.element(handle).toHaveClass(edgeClass);
    await expect.element(handle).toHaveClass('w-6');
    await expect.element(handle).toHaveClass('h-6');
    await expect.element(handle).toHaveClass('pointer-events-auto');
    await expect.element(handle).toHaveAttribute('aria-orientation', 'vertical');
    await expect.element(handle).toHaveAttribute('aria-valuemin', '192');
    await expect.element(handle).toHaveAttribute('aria-valuemax', '384');
    await expect.element(handle).toHaveAttribute('aria-valuenow', '256');

    const wrapper = page.getByTestId('resize-handle');
    await expect.element(wrapper).toHaveClass(edgeClass);
    await expect.element(wrapper).toHaveClass('w-6');
    await expect.element(wrapper).toHaveClass('pointer-events-none');

    const dragStrip = wrapper.getByTestId('resize-handle-drag-strip');
    await expect.element(dragStrip).toHaveClass(edgeClass);
    await expect.element(dragStrip).toHaveClass('w-2');
    await expect.element(dragStrip).toHaveClass('h-full');

    const line = wrapper.getByTestId('resize-handle-line');
    await expect.element(line).toHaveClass(edgeClass);
    await expect.element(line).toHaveClass('w-px');
  });

  it('supports vertical slider keyboard controls', async () => {
    const onResize = vi.fn();
    const { container } = render(ResizeHandle, {
      width: 256,
      min: 192,
      max: 384,
      onResize
    });

    const handle = container.querySelector('[role="slider"]') as HTMLElement;
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
      onResize
    });

    const handle = container.querySelector('[role="slider"]') as HTMLElement;
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
