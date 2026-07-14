import '../../app.css';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import ResizeHandle from './ResizeHandle.svelte';

describe('ResizeHandle', () => {
  it.each([
    { edge: 'right' as const, edgeClass: 'right-0' },
    { edge: 'left' as const, edgeClass: 'left-0' }
  ])('keeps the $edge hit target inside its owning sidebar', async ({ edge, edgeClass }) => {
    await page.viewport(800, 600);
    render(ResizeHandle, {
      width: 256,
      min: 192,
      max: 384,
      edge,
      onResize: vi.fn()
    });

    const handle = page.getByRole('button', { name: 'Resize' });
    await expect.element(handle).toHaveClass(edgeClass);
    await expect.element(handle).toHaveClass('w-2');

    const line = handle.getByTestId('resize-handle-line');
    await expect.element(line).toHaveClass(edgeClass);
  });
});
