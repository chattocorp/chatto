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

let originalClipboard: PropertyDescriptor | undefined;
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
  it('dims the foreground without fading the background of ANSI cells', async () => {
    const { container } = render(MarkdownHtml, {
      html: await renderMarkdown('```ansi\n\x1b[2;44mDIM\n```')
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
      html: await renderMarkdown('```ansi\n' + source + '```')
    });
    container.classList.add('prose');
    const code = container.querySelector('.markdown-ansi code')!;
    expect(code.textContent?.trim()).toBe('ART');
    expect(getComputedStyle(code.querySelector('span')!).color).toBe('rgb(170, 0, 0)');
    expect(getComputedStyle(code).backgroundColor).toBe('rgb(0, 0, 0)');
    expect(getComputedStyle(code).whiteSpace).toBe('pre');
    await userEvent.click(container.querySelector('button[data-markdown-copy]')!);
    expect(writeText).toHaveBeenCalledWith(source);
  });

  it('copies the original code from each block by mouse and keyboard', async () => {
    const html = await renderMarkdown('```text\n\tfirst\n```\n\n    second\n\n`inline`');
    const { container } = render(MarkdownHtml, { html });
    const buttons = container.querySelectorAll<HTMLButtonElement>('button[data-markdown-copy]');

    expect(buttons).toHaveLength(2);
    expect(buttons[0].previousElementSibling?.textContent).toBe('text');
    expect(buttons[1].previousElementSibling).toBeNull();
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
