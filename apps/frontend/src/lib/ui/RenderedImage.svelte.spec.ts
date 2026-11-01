import '../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { on } from 'svelte/events';
import RenderedImage from './RenderedImage.svelte';
import RenderedImageViewerModal from '../../routes/chat/modals/RenderedImageViewerModal.svelte';

const { pushState } = vi.hoisted(() => ({ pushState: vi.fn() }));
vi.mock('$app/navigation', () => ({ pushState, goto: vi.fn() }));

const src = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="400" height="300"/>')}`;

describe('RenderedImage', () => {
  it('leaves scrolling alone and opens the shared viewer through shallow routing', async () => {
    const image = new Image();
    image.src = src;
    image.alt = 'Diagram';
    const view = render(RenderedImage, { image });
    view.container.classList.add('prose');
    view.container.style.width = '400px';
    try {
      await expect.poll(() => image.naturalWidth).toBe(400);
      expect(getComputedStyle(image).touchAction).toBe('auto');
      expect(view.container.querySelector('[aria-label="Zoom in"]')).toBeNull();
      const wheel = new WheelEvent('wheel', {
        deltaY: 100,
        ctrlKey: true,
        bubbles: true,
        cancelable: true
      });
      image.dispatchEvent(wheel);
      expect(wheel.defaultPrevented).toBe(false);
      const parentClick = vi.fn();
      // Observe after delegated Svelte handlers, as the message row does.
      const stopObserving = on(view.container, 'click', parentClick);
      const opener = view.container.querySelector('button')!;
      await expect.element(opener).toHaveAccessibleDescription('Diagram');
      expect(opener.textContent?.trim()).toBe('');
      await userEvent.click(image);
      expect(parentClick).not.toHaveBeenCalled();
      expect(pushState).toHaveBeenCalledWith('', {
        modal: { type: 'renderedImageViewer', src, alt: 'Diagram' }
      });
      expect(document.querySelector('dialog')).toBeNull();
      stopObserving();
    } finally {
      await view.unmount();
    }
  });

  it('shows the snapshot with zoom and download controls and delegates dismissal', async () => {
    const onclose = vi.fn();
    const view = render(RenderedImageViewerModal, {
      modal: { type: 'renderedImageViewer', src, alt: 'Diagram' },
      onclose
    });
    try {
      const dialog = document.querySelector('dialog')!;
      await expect.poll(() => dialog.matches(':modal')).toBe(true);
      expect(dialog.closest('.prose')).toBeNull();
      expect(dialog.querySelector('img')?.src).toBe(src);
      expect(dialog.querySelector('[data-image-controls] output')?.textContent).toBe('100%');
      expect(dialog.querySelector('.image-canvas output')).toBeNull();
      expect(dialog.querySelector('a[download]')?.getAttribute('href')).toBe(src);
      await userEvent.click(dialog.querySelector('[aria-label="Zoom out"]')!);
      expect(dialog.querySelector('output')?.textContent).toBe('75%');
      dialog.dispatchEvent(new Event('cancel', { cancelable: true }));
      await expect.poll(() => document.querySelector('dialog[open]')).toBeNull();
      await expect.poll(() => onclose.mock.calls.length).toBe(1);
    } finally {
      await view.unmount();
    }
    expect(document.querySelector('dialog')).toBeNull();
  });
});
