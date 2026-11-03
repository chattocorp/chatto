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
  it('keeps the visual frame fitted while an animated poster is loading', async () => {
    const host = document.createElement('div');
    host.className = 'prose';
    host.style.width = '900px';
    const frame = document.createElement('div');
    frame.className = 'markdown-code-frame';
    frame.dataset.presentation = 'visual';
    const panel = document.createElement('div');
    panel.dataset.codePanel = 'render';
    frame.append(panel);
    host.append(frame);
    document.body.append(host);
    const image = new Image();
    // Hold the first load so the component must reserve the known dimensions.
    const view = render(RenderedImage, {
      target: panel,
      props: {
        image,
        initialSize: { width: 100, height: 200 },
        svgSource:
          '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="200"><circle r="8"><animate attributeName="cy" values="10;190" dur="4s"/></circle></svg>'
      }
    });
    try {
      expect(panel.querySelector('iframe')).toBeNull();
      const loadingWidth = frame.getBoundingClientRect().width;
      expect(loadingWidth).toBeLessThan(300);
      image.src =
        'data:image/svg+xml,' +
        encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="100" height="200"/>');
      await expect.poll(() => panel.querySelector('iframe')).toBeTruthy();
      expect(frame.getBoundingClientRect().width).toBeCloseTo(loadingWidth, 0);
    } finally {
      await view.unmount();
      host.remove();
    }
  });

  it('keeps animation playback controls in the viewer footer when zooming', async () => {
    const svgSource =
      '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100"><text x="10" y="20">動き 🙂</text><circle cx="10" cy="50" r="8"><animate attributeName="cx" values="10;190;10" dur="4s" repeatCount="indefinite"/></circle></svg>';
    const view = render(RenderedImageViewerModal, {
      modal: {
        type: 'renderedImageViewer',
        alt: 'Moving circle',
        svgSource
      },
      onclose: vi.fn()
    });
    try {
      await expect
        .poll(() =>
          document
            .querySelector<HTMLIFrameElement>('dialog iframe')
            ?.contentDocument?.querySelector('svg')
        )
        .toBeTruthy();
      const control = document.querySelector<HTMLButtonElement>(
        'dialog [aria-label="Pause animation"], dialog [aria-label="Resume animation"]'
      )!;
      expect(control.closest('[data-image-controls]')).not.toBeNull();
      const poster = document.querySelector<HTMLImageElement>('dialog img')!;
      expect(decodeURIComponent(poster.src.split(',')[1])).not.toContain('<animate');
      const download = document.querySelector<HTMLAnchorElement>('dialog a[download]')!;
      expect(decodeURIComponent(download.href.split(',')[1])).toBe(svgSource);
      await userEvent.click(control);
      const root = document
        .querySelector<HTMLIFrameElement>('dialog iframe')!
        .contentDocument!.querySelector('svg')!;
      const paused = root.animationsPaused();
      await userEvent.click(document.querySelector('dialog [aria-label="Zoom in"]')!);
      expect(root.animationsPaused()).toBe(paused);
      expect(document.querySelector('dialog output')?.textContent).toBe('125%');
    } finally {
      await view.unmount();
    }
  });

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
