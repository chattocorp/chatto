import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cdp, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import { renderMarkdown } from '$lib/markdown';

const toastMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const navigationMocks = vi.hoisted(() => ({ goto: vi.fn(), registered: true }));
vi.mock('$app/navigation', () => ({ goto: navigationMocks.goto, pushState: vi.fn() }));
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
import FloatingPopover from './FloatingPopover.svelte';
import { testSnippet } from '$lib/test-utils';

let originalClipboard: PropertyDescriptor | undefined;

describe('Mermaid previews', () => {
  it.each(['svg', 'mermaid'])(
    'reserves %s geometry immediately when a virtualized preview remounts',
    async (language) => {
      const source =
        language === 'svg'
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
    'fits a %s × %s visual frame to its artwork at wide and narrow widths',
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
          Math.min(width, availableWidth - 10, (Math.min(480, innerHeight * 0.6) * width) / height)
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
        expect(imageBounds.width).toBeLessThanOrEqual(width);
        expect(imageBounds.height).toBeLessThanOrEqual(height);
        expect(Math.abs(frame.getBoundingClientRect().width - bounds.width - 10)).toBeLessThan(1);
        expect(frame.scrollWidth).toBeLessThanOrEqual(frame.clientWidth + 1);
      }
    }
  );

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
  it('updates surface lighting with UI Depth and removes it in flat mode', async () => {
    const root = document.documentElement;
    const previous = root.style.getPropertyValue('--depth-level');
    try {
      root.style.setProperty('--depth-level', '0');
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown('```mermaid rendered\nflowchart LR\nA[Dock]-->B[Trade]\n```')
      });
      await expect
        .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
        .toBeGreaterThan(0);
      const image = container.querySelector<HTMLImageElement>('img')!;
      const source = () => decodeURIComponent(image.src.split(',')[1]);
      await expect.poll(source).not.toContain('chatto-pele-depth-edge');
      const width = image.naturalWidth;
      root.style.setProperty('--depth-level', '100');
      await expect.poll(source).toContain('chatto-pele-depth-edge');
      await expect.poll(() => image.complete && image.naturalWidth === width).toBe(true);
      root.style.setProperty('--depth-level', '0');
      await expect.poll(source).not.toContain('chatto-pele-depth-edge');
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
