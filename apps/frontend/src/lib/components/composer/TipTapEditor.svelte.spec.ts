import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import '../../../app.css';
import TipTapEditor from './TipTapEditor.svelte';
import type { ComposerEditorApi } from './editorTypes';

function selectEditorContents(editor: Element) {
  editor.dispatchEvent(
    new KeyboardEvent('keydown', {
      key: 'a',
      bubbles: true,
      cancelable: true,
      ...(navigator.platform.startsWith('Mac') ? { metaKey: true } : { ctrlKey: true })
    })
  );
}

describe('TipTapEditor accessibility', () => {
  it('keeps its accessible name synchronized with the placeholder', async () => {
    const rendered = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    await rendered.rerender({ placeholder: 'Edit your message' });

    await expect.element(page.getByRole('textbox', { name: 'Edit your message' })).toBeVisible();
  });
});

describe('TipTapEditor wrapping', () => {
  it('formats selected text as inline code when backtick is pressed', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const editor = page.getByRole('textbox', { name: 'Write a message' }).element();

    api.setContent('moo');
    api.focus();
    selectEditorContents(editor);
    editor.dispatchEvent(
      new KeyboardEvent('keydown', { key: '`', bubbles: true, cancelable: true })
    );

    await vi.waitFor(() => expect(container.querySelector('code')?.textContent).toBe('moo'));

    api.setContent('moo');
    api.focus();
    await userEvent.keyboard('`');
    await vi.waitFor(() => expect(editor.textContent).toBe('moo`'));
    expect(container.querySelector('code')).toBeNull();
  });

  it('performs the normal structural Enter action through its API', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const onKeyDown = vi.fn(() => true);
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onKeyDown,
        onReady: (api: ComposerEditorApi) => readyApis.push(api)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('- first');
    api.focus('end');
    api.performEnter();

    await vi.waitFor(() =>
      expect(container.querySelectorAll('.ProseMirror ul li')).toHaveLength(2)
    );
    expect(onKeyDown).not.toHaveBeenCalled();
  });

  it('indents and outdents list items through the shared API', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const indentation: { canIndent: boolean; canOutdent: boolean }[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onIndentStateChange: (state) => indentation.push(state)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('- first\n- second');
    api.focus('end');
    await vi.waitFor(() => expect(indentation.at(-1)?.canIndent).toBe(true));

    expect(api.adjustIndent('indent')).toBe(true);
    await vi.waitFor(() =>
      expect(container.querySelectorAll('.ProseMirror ul ul li')).toHaveLength(1)
    );
    expect(indentation.at(-1)).toEqual({ canIndent: false, canOutdent: true });

    expect(api.adjustIndent('outdent')).toBe(true);
    await vi.waitFor(() =>
      expect(container.querySelectorAll('.ProseMirror > ul > li')).toHaveLength(2)
    );
  });

  it('uses stable wrapping instead of global prose wrapping', async () => {
    const { container } = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    const paragraph = container.querySelector('.ProseMirror p');
    expect(paragraph).toBeInstanceOf(HTMLParagraphElement);
    expect(getComputedStyle(paragraph!).textWrap).toBe('wrap');
  });

  it('uses logical prose edges and isolates code as LTR', async () => {
    const { container } = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    const editor = container.querySelector('.ProseMirror');
    expect(editor).toBeInstanceOf(HTMLElement);
    if (!editor) return;

    const quote = document.createElement('blockquote');
    quote.textContent = 'مرحبا';
    const code = document.createElement('pre');
    code.textContent = 'const direction = "ltr";';
    const inlineCode = document.createElement('code');
    inlineCode.textContent = 'const direction = "ltr";';
    editor.append(quote, code, inlineCode);

    const quoteStyle = getComputedStyle(quote);
    expect(quoteStyle.borderInlineStartWidth).toBe('3px');
    expect(quoteStyle.paddingInlineStart).toBe('14.4px');
    expect(quoteStyle.unicodeBidi).toBe('plaintext');
    expect(getComputedStyle(code).direction).toBe('ltr');
    expect(getComputedStyle(code).unicodeBidi).toBe('isolate');
    expect(getComputedStyle(inlineCode).direction).toBe('ltr');
    expect(getComputedStyle(inlineCode).unicodeBidi).toBe('isolate');
  });

  it('aligns RTL ordered-list markers toward their content without start padding', async () => {
    const { container } = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    await expect.element(page.getByRole('textbox', { name: 'Write a message' })).toBeVisible();

    const editor = container.querySelector('.ProseMirror');
    expect(editor).toBeInstanceOf(HTMLElement);
    if (!editor) return;

    const list = document.createElement('ol');
    const item = document.createElement('li');
    item.textContent = 'العنصر الأول';
    list.append(item);
    editor.setAttribute('dir', 'rtl');
    editor.append(list);

    expect(getComputedStyle(list).paddingInlineStart).toBe('0px');
    expect(getComputedStyle(item, '::before').textAlign).toBe('end');
  });
});
