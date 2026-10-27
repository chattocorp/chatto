import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import { renderMarkdown } from '$lib/markdown';

const toastMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const navigationMocks = vi.hoisted(() => ({ goto: vi.fn(), registered: true }));
vi.mock('$app/navigation', () => ({ goto: navigationMocks.goto }));
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
      html: await renderMarkdown('```mermaid render\nflowchart LR\nA-->B\n```')
    });
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
    'shows the whole %s diagram without internal scrolling on a narrow screen',
    async (direction) => {
      const source =
        `flowchart ${direction}\n` +
        Array.from(
          { length: 10 },
          (_, i) => `A${i}[Checkpoint ${i}] --> A${i + 1}[Checkpoint ${i + 1}]`
        ).join('\n');
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown('```mermaid render\n' + source + '\n```')
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
      expect(image.getBoundingClientRect().width).toBeLessThanOrEqual(320);
      expect(block.clientHeight).toBeGreaterThanOrEqual(image.getBoundingClientRect().height - 1);
      if (direction === 'TB') expect(block.clientHeight).toBeGreaterThan(512);
      expect(block.hasAttribute('tabindex')).toBe(false);
    }
  );
  it('updates surface lighting with UI Depth and removes it in flat mode', async () => {
    const root = document.documentElement;
    const previous = root.style.getPropertyValue('--depth-level');
    try {
      root.style.setProperty('--depth-level', '0');
      const { container } = render(MarkdownHtml, {
        html: await renderMarkdown('```mermaid render\nflowchart LR\nA[Dock]-->B[Trade]\n```')
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
        '```mermaid render\nflowchart LR\naccTitle: Flight plan\naccDescr: Launch then orbit\nA[Launch] --> B[Orbit]\n```'
      )
    });
    await expect
      .poll(
        () => container.querySelector<HTMLImageElement>('.markdown-mermaid-preview')?.naturalWidth
      )
      .toBeGreaterThan(0);
    const image = container.querySelector<HTMLImageElement>('img')!;
    expect(image.alt).toBe('Flight plan. Launch then orbit');
    expect(container.querySelector('svg')).toBeNull();
    expect(container.querySelector<HTMLElement>('[data-code-panel="source"]')?.hidden).toBe(true);
    const before = image.src;
    const previous = document.documentElement.dataset.theme;
    try {
      document.documentElement.dataset.theme = previous === 'dark' ? 'light' : 'dark';
      await expect.poll(() => image.src).not.toBe(before);
    } finally {
      if (previous === undefined) delete document.documentElement.dataset.theme;
      else document.documentElement.dataset.theme = previous;
    }
  });

  it('keeps invalid source copyable without blocking a valid sibling', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown(
        '```mermaid render\nnot a diagram\n```\n\n```mermaid render\nflowchart LR\nA-->B\n```'
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
      html: await renderMarkdown('```mermaid render\nflowchart LR\nOld-->Diagram\n```')
    });
    await view.rerender({ html: await renderMarkdown('Replacement message') });
    await expect.element(view.container).toHaveTextContent('Replacement message');
    expect(view.container.querySelector('img')).toBeNull();
  });

  it('defers offscreen diagrams until they approach the viewport', async () => {
    const view = render(MarkdownHtml, {
      html: await renderMarkdown('```mermaid render\nflowchart LR\nA-->B\n```')
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
    'resolves middle-click with the current registration after rendering (initially %s)',
    async (registered) => {
      navigationMocks.registered = registered;
      const url = 'https://bar.com/chat/-/overview';
      const { container } = render(MarkdownHtml, { html: await renderMarkdown(`[Go](${url})`) });
      navigationMocks.registered = !registered;
      const open = vi.spyOn(window, 'open').mockReturnValue(null);
      container
        .querySelector('a')!
        .dispatchEvent(new MouseEvent('auxclick', { button: 1, bubbles: true, cancelable: true }));
      expect(open).toHaveBeenCalledWith(
        registered ? url : '/chat/bar.com/overview',
        '_blank',
        'noopener,noreferrer'
      );
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
    await userEvent.click(container.querySelector('a')!);
    expect(navigationMocks.goto).not.toHaveBeenCalled();
    expect(open).toHaveBeenCalledWith(url, '_blank', 'noopener,noreferrer');
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
      html: await renderMarkdown('```text\n' + source + '```\n\n```ansi render\nART\n```')
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

  it.each(['light', 'dark'])(
    'keeps code chrome above the message surface in %s mode',
    async (theme) => {
      const root = document.documentElement;
      const previous = root.dataset.theme;
      const transition = root.style.transition;
      try {
        root.style.transition = 'none';
        root.dataset.theme = theme;
        const { container } = render(MarkdownHtml, {
          html: await renderMarkdown('```ruby\nputs "Hello"\n```')
        });
        container.classList.add('prose', 'message-row');
        const frame = container.querySelector<HTMLElement>('.markdown-code-frame')!;
        const header = container.querySelector<HTMLElement>('.markdown-code-header')!;
        await userEvent.hover(document.body);
        const normal = getComputedStyle(header).backgroundColor;
        expect(getComputedStyle(frame).borderTopColor).toBe(normal);
        await userEvent.hover(header);
        const highlighted = getComputedStyle(header).backgroundColor;
        expect(highlighted).not.toBe(normal);
        expect(getComputedStyle(frame).borderTopColor).toBe(highlighted);
        container.classList.add('bg-surface');
        await userEvent.hover(document.body);
        expect(getComputedStyle(header).backgroundColor).toBe(highlighted);
        expect(getComputedStyle(frame).borderTopColor).toBe(highlighted);
        container.classList.remove('bg-surface');
        expect(getComputedStyle(header).backgroundColor).toBe(normal);
      } finally {
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

  it('renders SVG inside a shared frame with source and copy controls', async () => {
    const source =
      '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"><rect width="80" height="40" fill="red"/></svg>\n';
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```svg render\n' + source + '```')
    });
    await expect
      .poll(() => container.querySelector<HTMLImageElement>('img')?.naturalWidth)
      .toBe(80);
    const image = container.querySelector('img')!;
    expect(container.querySelector('svg')).toBeNull();
    container.classList.add('prose');
    expect(container.querySelector('details, summary')).toBeNull();
    await expect
      .element(container.querySelector<HTMLButtonElement>('button[data-markdown-copy]')!)
      .toBeVisible();
    expect(getComputedStyle(image.parentElement!).borderTopWidth).toBe('0px');
    expect(getComputedStyle(image).backgroundColor).toBe('rgba(0, 0, 0, 0)');
    expect(getComputedStyle(image).outlineStyle).toBe('none');
    expect(getComputedStyle(image).borderRadius).toBe('0px');
  });

  it('reveals malformed SVG source instead of a broken image', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```svg render\n<svg broken\n```')
    });
    await expect.element(container.querySelector('code')!).toBeVisible();
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
      html: await renderMarkdown('```svg render\n' + source + '\n```')
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
    await rendered.rerender({ html: await renderMarkdown('```svg render\n<svg broken\n```') });
    await expect.element(rendered.container.querySelector('code')!).toBeVisible();
    await rendered.rerender({ html: await renderMarkdown('Updated message') });
    expect(rendered.container.querySelector('img, details')).toBeNull();
  });

  it('dims the foreground without fading the background of ANSI cells', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```ansi render\n\x1b[2;44mDIM\n```')
    });
    const span = container.querySelector('.markdown-ansi code span')!;
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
      html: await renderMarkdown('```ansi render\n' + source + '```')
    });
    container.classList.add('prose');
    const code = container.querySelector('.markdown-ansi code')!;
    expect(code.textContent?.trim()).toBe('ART');
    expect(getComputedStyle(code.querySelector('span')!).color).toBe('rgb(170, 0, 0)');
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
