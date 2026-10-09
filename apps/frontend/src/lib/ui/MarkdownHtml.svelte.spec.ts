import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import { renderMarkdown } from '$lib/markdown';

const toastMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock('$lib/ui/toast', () => ({ toast: toastMocks }));

import MarkdownHtml from './MarkdownHtml.svelte';

let originalClipboard: PropertyDescriptor | undefined;
const writeText = vi.fn<(_: string) => Promise<void>>();

beforeEach(() => {
  originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText }
  });
  writeText.mockReset().mockResolvedValue(undefined);
  toastMocks.success.mockReset();
  toastMocks.error.mockReset();
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
