import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import '../../../app.css';
import MarkdownEditor from './MarkdownEditor.svelte';
import type {
  ComposerEditorApi,
  ComposerFormattingState,
  ComposerIndentState
} from './editorTypes';

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

function dispatchEditorKey(target: Element, key: string, options: KeyboardEventInit = {}) {
  target.dispatchEvent(
    new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...options })
  );
}

function selectCurrentLine(target: Element) {
  dispatchEditorKey(target, 'Home', { shiftKey: true });
}

function pressPrimaryModifierKey(target: Element, key: string) {
  dispatchEditorKey(
    target,
    key,
    navigator.platform.startsWith('Mac') ? { metaKey: true } : { ctrlKey: true }
  );
}

function pasteText(target: Element, text: string) {
  const dataTransfer = new DataTransfer();
  dataTransfer.setData('text/plain', text);
  target.dispatchEvent(
    new ClipboardEvent('paste', {
      bubbles: true,
      cancelable: true,
      clipboardData: dataTransfer
    })
  );
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

  it('toggles bold and italic with the platform modifier shortcuts', async () => {
    const readyApis: ComposerEditorApi[] = [];
    await renderEditor({ onReady: (api: ComposerEditorApi) => readyApis.push(api) });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const textbox = page.getByRole('textbox', { name: 'Write Markdown' }).element();

    api.setContent('bold');
    api.focus();
    selectCurrentLine(textbox);
    pressPrimaryModifierKey(textbox, 'b');
    await vi.waitFor(() => expect(api.getText()).toBe('**bold**'));

    api.setContent('italic');
    api.focus();
    selectCurrentLine(textbox);
    pressPrimaryModifierKey(textbox, 'i');
    await vi.waitFor(() => expect(api.getText()).toBe('*italic*'));
  });

  it('wraps selected text in inline-code markers when backtick is pressed', async () => {
    const readyApis: ComposerEditorApi[] = [];
    await renderEditor({ onReady: (api: ComposerEditorApi) => readyApis.push(api) });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const textbox = page.getByRole('textbox', { name: 'Write Markdown' }).element();

    api.setContent('moo');
    api.focus();
    selectCurrentLine(textbox);
    dispatchEditorKey(textbox, '`');

    await vi.waitFor(() => expect(api.getText()).toBe('`moo`'));

    api.setContent('moo');
    api.focus();
    await userEvent.keyboard('`');
    await vi.waitFor(() => expect(api.getText()).toBe('moo`'));
  });

  it('turns selected text into a Markdown link when a URL is pasted over it', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const onPaste = vi.fn(() => false);
    await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api),
      onPaste
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const textbox = page.getByRole('textbox', { name: 'Write Markdown' }).element();

    api.setContent('Chatto docs');
    api.focus();
    selectCurrentLine(textbox);
    pasteText(textbox, 'https://chatto.dev/docs');

    await vi.waitFor(() =>
      expect(api.getText()).toBe('[Chatto docs](https://chatto.dev/docs)')
    );
    expect(onPaste).toHaveBeenCalledOnce();
  });

  it('performs Markdown list continuation through its normal Enter API', async () => {
    const readyApis: ComposerEditorApi[] = [];
    await renderEditor({ onReady: (api: ComposerEditorApi) => readyApis.push(api) });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('- first');
    api.performEnter();

    expect(api.getText()).toBe('- first\n- ');
  });

  it('uses CodeMirror line indentation through the shared API', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const indentation: ComposerIndentState[] = [];
    await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api),
      onIndentStateChange: (state: ComposerIndentState) => indentation.push(state)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('first\nsecond');

    expect(indentation.at(-1)).toEqual({ canIndent: true, canOutdent: true });
    expect(api.adjustIndent('indent')).toBe(true);
    expect(api.getText()).toBe('first\n  second');
    expect(api.adjustIndent('outdent')).toBe(true);
    expect(api.getText()).toBe('first\nsecond');
    expect(api.adjustIndent('outdent')).toBe(true);
    expect(api.getText()).toBe('first\nsecond');
  });

  it('lets Escape followed by Tab move focus out of the editor', async () => {
    const { container } = await renderEditor();
    const afterEditor = document.createElement('button');
    afterEditor.textContent = 'After editor';
    container.append(afterEditor);

    await userEvent.click(page.getByRole('textbox', { name: 'Write Markdown' }));
    await userEvent.keyboard('{Escape}{Tab}');

    expect(document.activeElement).toBe(afterEditor);
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

    api.focus();
    await vi.waitFor(() => expect(container.querySelector('.cm-cursor')).toBeTruthy());
    expect(getComputedStyle(container.querySelector('.cm-cursor')!).borderLeftColor).toBe(
      getComputedStyle(content!).color
    );
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
    expect(container.querySelector('.cm-code-fence-open')?.textContent).toBe('```js');
    expect(container.querySelector('.cm-code-fence-body')?.textContent).toBe(
      'const answer = "yes";'
    );
    expect(container.querySelector('.cm-code-fence-close')?.textContent).toBe('```');
    const keyword = container.querySelector('.hljs-keyword')!;
    const keywordText = keyword.querySelector('span')!;
    expect(getComputedStyle(keyword).fontFamily.toLowerCase()).toContain('plex mono');
    expect(getComputedStyle(keywordText).color).toBe(getComputedStyle(keyword).color);
    expect(getComputedStyle(keywordText).backgroundColor).toBe('rgba(0, 0, 0, 0)');

    readyApis[0]!.setContent('```js\nconst unfinished = true;');
    await vi.waitFor(() =>
      expect(container.querySelectorAll('.cm-code-fence-body')).toHaveLength(1)
    );
    expect(container.querySelector('.cm-code-fence-body')?.textContent).toBe(
      'const unfinished = true;'
    );
    expect(container.querySelector('.cm-code-fence-close')).toBeNull();
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
