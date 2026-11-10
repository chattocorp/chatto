import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cdp, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import { renderMarkdown } from '$lib/markdown';

const toastMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const navigationMocks = vi.hoisted(() => ({ goto: vi.fn(), pushState: vi.fn(), registered: true }));
vi.mock('$app/navigation', () => ({
  goto: navigationMocks.goto,
  pushState: navigationMocks.pushState
}));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return navigationMocks.registered ? [{ id: 'remote', url: 'https://bar.com' }] : [];
    },
    originServer: { id: 'origin' },
    isOriginServer: (id: string) => id === 'origin',
    getServer: () => ({ id: 'remote', url: 'https://bar.com' })
  }
}));

vi.mock('$lib/ui/toast', () => ({ toast: toastMocks }));

import MarkdownHtml from './MarkdownHtml.svelte';
import DiagramTimelineTestHarness from './DiagramTimelineTestHarness.svelte';
import FloatingPopover from './FloatingPopover.svelte';
import { testSnippet } from '$lib/test-utils';

let originalClipboard: PropertyDescriptor | undefined;

describe('Sanitized SVG previews', () => {
  it.each(['blocked', 'missing-policy'])(
    'reveals source for a %s animation frame',
    async (failure) => {
      const documentGetter = Object.getOwnPropertyDescriptor(
        HTMLIFrameElement.prototype,
        'contentDocument'
      )!.get!;
      const blockedFrame = vi
        .spyOn(HTMLIFrameElement.prototype, 'contentDocument', 'get')
        .mockImplementation(function (this: HTMLIFrameElement) {
          const frameDocument: Document | null = documentGetter.call(this);
          if (this.getAttribute('sandbox') !== 'allow-same-origin') return frameDocument;
          if (failure === 'blocked') return null;
          frameDocument?.querySelector('meta[http-equiv="Content-Security-Policy"]')?.remove();
          return frameDocument;
        });
      const source =
        '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100"><circle r="8"><animate attributeName="cx" values="10;190" dur="60s" repeatCount="indefinite"/></circle></svg>';
      const view = render(MarkdownHtml, {
        html: await renderMarkdown('```svg rendered\n' + source + '\n```')
      });
      try {
        await expect
          .poll(
            () =>
              view.container.querySelector<HTMLButtonElement>('[data-code-tab="render"]')?.disabled
          )
          .toBe(true);
        expect(view.container.querySelector('iframe')).toBeNull();
        expect(view.container.querySelector<HTMLElement>('.markdown-code-error')?.hidden).toBe(
          false
        );
        expect(view.container.querySelector('code')?.textContent).toContain(source);
      } finally {
        blockedFrame.mockRestore();
        await view.unmount();
      }
    }
  );

  it('keeps the frame when unchanged HTML and owner props are assigned again', async () => {
    const html = await renderMarkdown(
      '```svg rendered\n<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100"><circle r="8"><animate attributeName="cx" values="10;190" dur="60s" repeatCount="indefinite"/></circle></svg>\n```'
    );
    const artifactOwner = {};
    const view = render(MarkdownHtml, { html, artifactOwner });
    try {
      await expect
        .poll(() => view.container.querySelector('iframe')?.contentDocument?.querySelector('svg'))
        .toBeTruthy();
      const frame = view.container.querySelector('iframe')!;
      const svg = frame.contentDocument!.querySelector('svg');
      await view.rerender({ html, artifactOwner });
      await new Promise(requestAnimationFrame);
      expect(view.container.querySelector('iframe')).toBe(frame);
      expect(frame.contentDocument!.querySelector('svg')).toBe(svg);
    } finally {
      await view.unmount();
    }
  });

  it('offers playback on animated SVG and passes sanitized source to the zoom viewer', async () => {
    const source =
      '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100"><circle cx="10" cy="50" r="8"><animate attributeName="cx" values="10;190;10" dur="4s" repeatCount="indefinite"/></circle><script>alert(1)</script></svg>';
    const html = await renderMarkdown('```svg rendered\n' + source + '\n```');
    const view = render(MarkdownHtml, { html });
    await expect
      .poll(() => view.container.querySelector('iframe')?.contentDocument?.querySelector('svg'))
      .toBeTruthy();
    const playback = view.container.querySelector<HTMLButtonElement>(
      '[aria-label="Pause animation"]'
    )!;
    expect(playback).not.toBeNull();
    await userEvent.click(playback);
    await userEvent.click(
      view.container.querySelector<HTMLButtonElement>('button.rendered-image')!
    );
    const modal = navigationMocks.pushState.mock.lastCall![1].modal;
    expect(modal.svgSource).toContain('<animate');
    expect(modal.svgSource).not.toContain('<script');
    expect(modal).not.toHaveProperty('src');
    const poster = view.container.querySelector<HTMLImageElement>('img')!;
    expect(decodeURIComponent(poster.src.split(',')[1])).not.toContain('<animate');
  });

  it('uses sanitized SVG for the preview and viewer while retaining the original source', async () => {
    const source =
      '<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><script>alert(1)</script><rect width="20" height="20" fill="red" onload="alert(1)"/></svg>';
    const html = await renderMarkdown('```svg rendered\n' + source + '\n```');
    const { container } = render(MarkdownHtml, { html });
    await expect
      .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBe(20);
    const image = container.querySelector<HTMLImageElement>('img')!;
    const clean = decodeURIComponent(image.src.split(',')[1]);
    expect(clean).not.toMatch(/script|onload|alert\(/);
    expect(clean).toContain('<rect');
    expect(
      container.querySelector<HTMLElement>('[data-artifact-source]')?.dataset.artifactSource
    ).toContain(source);
    await userEvent.click(container.querySelector<HTMLButtonElement>('button.rendered-image')!);
    expect(navigationMocks.pushState).toHaveBeenLastCalledWith('', {
      modal: { type: 'renderedImageViewer', src: image.src, alt: 'SVG' }
    });
  });

  it('reveals source and disables Render when an SVG declaration is rejected', async () => {
    const html = await renderMarkdown(
      '```svg rendered\n<!DOCTYPE svg><svg xmlns="http://www.w3.org/2000/svg"/>\n```'
    );
    const { container } = render(MarkdownHtml, { html });
    await expect
      .poll(() => container.querySelector<HTMLButtonElement>('[data-code-tab="render"]')?.disabled)
      .toBe(true);
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector<HTMLElement>('.markdown-code-error')?.hidden).toBe(false);
  });
});

