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
  it('focuses the end of a long draft without moving its caret', async () => {
    const readyApis: ComposerEditorApi[] = [];
    await renderEditor({ onReady: (api: ComposerEditorApi) => readyApis.push(api) });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const draft = Array.from({ length: 400 }, (_, index) => `Line ${index}`).join('\n');
    api.setContent(draft);
    api.focus('end');
    await expect.element(page.getByRole('textbox', { name: 'Write Markdown' })).toHaveFocus();
    await userEvent.keyboard('X');
    await expect.poll(() => api.getText()).toBe(`${draft}X`);
  });

  it('uses native caret and selection after refocusing', async () => {
    const { container } = await renderEditor();
    const textbox = page.getByRole('textbox', { name: 'Write Markdown' });
    const outside = document.createElement('button');
    outside.textContent = 'Outside editor';
    container.append(outside);
    await textbox.click();
    await userEvent.click(outside);
    await textbox.click();
    await expect.element(textbox).toHaveFocus();

    const content = textbox.element();
    expect(container.querySelector('.cm-cursorLayer, .cm-selectionLayer')).toBeNull();
    expect(getComputedStyle(content).caretColor).toBe(getComputedStyle(content).color);
    expect(content.contains(window.getSelection()?.anchorNode ?? null)).toBe(true);

    await userEvent.keyboard('hello');
    await userEvent.keyboard('{Shift>}{ArrowLeft}{ArrowLeft}{/Shift}');
    expect(window.getSelection()?.toString()).toBe('lo');
    await userEvent.keyboard('p');
    expect(content.textContent).toBe('help');
  });

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

  it('toggles bold, italic, and underline with the platform modifier shortcuts', async () => {
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

    api.setContent('underline');
    api.focus();
    selectCurrentLine(textbox);
    pressPrimaryModifierKey(textbox, 'u');
    await vi.waitFor(() => expect(api.getText()).toBe('__underline__'));
    pressPrimaryModifierKey(textbox, 'u');
    await vi.waitFor(() => expect(api.getText()).toBe('underline'));
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

    await vi.waitFor(() => expect(api.getText()).toBe('[Chatto docs](https://chatto.dev/docs)'));
    expect(onPaste).toHaveBeenCalledOnce();
  });

  it('does not turn selected text into a link while read-only', async () => {
    const readyApis: ComposerEditorApi[] = [];
    await renderEditor({
      readOnly: true,
      onReady: (api: ComposerEditorApi) => readyApis.push(api)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const textbox = page.getByRole('textbox', { name: 'Write Markdown' }).element();

    api.setContent('Chatto docs');
    api.focus();
    selectCurrentLine(textbox);
    pasteText(textbox, 'https://chatto.dev/docs');

    expect(api.getText()).toBe('Chatto docs');
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

  it.each([
    { name: 'empty text', text: '', select: false },
    { name: 'normal text', text: 'A draft message', select: false },
    { name: 'a list', text: '- first\n  - second', select: false },
    { name: 'fenced code', text: '```js\n  const value = 1;\n```', select: false },
    { name: 'selected text', text: 'first\nsecond', select: true }
  ])('lets Tab and Shift+Tab leave $name unchanged and move focus', async ({ text, select }) => {
    const readyApis: ComposerEditorApi[] = [];
    const { container } = await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const beforeEditor = document.createElement('button');
    beforeEditor.textContent = 'Before editor';
    const afterEditor = document.createElement('button');
    afterEditor.textContent = 'After editor';
    container.prepend(beforeEditor);
    container.append(afterEditor);
    api.setContent(text);

    for (const [key, target] of [
      ['{Tab}', afterEditor],
      ['{Shift>}{Tab}{/Shift}', beforeEditor]
    ] as const) {
      api.focus('end');
      if (text.startsWith('```')) await userEvent.keyboard('{ArrowUp}');
      if (select) {
        await userEvent.keyboard('{Shift>}{ArrowUp}{Home}{/Shift}');
        expect(window.getSelection()?.toString()).toBe(text);
      }
      await userEvent.keyboard(key);

      expect(document.activeElement).toBe(target);
      expect(api.getText()).toBe(text);
    }
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
    expect(getComputedStyle(content!).caretColor).toBe(getComputedStyle(content!).color);
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

  it('highlights Mermaid fences while preserving editable source', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const { container } = await renderEditor({
      onReady: (api: ComposerEditorApi) => readyApis.push(api)
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const source = '```mermaid rendered\nflowchart TD\n  A[Scan cargo] --> B{Ready?}\n```';
    readyApis[0].setContent(source);
    await expect
      .poll(() => container.querySelector('.hljs-keyword')?.textContent)
      .toBe('flowchart');
    expect(container.querySelector('.hljs-built_in')?.textContent).toBe('TD');
    expect(container.querySelector('.hljs-string')?.textContent).toBe('[Scan cargo]');
    expect(container.querySelector('.hljs-operator')?.textContent).toBe('-->');
    expect(readyApis[0].getText()).toBe(source);
    readyApis[0].setContent('```mermaid\nsequenceDiagram\nAlice->>Bob: Hello\n```');
    await expect
      .poll(() => container.querySelector('.hljs-keyword')?.textContent)
      .toBe('sequenceDiagram');
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

it.each([
  { text: 'foo@bar.com', linked: false },
  { text: 'ftp://example.com/file', linked: false },
  { text: 'ssh://example.com', linked: false },
  { text: 'blob:https://example.com/id', linked: false },
  { text: 'tel:+123456789', linked: false },
  { text: 'agents.md readme.md foo.py', linked: false },
  { text: 'mailto:foo@bar.com', linked: true },
  { text: 'https://example.com', linked: true },
  { text: 'www.example.com', linked: true }
])('uses prefix rules for source highlighting: $text', async ({ text, linked }) => {
  await renderEditor();
  const textbox = page.getByRole('textbox', { name: 'Write Markdown' });
  await textbox.fill(text);
  await expect
    .poll(() => {
      const spans = textbox.element().querySelectorAll('span');
      return [...spans].some((span) =>
        getComputedStyle(span).textDecorationLine.includes('underline')
      );
    })
    .toBe(linked);
});

it.each([
  { text: '[title](destination)', label: 'title' },
  { text: '[title](https://example.com)', label: 'title' },
  { text: '[title]()', label: 'title' },
  { text: '[**bold** title](destination "tooltip")', label: '**bold** title' },
  { text: '[writing in brackets]', label: '' },
  { text: '[title](destination', label: '' },
  { text: '[title](', label: '' },
  { text: '`[title](destination)`', label: '' },
  { text: '![title](destination)', label: '![title](destination)' }
])('styles only a complete inline link label: $text', async ({ text, label }) => {
  await renderEditor();
  const textbox = page.getByRole('textbox', { name: 'Write Markdown' });
  await textbox.fill(text);
  await expect
    .poll(() => {
      const walker = document.createTreeWalker(textbox.element(), NodeFilter.SHOW_TEXT);
      let styled = '';
      while (walker.nextNode()) {
        const node = walker.currentNode;
        let element = node.parentElement;
        while (element && element !== textbox.element()) {
          if (getComputedStyle(element).textDecorationLine.includes('underline')) {
            styled += node.textContent;
            break;
          }
          element = element.parentElement;
        }
      }
      return styled;
    })
    .toBe(label);
});

it('updates link styling as a link is completed and broken', async () => {
  await renderEditor();
  const textbox = page.getByRole('textbox', { name: 'Write Markdown' });
  await textbox.fill('[title](destination');
  expect(textbox.element().querySelector('.cm-link-label')).toBeNull();
  await userEvent.keyboard(')');
  await expect
    .poll(() => textbox.element().querySelector('.cm-link-label')?.textContent)
    .toBe('title');
  await userEvent.keyboard('{Backspace}');
  await expect.poll(() => textbox.element().querySelector('.cm-link-label')).toBeNull();
});
