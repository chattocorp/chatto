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
  if (originalClipboard) {
    Object.defineProperty(navigator, 'clipboard', originalClipboard);
  } else {
    Reflect.deleteProperty(navigator, 'clipboard');
  }
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