describe('Mermaid previews', () => {
  it.each(['svg', 'mermaid'])(
    'reuses the loaded %s image when wheel scrolling remounts a row',
    async (language) => {
      const jobs = vi.spyOn(Worker.prototype, 'postMessage');
      const html = await Promise.all(
        Array.from({ length: 8 }, (_, index) =>
          renderMarkdown(
            '```' +
              language +
              ' rendered\n' +
              (language === 'svg'
                ? `<svg xmlns="http://www.w3.org/2000/svg" width="300" height="400"><title>Wheel row ${index}</title><rect width="300" height="400" fill="red"/></svg>`
                : `flowchart TB\nA[Wheel row ${index}]-->B[Second]-->C[Third]-->D[Fourth]`) +
              '\n```'
          )
        )
      );
      // Give the virtualizer known geometry. This test covers remounts, not
      // cold rows growing while native wheel scrolling is still in progress.
      for (const value of html) {
        const preview = render(MarkdownHtml, { html: value });
        await expect
          .poll(() => preview.container.querySelector<HTMLImageElement>('img')?.naturalWidth)
          .toBeGreaterThan(0);
        await preview.unmount();
      }
      jobs.mockClear();
      const { container } = render(DiagramTimelineTestHarness, { html });
      const scroller = container.querySelector<HTMLElement>('[data-testid="diagram-timeline"]')!;
      const firstImage = () =>
        scroller.querySelector<HTMLImageElement>('[data-diagram-row="0"] img');
      await expect.poll(() => firstImage()?.naturalWidth).toBeGreaterThan(0);
      const original = firstImage();
      const { x, y } = scroller.getBoundingClientRect();
      const wheel = (deltaY: number) =>
        cdp().send('Input.dispatchMouseEvent', {
          type: 'mouseWheel',
          x: x + 100,
          y: y + 100,
          deltaX: 0,
          deltaY
        });
      for (let step = 0; step < 5; step++) {
        await wheel(300);
        await new Promise(requestAnimationFrame);
        await new Promise(requestAnimationFrame);
        await expect.poll(() => scroller.querySelector('[aria-busy="true"]')).toBeNull();
      }
      await expect.poll(() => scroller.querySelector('[data-diagram-row="0"]')).toBeNull();
      for (let step = 0; step < 5; step++) {
        await wheel(-300);
        await new Promise(requestAnimationFrame);
        await new Promise(requestAnimationFrame);
      }
      await expect.poll(() => firstImage()?.naturalWidth).toBeGreaterThan(0);
      expect(firstImage()).toBe(original);
      expect(
        jobs.mock.calls.filter(([source]) => String(source).includes('Wheel row 0]'))
      ).toHaveLength(language === 'mermaid' ? 1 : 0);
    }
  );

  it('reuses a loaded message layout on remount but renders changed source and new owners', async () => {
    const artifactOwner = {};
    const jobs = vi.spyOn(Worker.prototype, 'postMessage');
    const html = await renderMarkdown(
      '```mermaid rendered\nflowchart LR\nA[Reuse]-->B[Layout]\n```'
    );
    const loaded = async (container: HTMLElement) => {
      await expect
        .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      return container.querySelector('img')!;
    };
    const first = render(MarkdownHtml, { html, artifactOwner });
    const firstImage = await loaded(first.container);
    expect(jobs).toHaveBeenCalledOnce();
    await first.unmount();
    jobs.mockClear();

    const second = render(MarkdownHtml, { html, artifactOwner });
    const secondImage = await loaded(second.container);
    expect(secondImage).toBe(firstImage);
    expect(secondImage.src).toBe(firstImage.src);
    expect(jobs).not.toHaveBeenCalled();
    await second.rerender({
      html: await renderMarkdown(
        '```mermaid rendered\nflowchart LR\nA[Edited]-->C[New layout]\n```'
      )
    });
    await loaded(second.container);
    expect(jobs).toHaveBeenCalledOnce();
    await second.unmount();

    jobs.mockClear();
    const fresh = render(MarkdownHtml, { html, artifactOwner: {} });
    await loaded(fresh.container);
    expect(jobs).toHaveBeenCalledOnce();
  });

  it.each(['svg', 'mermaid'])(
    'keeps simultaneous %s previews independent and reuses only the unchanged image',
    async (language) => {
      const artifactOwner = {};
      const markup = (label: string) =>
        renderMarkdown(
          '```' +
            language +
            ' rendered\n' +
            (language === 'svg'
              ? `<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"><title>${label}</title></svg>`
              : `flowchart LR\nA[${label}]-->B[Preview]`) +
            '\n```'
        );
      const html = await markup('Original');
      const first = render(MarkdownHtml, { html, artifactOwner });
      await expect
        .poll(() => first.container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      const original = first.container.querySelector('img')!;
      await first.unmount();
      const loads = vi.fn();
      original.addEventListener('load', loads);
      const second = render(MarkdownHtml, { html, artifactOwner });
      // Cached SVG mounting is synchronous; no empty intersection-observer frame.
      if (language === 'svg') expect(second.container.querySelector('img')).toBe(original);
      await expect.poll(() => second.container.querySelector('img')).toBe(original);
      const simultaneous = render(MarkdownHtml, { html, artifactOwner });
      await expect
        .poll(() => simultaneous.container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      expect(simultaneous.container.querySelector('img')).not.toBe(original);
      expect(second.container.contains(original)).toBe(true);
      expect(loads).not.toHaveBeenCalled();
      original.removeEventListener('load', loads);
      await second.rerender({ html: await markup('Edited') });
      await expect
        .poll(() => second.container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      expect(second.container.querySelector('img')).not.toBe(original);
      const fresh = render(MarkdownHtml, { html, artifactOwner: {} });
      await expect
        .poll(() => fresh.container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      expect(fresh.container.querySelector('img')).not.toBe(original);
    }
  );

  it('does not correct upward scrolling when Gantt rows remount', async () => {
    const source = `gantt
    title A Gantt Diagram
    dateFormat  YYYY-MM-DD
    section Section
    A task           :a1, 2014-01-01, 30d
    Another task     :after a1  , 20d
    section Another
    Task in sec      :2014-01-12  , 12d
    another task      : 24d`;
    const html = await Promise.all(
      Array.from({ length: 30 }, (_, index) =>
        renderMarkdown(
          `Message ${index}\n\n` + ('```mermaid rendered\n' + source + '\n```\n\n').repeat(3)
        )
      )
    );
    // Load the shared diagram geometry before testing remounts. Cold rows can
    // finish loading only on the return trip and legitimately change height.
    const preview = render(MarkdownHtml, { html: html[0] });
    await expect
      .poll(
        () =>
          [...preview.container.querySelectorAll('img')].filter((image) => image.naturalWidth > 0)
            .length,
        { timeout: 5000 }
      )
      .toBe(3);
    await preview.unmount();
    const { container } = render(DiagramTimelineTestHarness, { html });
    const scroller = container.querySelector<HTMLElement>('[data-testid="diagram-timeline"]')!;
    await expect.poll(() => scroller.querySelectorAll('img').length).toBeGreaterThanOrEqual(3);
    // Scroll through loaded rows, then revisit them continuously. Jumping once
    // and waiting for an idle layout misses corrections during an active scroll.
    for (let frame = 0; frame < 160; frame++) {
      scroller.scrollTop += 12;
      await new Promise(requestAnimationFrame);
    }
    expect(scroller.scrollTop).toBeGreaterThan(1500);
    expect(scroller.querySelector('[data-diagram-row="0"]')).toBeNull();
    // Base the return distance on the position after the virtualizer measures
    // the rows. Check each frame for corrections while those rows remount.
    const returnFrames = Math.ceil(scroller.scrollTop / 12);
    for (let frame = 0; frame < returnFrames; frame++) {
      scroller.scrollTop -= 12;
      const requested = scroller.scrollTop;
      await new Promise(requestAnimationFrame);
      expect(Math.abs(scroller.scrollTop - requested)).toBeLessThan(2);
    }
    expect(scroller.scrollTop).toBe(0);
    expect(scroller.querySelector('[data-diagram-row="0"]')).not.toBeNull();
  });

  it.each(['svg', 'small svg', 'mermaid'])(
    'reserves %s geometry immediately when a virtualized preview remounts',
    async (example) => {
      const language = example === 'mermaid' ? 'mermaid' : 'svg';
      const source =
        example === 'small svg'
          ? '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="150"/>'
          : language === 'svg'
            ? '<svg xmlns="http://www.w3.org/2000/svg" width="620" height="810"/>'
            : 'flowchart TB\nA[Retained geometry]-->B[Remounted preview]-->C[Stable scroll]';
      const html = await renderMarkdown('```' + language + ' rendered\n' + source + '\n```');
      for (const availableWidth of [900, 240]) {
        const first = render(MarkdownHtml, { html });
        first.container.classList.add('prose');
        first.container.style.width = `${availableWidth}px`;
        await expect
          .poll(() => first.container.querySelector<HTMLImageElement>('img')?.naturalWidth)
          .toBeGreaterThan(0);
        const bounds = first.container.querySelector('[data-artifact]')!.getBoundingClientRect();
        await first.unmount();

        const second = render(MarkdownHtml, { html });
        second.container.classList.add('prose');
        second.container.style.width = `${availableWidth}px`;
        // This check precedes the intersection callback, worker job and image load.
        // Waiting for an image here would hide the virtualizer's empty measurement.
        const frame = second.container.querySelector('[data-artifact]')!;
        expect(Math.abs(frame.getBoundingClientRect().height - bounds.height)).toBeLessThan(1);
        expect(Math.abs(frame.getBoundingClientRect().width - bounds.width)).toBeLessThan(1);
        await expect
          .poll(() => second.container.querySelector<HTMLImageElement>('img')?.naturalWidth)
          .toBeGreaterThan(0);
        expect(Math.abs(frame.getBoundingClientRect().height - bounds.height)).toBeLessThan(1);
        expect(frame.querySelector('[aria-hidden="true"].rendered-image')).toBeNull();
        await second.unmount();
      }
    }
  );

  it.each([
    [1200, 600],
    [480, 960],
    [640, 640],
    [10000, 100],
    [100, 10000],
    [80, 40],
    [10, 10]
  ])(
    'scales a %s × %s visual frame to the message width within the height limit',
    async (width, height) => {
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown(
          `\u0060\u0060\u0060svg rendered\n<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"/>\n\u0060\u0060\u0060`
        )
      });
      container.classList.add('prose');
      const frame = container.querySelector<HTMLElement>('[data-artifact]')!;
      await expect
        .poll(() => frame.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBe(width);
      const stage = frame.querySelector<HTMLElement>('.rendered-image')!;
      for (const availableWidth of [900, 240]) {
        container.style.width = `${availableWidth}px`;
        const expectedWidth = Math.max(
          44,
          Math.min(availableWidth - 10, (Math.min(480, innerHeight * 0.6) * width) / height)
        );
        await expect
          .poll(() => Math.abs(stage.getBoundingClientRect().width - expectedWidth))
          .toBeLessThan(1);
        const bounds = stage.getBoundingClientRect();
        const expectedHeight = Math.max(
          44,
          Math.min(480, innerHeight * 0.6, (bounds.width * height) / width)
        );
        expect(Math.abs(bounds.height - expectedHeight)).toBeLessThan(2);
        expect(bounds.width).toBeGreaterThanOrEqual(44);
        expect(bounds.height).toBeGreaterThanOrEqual(44);
        const imageBounds = frame.querySelector('img')!.getBoundingClientRect();
        expect(Math.abs(imageBounds.width - bounds.width)).toBeLessThan(1);
        expect(Math.abs(imageBounds.height - bounds.height)).toBeLessThan(1);
        expect(getComputedStyle(frame.querySelector('img')!).objectFit).toBe('contain');
        expect(Math.abs(frame.getBoundingClientRect().width - bounds.width - 10)).toBeLessThan(1);
        expect(frame.scrollWidth).toBeLessThanOrEqual(frame.clientWidth + 1);
      }
    }
  );

  it('enlarges a small Mermaid diagram within the message width and height limit', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```mermaid rendered\nflowchart LR\nA-->B\n```')
    });
    container.classList.add('prose');
    container.style.width = '900px';
    await expect
      .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBeGreaterThan(0);
    const image = container.querySelector('img')!;
    const stage = container.querySelector<HTMLElement>('.rendered-image')!;
    const bounds = stage.getBoundingClientRect();
    const expectedWidth = Math.min(
      890,
      (Math.min(480, innerHeight * 0.6) * image.naturalWidth) / image.naturalHeight
    );
    expect(Math.abs(bounds.width - expectedWidth)).toBeLessThan(1);
    expect(image.getBoundingClientRect().width).toBeGreaterThan(image.naturalWidth);
    expect(bounds.height).toBeLessThanOrEqual(Math.min(480, innerHeight * 0.6) + 1);
  });

  it.each(['svg', 'mermaid', 'ansi'])(
    'distinguishes code and visual presentation for %s',
    async (language) => {
      const source =
        language === 'svg'
          ? '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"/>'
          : language === 'mermaid'
            ? 'flowchart LR\nA-->B'
            : 'ART';
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown(
          '```' +
            language +
            '\n' +
            source +
            '\n```\n\n```' +
            language +
            ' rendered\n' +
            source +
            '\n```'
        )
      });
      container.classList.add('prose');
      const [code, visual] = [...container.querySelectorAll<HTMLElement>('[data-artifact]')];
      expect(code.dataset.presentation).toBe('code');
      expect(visual.dataset.presentation).toBe('visual');
      expect(getComputedStyle(code.querySelector('.markdown-code-header')!).position).toBe(
        'static'
      );
      expect(
        visual.querySelector('.markdown-code-header, [data-code-tab], [data-code-panel="source"]')
      ).toBeNull();
      await userEvent.click(code.querySelector('[data-code-tab="render"]')!);
      await expect
        .element(code.querySelector<HTMLElement>('[data-code-tab="source"]')!)
        .toBeVisible();
    }
  );

  it.each(['svg', 'mermaid', 'ansi'])(
    'keeps %s source available through independent keyboard tabs',
    async (language) => {
      const source =
        language === 'svg'
          ? '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"/>\n'
          : language === 'mermaid'
            ? 'flowchart LR\nA-->B\n'
            : '\x1b[31mART\n';
      const html = await renderMarkdown('```' + language + '\n' + source + '```');
      const first = render(MarkdownHtml, { html }).container;
      const second = render(MarkdownHtml, { html }).container;
      const tabs = first.querySelectorAll<HTMLButtonElement>('[role="tab"]');
      expect(first.querySelector('img')).toBeNull();
      expect(tabs[0].getAttribute('aria-selected')).toBe('true');
      const ids = [...first.querySelectorAll('[id]'), ...second.querySelectorAll('[id]')].map(
        (node) => node.id
      );
      expect(new Set(ids).size).toBe(ids.length);
      for (const tab of tabs)
        expect(
          first.querySelector('#' + CSS.escape(tab.getAttribute('aria-controls')!))
        ).not.toBeNull();
      tabs[0].focus();
      await userEvent.keyboard('{ArrowRight}');
      await expect.element(tabs[1]).toHaveFocus();
      if (language !== 'ansi')
        await expect
          // The first Mermaid job also loads its worker chunk. The assertion's
          // default one-second timeout is shorter than the three-second job limit.
          .poll(() => first.querySelector<HTMLImageElement>('img')?.naturalWidth, {
            timeout: 5_000
          })
          .toBeGreaterThan(0);
      await userEvent.click(first.querySelector('[data-markdown-copy]')!);
      expect(writeText).toHaveBeenCalledWith(source);
      expect(second.querySelector('[role="tab"]')?.getAttribute('aria-selected')).toBe('true');
      tabs[1].focus();
      await userEvent.keyboard('{Home}');
      await expect
        .element(first.querySelector<HTMLElement>('[data-code-panel="source"]')!)
        .toBeVisible();
      await expect
        .element(first.querySelector<HTMLElement>('[data-code-panel="render"]')!)
        .not.toBeVisible();
    },
    10_000
  );

  it('does not switch back to Render when a pending image finishes', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```mermaid\nflowchart LR\nA-->B\n```')
    });
    await userEvent.click(container.querySelector('[data-code-tab="render"]')!);
    await expect
      .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBeGreaterThan(0);
    await userEvent.click(container.querySelector('[data-code-tab="source"]')!);
    container.querySelector('img')!.dispatchEvent(new Event('load'));
    expect(container.querySelector('[data-code-tab="source"]')?.getAttribute('aria-selected')).toBe(
      'true'
    );
  });

  it.each(['TB', 'LR'])(
    'bounds the %s diagram in the shared image canvas on a narrow screen',
    async (direction) => {
      const source =
        `flowchart ${direction}\n` +
        Array.from(
          { length: 10 },
          (_, i) => `A${i}[Checkpoint ${i}] --> A${i + 1}[Checkpoint ${i + 1}]`
        ).join('\n');
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown('```mermaid rendered\n' + source + '\n```')
      });
      container.classList.add('prose');
      container.style.width = '320px';
      await expect
        .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      const block = container.querySelector<HTMLElement>('[data-artifact="mermaid"]')!;
      const image = block.querySelector('img')!;
      expect(getComputedStyle(block).overflowY).toBe('hidden');
      expect(block.scrollHeight).toBeLessThanOrEqual(block.clientHeight + 1);
      expect(block.scrollWidth).toBeLessThanOrEqual(block.clientWidth + 1);
      const stage = block.querySelector<HTMLElement>('.rendered-image')!;
      expect(stage.getBoundingClientRect().width).toBeLessThanOrEqual(320);
      await expect.poll(() => stage.clientHeight).toBeGreaterThan(0);
      expect(stage.clientHeight).toBeLessThanOrEqual(Math.min(480, innerHeight * 0.6) + 1);
      expect(block.querySelector('button[aria-label="Zoom in"]')).toBeNull();
      expect(block.querySelector('button.rendered-image')).not.toBeNull();
      expect(image.getBoundingClientRect().height).toBeLessThanOrEqual(stage.clientHeight + 1);
      expect(stage.clientWidth).toBe(block.clientWidth - 8);
      expect(block.hasAttribute('tabindex')).toBe(false);
    }
  );
  it('keeps Mermaid artwork unchanged when UI Depth changes', async () => {
    const root = document.documentElement;
    const previous = root.style.getPropertyValue('--depth-level');
    try {
      root.style.setProperty('--depth-level', '0');
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown(
          '```mermaid rendered\nflowchart LR\nsubgraph Port\nA[Dock]-->B[Trade]\nend\n```'
        )
      });
      await expect
        .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      const image = container.querySelector<HTMLImageElement>('img')!;
      const originalUrl = image.src;
      const source = decodeURIComponent(originalUrl.split(',')[1]);
      expect(source).not.toMatch(/<filter\b|\bfilter\s*:|chatto-pele-/u);
      const width = image.naturalWidth;
      const loads = vi.fn();
      image.addEventListener('load', loads);
      for (const depth of ['100', '0']) {
        root.style.setProperty('--depth-level', depth);
        // Let the root mutation observer process the changed preference.
        await new Promise<void>((resolve) => queueMicrotask(resolve));
        root.dispatchEvent(new TransitionEvent('transitionend'));
        expect(image.src).toBe(originalUrl);
        expect(image.complete && image.naturalWidth === width).toBe(true);
      }
      expect(loads).not.toHaveBeenCalled();
      image.removeEventListener('load', loads);
      expect(container.querySelector('img')).toBe(image);
    } finally {
      if (previous) root.style.setProperty('--depth-level', previous);
      else root.style.removeProperty('--depth-level');
    }
  });
  it('loads a real worker result as an image with accessible text and updates its theme', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown(
        '```mermaid\nflowchart LR\naccTitle: Flight plan\naccDescr: Launch then orbit\nA[Launch] --> B[Orbit]\n```'
      )
    });
    await userEvent.click(container.querySelector('[data-code-tab="render"]')!);
    await expect
      .poll(
        () => container.querySelector<HTMLImageElement>('.markdown-mermaid-preview')?.naturalWidth
      )
      .toBeGreaterThan(0);
    const image = container.querySelector<HTMLImageElement>('img')!;
    expect(image.alt).toBe('Flight plan. Launch then orbit');
    expect(container.querySelector('svg')).toBeNull();
    expect(container.querySelector<HTMLElement>('[data-code-panel="source"]')?.hidden).toBe(true);
    await userEvent.click(container.querySelector('[data-code-tab="source"]')!);
    await userEvent.click(container.querySelector('[data-code-tab="render"]')!);
    expect(container.querySelector('img')).toBe(image);
    const before = image.src;
    const previous = document.documentElement.dataset.theme;
    try {
      document.documentElement.dataset.theme = previous === 'dark' ? 'light' : 'dark';
      await expect.poll(() => image.src).not.toBe(before);
      expect(container.querySelector('img')).toBe(image);
      expect(container.querySelector('[aria-label="Zoom in"]')).toBeNull();
    } finally {
      if (previous === undefined) delete document.documentElement.dataset.theme;
      else document.documentElement.dataset.theme = previous;
    }
  });

  it('keeps invalid source copyable without blocking a valid sibling', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown(
        '```mermaid rendered\nnot a diagram\n```\n\n```mermaid rendered\nflowchart LR\nA-->B\n```'
      )
    });
    await expect
      .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBeGreaterThan(0);
    expect(container.querySelector('pre')?.hidden).toBe(false);
    await userEvent.click(container.querySelector('button[data-markdown-copy]')!);
    expect(writeText).toHaveBeenCalledWith('not a diagram\n');
  });

  it('cancels old work when the HTML changes', async () => {
    const view = render(MarkdownHtml, {
      html: await renderMarkdown('```mermaid rendered\nflowchart LR\nOld-->Diagram\n```')
    });
    await view.rerender({ html: await renderMarkdown('Replacement message') });
    await expect.element(view.container).toHaveTextContent('Replacement message');
    expect(view.container.querySelector('img')).toBeNull();
  });

  it('defers offscreen diagrams until they approach the viewport', async () => {
    const view = render(MarkdownHtml, {
      html: await renderMarkdown('```mermaid rendered\nflowchart LR\nA-->B\n```')
    });
    const block = view.container.querySelector<HTMLElement>('[data-artifact="mermaid"]')!;
    block.style.marginTop = '10000px';
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
    );
    expect(block.querySelector('img')).toBeNull();
    block.style.marginTop = '0';
    await expect
      .poll(() => block.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBeGreaterThan(0);
  });
});
const writeText = vi.fn<(_: string) => Promise<void>>();

