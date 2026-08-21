import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import '../../../app.css';
import MarkdownEditor from './MarkdownEditor.svelte';
import type { ComposerEditorApi, ComposerFormattingState } from './editorTypes';

async function renderEditor(props: Record<string, unknown> = {}) {
  const rendered = render(MarkdownEditor, {
    props: {
      placeholder: 'Write Markdown',
      ...props
    }
  });
  await expect.element(page.getByRole('textbox', { name: 'Write Markdown' })).toBeVisible();
  return rendered;
}

describe('MarkdownEditor', () => {
  it('synchronizes its accessible name, placeholder, and disabled state', async () => {
    const rendered = await renderEditor();
    const textbox = page.getByRole('textbox', { name: 'Write Markdown' });
    await expect.element(textbox).toHaveAttribute('aria-multiline', 'true');
    await expect.element(textbox).toHaveAttribute('contenteditable', 'true');

    await rendered.rerender({ placeholder: 'Edit Markdown', editable: false });

    const disabledTextbox = page.getByRole('textbox', { name: 'Edit Markdown' });
    await expect.element(disabledTextbox).toHaveAttribute('contenteditable', 'false');
  });

  it('updates source through cursor replacement, insertion, and quote serialization', async () => {
    const updates: string[] = [];
    const readyApis: ComposerEditorApi[] = [];
    await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api),
      onUpdate: (source: string) => updates.push(source)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('@ali');
    api.replaceTextBeforeCursor(4, '@alice ');
    api.insertText(':wave: ');
    api.insertQuote([{ quoteDepth: 1, text: 'nested' }]);

    expect(api.getText()).toBe('@alice :wave: \n\n> > nested\n\n');
    expect(api.getTextBeforeCursor()).toBe(api.getText());
    expect(updates.at(-1)).toBe(api.getText());
  });

  it('applies block formatting and reports the active source syntax', async () => {
    const formatting: ComposerFormattingState[] = [];
    const readyApis: ComposerEditorApi[] = [];
    await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api),
      onFormattingStateChange: (state: ComposerFormattingState) => formatting.push(state)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('first');
    api.toggleFormatting('bulletList');
    expect(api.getText()).toBe('- first');
    expect(formatting.at(-1)?.bulletList).toBe(true);
  });

  it('uses the visual editor font at 16px with per-line bidirectional text', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const { container } = await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('English\nمرحبا');

    const content = container.querySelector('.cm-content');
    expect(content).toBeInstanceOf(HTMLElement);
    expect(getComputedStyle(content!).fontSize).toBe('16px');
    expect(getComputedStyle(content!).fontFamily.toLowerCase()).toContain('plex sans');
    await vi.waitFor(() => expect(container.querySelectorAll('.cm-line')).toHaveLength(2));
  });

  it('highlights programming syntax inside labelled code fences', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const { container } = await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    readyApis[0]!.setContent('```js\nconst answer = "yes";\n```');

    await vi.waitFor(() => expect(container.querySelector('.hljs-keyword')).toBeTruthy());
    expect(container.querySelector('.hljs-keyword')?.textContent).toBe('const');
    expect(container.querySelector('.hljs-string')?.textContent).toBe('"yes"');
    expect(
      getComputedStyle(container.querySelector('.hljs-keyword')!).fontFamily.toLowerCase()
    ).toContain('plex mono');
  });

  it('fences stale APIs after destruction', async () => {
    const onDestroy = vi.fn();
    const readyApis: ComposerEditorApi[] = [];
    const rendered = await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api),
      onDestroy
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    rendered.unmount();

    expect(onDestroy).toHaveBeenCalledWith(api);
    api.insertText('ignored');
    expect(api.getText()).toBe('');
  });
});