/** Observe application cancellation before blocking navigation in the test browser. */
function expectNativeClick(anchor: HTMLAnchorElement, type = 'click', button = 0) {
  let prevented: boolean | undefined;
  document.addEventListener(
    type,
    (event) => {
      prevented = event.defaultPrevented;
      event.preventDefault();
    },
    { once: true }
  );
  anchor.dispatchEvent(new MouseEvent(type, { button, bubbles: true, cancelable: true }));
  expect(prevented).toBe(false);
}

beforeEach(() => {
  navigationMocks.goto.mockReset();
  navigationMocks.registered = true;
  originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText }
  });
  writeText.mockReset().mockResolvedValue(undefined);
  toastMocks.success.mockReset();
  toastMocks.error.mockReset();
});

describe('MarkdownHtml link navigation', () => {
  it.each(['overview', 'search?q=hello#results', 'threads', 'settings/appearance', 'manage/rooms'])(
    'rewrites general Chatto routes: %s',
    async (suffix) => {
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown(`[Go](https://bar.com/chat/-/${suffix})`)
      });
      await userEvent.click(container.querySelector('a')!);
      expect(navigationMocks.goto).toHaveBeenCalledWith(`/chat/bar.com/${suffix}`);
    }
  );

  it.each([true, false])(
    'leaves middle-click on the rendered destination native after registration changes (initially %s)',
    async (registered) => {
      navigationMocks.registered = registered;
      const url = 'https://bar.com/chat/-/overview';
      const { container } = render(MarkdownHtml, { html: await renderMarkdown(`[Go](${url})`) });
      navigationMocks.registered = !registered;
      const open = vi.spyOn(window, 'open').mockReturnValue(null);
      const anchor = container.querySelector('a')!;
      expectNativeClick(anchor, 'auxclick', 1);
      expect(anchor.getAttribute('href')).toBe(registered ? '/chat/bar.com/overview' : url);
      expect(open).not.toHaveBeenCalled();
      expect(navigationMocks.goto).not.toHaveBeenCalled();
    }
  );
  it.each([
    ['https://bar.com/chat/-/R123456789abcde', '/chat/bar.com/R123456789abcde'],
    [
      'http://bar.com:8080/chat/bar.com/R123456789abcde/m/Eabc123DEF456gh',
      '/chat/bar.com/R123456789abcde/m/Eabc123DEF456gh'
    ]
  ])('rewrites and navigates registered conversation links: %s', async (url, path) => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    const { container } = render(MarkdownHtml, { html: await renderMarkdown(`[Go](${url})`) });
    const anchor = container.querySelector('a')!;
    expect(anchor.getAttribute('href')).toBe(path);
    await userEvent.click(anchor);
    expect(navigationMocks.goto).toHaveBeenCalledWith(path);
    expect(open).not.toHaveBeenCalled();
  });

  it.each([
    'https://bar.com/docs',
    'https://unknown.com/chat/-/R123456789abcde',
    'https://bar.com/chat/unknown.com/R123456789abcde'
  ])('opens other destinations externally: %s', async (url) => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    const { container } = render(MarkdownHtml, { html: await renderMarkdown(`[Go](${url})`) });
    const anchor = container.querySelector('a')!;
    expectNativeClick(anchor);
    expect(anchor.href).toBe(url);
    expect(anchor.target).toBe('_blank');
    expect(navigationMocks.goto).not.toHaveBeenCalled();
    expect(open).not.toHaveBeenCalled();
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  if (originalClipboard) {
    Object.defineProperty(navigator, 'clipboard', originalClipboard);
  } else {
    Reflect.deleteProperty(navigator, 'clipboard');
  }
});

describe('MarkdownHtml footnotes', () => {
  it('scopes targets to each instance even when rendered HTML is reused', async () => {
    const html = await renderMarkdown('Text[^note], repeated[^note].\n\n[^note]: Note.');
    const first = render(MarkdownHtml, { html }).container;
    const second = render(MarkdownHtml, { html }).container;
    const ids = [...first.querySelectorAll('[id]'), ...second.querySelectorAll('[id]')].map(
      (element) => element.id
    );

    expect(new Set(ids).size).toBe(ids.length);
    for (const container of [first, second]) {
      for (const link of container.querySelectorAll<HTMLAnchorElement>(
        '.footnote-ref > a, .footnote-backref'
      )) {
        const id = link.getAttribute('href')!.slice(1);
        expect(container.querySelector(`#${CSS.escape(id)}`)).not.toBeNull();
        expect(link.hasAttribute('target')).toBe(false);
      }
    }
  });

  it('navigates and returns by mouse and keyboard without reaching message link handlers', async () => {
    const html = await renderMarkdown('Text[^note], repeated[^note].\n\n[^note]: Note.');
    const { container } = render(MarkdownHtml, { html });
    const references = container.querySelectorAll<HTMLAnchorElement>('.footnote-ref > a');
    const note = container.querySelector<HTMLElement>('.footnote-item')!;
    const backlinks = container.querySelectorAll<HTMLAnchorElement>('.footnote-backref');
    const scroll = vi.spyOn(note, 'scrollIntoView');
    const parentClick = vi.fn();
    container.addEventListener('click', parentClick);
    const url = location.href;

    await userEvent.click(references[0]);
    await expect.element(note).toHaveFocus();
    expect(scroll).toHaveBeenCalledWith({ block: 'nearest' });
    await userEvent.click(backlinks[0]);
    await expect.element(references[0]).toHaveFocus();

    references[1].focus();
    await userEvent.keyboard('{Enter}');
    await expect.element(note).toHaveFocus();
    backlinks[1].focus();
    await userEvent.keyboard('{Enter}');
    await expect.element(references[1]).toHaveFocus();
    expect(parentClick).not.toHaveBeenCalled();
    expect(location.href).toBe(url);
  });
});

describe('MarkdownHtml code copy', () => {
  it('keeps virtualized code row sizes and scroll anchors through wheel remounts and width changes', async () => {
    const source =
      '```svg\n<svg xmlns="http://www.w3.org/2000/svg">\n' +
      Array.from(
        { length: 60 },
        (_, index) => `<path d="M0 ${index} L900 900" fill="none" stroke="red"/>`
      ).join('\n') +
      '\n</svg>\n```';
    const html = await Promise.all(
      Array.from({ length: 20 }, (_, index) => renderMarkdown(`Source row ${index}\n\n${source}`))
    );
    const view = render(DiagramTimelineTestHarness, { html });
    await document.fonts.ready;
    const scroller = view.container.querySelector<HTMLElement>('[data-testid="diagram-timeline"]')!;
    const row = (index: number) =>
      scroller.querySelector<HTMLElement>(`[data-diagram-row="${index}"]`);
    await expect
      .poll(() =>
        row(0)?.querySelector('.markdown-code-fade-bottom')?.classList.contains('opacity-0')
      )
      .toBe(false);
    const height = row(0)!.getBoundingClientRect().height;
    const nextLayout = () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
      );
    const wheel = async (deltaY: number) => {
      const bounds = scroller.getBoundingClientRect();
      await cdp().send('Input.dispatchMouseEvent', {
        type: 'mouseWheel',
        x: bounds.x + 100,
        y: bounds.y + 100,
        deltaX: 0,
        deltaY
      });
      await nextLayout();
    };
    for (let step = 0; step < 6; step++) await wheel(500);
    await expect.poll(() => row(0)).toBeNull();

    const bounds = scroller.getBoundingClientRect();
    const anchor = [...scroller.querySelectorAll<HTMLElement>('[data-diagram-row]')].find(
      (element) => {
        const box = element.getBoundingClientRect();
        return box.bottom > bounds.top + 1 && box.top < bounds.bottom;
      }
    )!;
    const anchorIndex = Number(anchor.dataset.diagramRow);
    const offset = anchor.getBoundingClientRect().top - bounds.top;
    const scrollTop = scroller.scrollTop;
    for (const width of ['240px', '720px']) {
      scroller.style.width = width;
      await nextLayout();
      expect(row(anchorIndex)!.getBoundingClientRect().height).toBe(height);
      expect(row(anchorIndex)!.getBoundingClientRect().top - bounds.top).toBeCloseTo(offset, 0);
      expect(scroller.scrollTop).toBeCloseTo(scrollTop, 0);
      await expect
        .poll(() =>
          row(anchorIndex)!
            .querySelector('.markdown-code-fade-right')!
            .classList.contains('opacity-0')
        )
        .toBe(width === '720px');
    }

    for (let step = 0; step < 6; step++) await wheel(-500);
    await expect.poll(() => scroller.scrollTop).toBe(0);
    expect(row(0)!.getBoundingClientRect().height).toBe(height);
    await view.unmount();
    const remounted = render(DiagramTimelineTestHarness, { html });
    await expect
      .poll(
        () =>
          remounted.container
            .querySelector<HTMLElement>('[data-diagram-row="0"]')
            ?.getBoundingClientRect().height
      )
      .toBe(height);
  });

  it('shows scroll fades only at overflowing edges and refreshes them after resize and replacement', async () => {
    const source = Array.from({ length: 60 }, () => 'wide '.repeat(40)).join('\n');
    const view = render(MarkdownHtml, {
      html: await renderMarkdown('```text\n' + source + '\n```')
    });
    const { container } = view;
    container.classList.add('prose');
    container.style.width = '320px';
    const code = container.querySelector<HTMLElement>('pre > code')!;
    const visible = (edge: string) =>
      !container.querySelector(`.markdown-code-fade-${edge}`)!.classList.contains('opacity-0');
    await expect.poll(() => visible('bottom') && visible('right')).toBe(true);
    expect(visible('top')).toBe(false);
    expect(visible('left')).toBe(false);
    const fade = container.querySelector('.markdown-code-fade-bottom')!;
    expect(getComputedStyle(fade).pointerEvents).toBe('none');
    expect(getComputedStyle(fade).backgroundImage).toContain('linear-gradient');
    code.scrollTop = code.scrollHeight;
    code.scrollLeft = code.scrollWidth;
    await expect.poll(() => visible('top') && visible('left')).toBe(true);
    expect(visible('bottom')).toBe(false);
    expect(visible('right')).toBe(false);
    container.style.width = '4000px';
    await expect.poll(() => visible('left') || visible('right')).toBe(false);
    await view.rerender({ html: await renderMarkdown('```text\nshort\n```') });
    await expect.poll(() => ['top', 'bottom', 'left', 'right'].some(visible)).toBe(false);
    expect(container.querySelectorAll('.markdown-code-fade')).toHaveLength(4);
    expect(container.querySelectorAll('.markdown-code-scroll-content')).toHaveLength(1);
  });

  it('updates source and ANSI fades when switching tabs', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```ansi\n' + 'X'.repeat(70) + '\n```')
    });
    container.classList.add('prose');
    container.style.width = '240px';
    await userEvent.click(container.querySelector('[data-code-tab="render"]')!);
    const renderedFade = container.querySelector('.markdown-ansi .markdown-code-fade-right')!;
    await expect.poll(() => renderedFade.classList.contains('opacity-0')).toBe(false);
    expect(getComputedStyle(renderedFade).getPropertyValue('--tw-gradient-from').trim()).toBe(
      getComputedStyle(container.querySelector('.markdown-ansi')!).backgroundColor
    );
    await userEvent.click(container.querySelector('[data-code-tab="source"]')!);
    const sourceFade = container.querySelector(
      '[data-code-panel="source"] .markdown-code-fade-right'
    )!;
    await expect.poll(() => sourceFade.classList.contains('opacity-0')).toBe(false);
  });

  it.each(['source', 'render', 'copy'])(
    'dismisses an open popover when pressing %s',
    async (control) => {
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown('```ansi\nART\n```')
      });
      const onclose = vi.fn();
      const popover = render(FloatingPopover, {
        position: { x: 400, y: 300 },
        onclose,
        children: testSnippet('<div>Open menu</div>')
      });
      // Outside-press dismissal is attached on the frame after opening.
      await new Promise(requestAnimationFrame);
      const selector = control === 'copy' ? '[data-markdown-copy]' : `[data-code-tab="${control}"]`;
      await userEvent.click(container.querySelector(selector)!);
      expect(onclose).toHaveBeenCalledOnce();
      await popover.unmount();
    }
  );

  it('numbers source lines without a background or changing copied text', async () => {
    const source = 'first\n\n' + 'long '.repeat(80) + '\nlast\n';
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```text\n' + source + '```\n\n```ansi rendered\nART\n```')
    });
    container.classList.add('prose');
    container.style.width = '320px';
    const code = container.querySelector<HTMLElement>('[data-code-panel="source"] code')!;
    const numbers = code.querySelectorAll<HTMLElement>('.markdown-code-line-number');
    expect([...numbers].map((number) => number.dataset.line)).toEqual(['1', '2', '3', '4']);
    expect([...numbers].every((number) => number.getAttribute('aria-hidden') === 'true')).toBe(
      true
    );
    expect(numbers[0].textContent).toBe('');
    expect(getComputedStyle(numbers[0]).backgroundColor).toBe('rgba(0, 0, 0, 0)');
    const left = numbers[2].getBoundingClientRect().left;
    code.scrollLeft = 160;
    expect(code.scrollLeft).toBe(160);
    expect(numbers[2].getBoundingClientRect().left).toBe(left - 160);
    expect(
      container.querySelector('[data-code-panel="render"] .markdown-code-line-number')
    ).toBeNull();
    await userEvent.click(container.querySelector('[data-markdown-copy]')!);
    expect(writeText).toHaveBeenCalledWith(source);
  });

  it.each([
    ['light', false],
    ['dark', false],
    ['light', true],
    ['dark', true]
  ] as const)(
    'uses the shared preview frame and inset in %s mode with touch=%s',
    async (theme, touch) => {
      const root = document.documentElement;
      const previous = root.dataset.theme;
      const transition = root.style.transition;
      // The body's center can overlap the message. Use a separate hit target
      // so the normal-state assertion never depends on test viewport geometry.
      const outside = document.createElement('div');
      outside.style.cssText = 'position:fixed;top:0;left:0;width:4px;height:4px;z-index:9999';
      document.body.append(outside);
      try {
        await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
        root.style.transition = 'none';
        root.dataset.theme = theme;
        const { container } = render(MarkdownHtml, {
          html: await renderMarkdown('```ruby\nputs "Hello"\n```')
        });
        container.classList.add('prose', 'message-row');
        const frame = container.querySelector<HTMLElement>('.markdown-code-frame')!;
        const header = container.querySelector<HTMLElement>('.markdown-code-header')!;
        const reference = document.createElement('div');
        reference.className = 'preview-frame';
        outside.append(reference);
        const inset = document.createElement('div');
        inset.className = 'preview-content';
        reference.append(inset);
        const panel = frame.querySelector<HTMLElement>('[data-code-panel="source"]')!;
        const checkFrame = () => {
          for (const property of [
            'borderTopColor',
            'borderRadius',
            'backgroundColor',
            'paddingTop'
          ] as const)
            expect(getComputedStyle(frame)[property]).toBe(getComputedStyle(reference)[property]);
          for (const property of ['backgroundColor', 'borderRadius', 'boxShadow'] as const)
            expect(getComputedStyle(panel)[property]).toBe(getComputedStyle(inset)[property]);
        };
        await userEvent.hover(outside);
        expect(container.matches(':hover')).toBe(false);
        checkFrame();
        await userEvent.hover(header);
        checkFrame();
        container.classList.add('bg-surface');
        await userEvent.hover(outside);
        checkFrame();
      } finally {
        await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
        outside.remove();
        root.style.transition = transition;
        if (previous === undefined) delete root.dataset.theme;
        else root.dataset.theme = previous;
      }
    }
  );

  it('consumes header control gestures and labels the source, preview and copy icons', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```ansi\nART\n```')
    });
    const parentGesture = vi.fn();
    const events = ['click', 'mousedown', 'mouseup', 'touchstart', 'touchend'];
    const outsidePress = vi.fn();
    document.addEventListener('pointerdown', outsidePress);
    for (const type of events) container.addEventListener(type, parentGesture);
    const header = container.querySelector('.markdown-code-header')!;
    expect(header.firstElementChild?.textContent).toBe('ansi');
    expect(header.lastElementChild?.hasAttribute('data-markdown-copy')).toBe(true);
    const buttons = header.querySelectorAll<HTMLButtonElement>('button');
    expect([...buttons].map((button) => button.getAttribute('aria-label'))).toEqual([
      'Source',
      'Render',
      'Copy to clipboard'
    ]);
    for (const button of buttons) {
      expect(button.title).toBe(button.getAttribute('aria-label'));
      expect(button.querySelector('[aria-hidden="true"]')).not.toBeNull();
    }
    for (const button of buttons) {
      await userEvent.click(button);
      for (const type of ['touchstart', 'touchend']) {
        button.dispatchEvent(new Event(type, { bubbles: true }));
      }
      button.focus();
      await userEvent.keyboard('{Enter}');
    }
    expect(parentGesture).not.toHaveBeenCalled();
    expect(outsidePress).toHaveBeenCalledTimes(3);
    document.removeEventListener('pointerdown', outsidePress);
    expect(writeText).toHaveBeenCalledWith('ART\n');
    // Ordinary message content must still reach the message's handlers.
    container.querySelector('code')!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
    expect(parentGesture).toHaveBeenCalledOnce();
    for (const type of events) container.removeEventListener(type, parentGesture);
  });

  it('renders visual SVG without source or copy controls over the image', async () => {
    const source =
      '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"><rect width="80" height="40" fill="red"/></svg>\n';
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```svg rendered\n' + source + '```')
    });
    await expect
      .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBe(80);
    const image = container.querySelector('img')!;
    expect(container.querySelector('svg')).toBeNull();
    container.classList.add('prose');
    expect(container.querySelector('details, summary')).toBeNull();
    expect(container.querySelector('button[data-markdown-copy]')).toBeNull();
    expect(getComputedStyle(image.parentElement!).borderTopWidth).toBe('0px');
    expect(getComputedStyle(image).backgroundColor).toBe('rgba(0, 0, 0, 0)');
    expect(getComputedStyle(image).outlineStyle).toBe('none');
    expect(getComputedStyle(image).borderRadius).toBe('0px');
  });

  it('reveals malformed SVG source instead of a broken image', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```svg rendered\n<svg broken\n```')
    });
    container.classList.add('prose');
    await expect.poll(() => container.querySelector('code')).not.toBeNull();
    await expect.element(container.querySelector('code')!).toBeVisible();
    await expect
      .element(container.querySelector<HTMLElement>('[data-markdown-copy]')!)
      .toBeVisible();
    expect(container.querySelector<HTMLElement>('[data-code-panel="render"]')?.hidden).toBe(true);
    expect(container.querySelector('code')?.textContent).toBe('<svg broken');
  });

  it('isolates SVG scripts and external resources in image mode', async () => {
    const source =
      '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40" onload="parent.document.body.dataset.svgExecuted=1">' +
      '<script>parent.document.body.dataset.svgExecuted=1</script>' +
      '<image href="https://svg-preview.invalid/probe.png" width="80" height="40"/>' +
      '<rect width="80" height="40" fill="red"/></svg>';
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```svg rendered\n' + source + '\n```')
    });
    await expect
      .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBe(80);
    expect(document.body.dataset.svgExecuted).toBeUndefined();
    expect(container.querySelector('script, svg, iframe, object')).toBeNull();
    expect(
      performance
        .getEntriesByType('resource')
        .some((entry) => entry.name.includes('svg-preview.invalid'))
    ).toBe(false);
  });

  it('handles SVG failures after the message HTML changes', async () => {
    const rendered = render(MarkdownHtml, { html: await renderMarkdown('Original message') });
    await rendered.rerender({ html: await renderMarkdown('```svg rendered\n<svg broken\n```') });
    await expect.poll(() => rendered.container.querySelector('code')).not.toBeNull();
    await expect.element(rendered.container.querySelector('code')!).toBeVisible();
    await rendered.rerender({ html: await renderMarkdown('Updated message') });
    expect(rendered.container.querySelector('img, details')).toBeNull();
  });

  it('dims the foreground without fading the background of ANSI cells', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```ansi rendered\n\x1b[2;44mDIM\n```')
    });
    const span = container.querySelector('.markdown-ansi code span[style]')!;
    const style = getComputedStyle(span);
    expect(style.opacity).toBe('1');
    expect(style.backgroundColor).toBe('rgb(0, 0, 170)');
    // Resolve the CSS color through the browser without depending on its
    // choice of rgb() or color(srgb) serialization for the mixed color.
    const canvas = document.createElement('canvas');
    const context = canvas.getContext('2d')!;
    context.fillStyle = style.color;
    context.fillRect(0, 0, 1, 1);
    expect([...context.getImageData(0, 0, 1, 1).data]).toEqual([85, 85, 170, 255]);
  });

  it('renders ANSI screen cells in the browser and copies the original escape codes', async () => {
    const source = '\x1b[31mwrong\rART\x1b[K\r\n';
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```ansi\n' + source + '```')
    });
    container.classList.add('prose');
    await userEvent.click(container.querySelector('[data-code-tab="render"]')!);
    const code = container.querySelector('.markdown-ansi code')!;
    expect(code.textContent?.trim()).toBe('ART');
    expect(getComputedStyle(code.querySelector('span[style]')!).color).toBe('rgb(170, 0, 0)');
    expect(getComputedStyle(code.parentElement!).backgroundColor).toBe('rgb(0, 0, 0)');
    expect(getComputedStyle(code).whiteSpace).toBe('pre');
    await userEvent.click(container.querySelector('button[data-markdown-copy]')!);
    expect(writeText).toHaveBeenCalledWith(source);
  });

  it('copies the original code from each block by mouse and keyboard', async () => {
    const html = await renderMarkdown('```text\n\tfirst\n```\n\n    second\n\n`inline`');
    const { container } = render(MarkdownHtml, { html });
    const buttons = container.querySelectorAll<HTMLButtonElement>('button[data-markdown-copy]');

    expect(buttons).toHaveLength(2);
    expect(buttons[0].parentElement?.firstElementChild?.textContent).toBe('text');
    expect(buttons[1].parentElement?.firstElementChild?.textContent).toBe('text');
    expect(buttons[0].getAttribute('aria-label')).toBe('Copy to clipboard');
    expect(container.querySelector('p code')?.textContent).toBe('inline');
    await userEvent.click(buttons[0]);
    expect(writeText).toHaveBeenCalledWith('\tfirst\n');

    buttons[1].focus();
    await userEvent.keyboard('{Enter}');
    expect(writeText).toHaveBeenCalledWith('second\n');
    expect(toastMocks.success).toHaveBeenCalledTimes(2);
  });

  it('shows a failure toast when clipboard access fails', async () => {
    writeText.mockRejectedValueOnce(new Error('Clipboard denied'));
    const html = await renderMarkdown('```\nsecret\n```');
    const { container } = render(MarkdownHtml, { html });

    await userEvent.click(container.querySelector('button[data-markdown-copy]')!);
    expect(toastMocks.error).toHaveBeenCalledOnce();
    expect(toastMocks.success).not.toHaveBeenCalled();
  });
});
