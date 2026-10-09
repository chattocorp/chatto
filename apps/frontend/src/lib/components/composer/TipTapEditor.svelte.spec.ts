import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { describe, expect, it, vi } from 'vitest';
import '../../../app.css';
import TipTapEditor from './TipTapEditor.svelte';
import MarkdownEditor from './MarkdownEditor.svelte';
import type { ComposerEditorApi } from './editorTypes';
import { renderInlineMarkdown } from '$lib/markdown';

describe('Composer footnote editing', () => {
  it.each([
    { content: '- First item\n    - Second item', expected: '- Edited First item' },
    { content: '> Quoted text', expected: '> Edited Quoted text' },
    {
      content: '| Name | Value |\n    | --- | --- |\n    | Answer | 42 |',
      expected: 'Edited '
    }
  ])('focuses editable text without replacing a note that starts with $content', async (note) => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Footnote editor',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent(`Message[^note].\n\n[^note]: ${note.content}`);
    await userEvent.click(container.querySelector('.composer-footnote-ref')!);
    api.insertText('Edited ');
    await vi.waitFor(() => expect(updates.at(-1)).toContain(note.expected));
    expect(updates.at(-1)).toContain('Message[^note].');
    if (note.content.startsWith('|')) {
      expect(updates.at(-1)).toContain('| Answer | 42 |');
    }
  });

  it('retains footnote definitions when copying and pasting editor text', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Footnote editor',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('Copy[^note].\n\n[^note]: **Keep** this note.');
    const copiedText = api.getText();
    expect(copiedText).toContain('[^note]: **Keep** this note.');
    api.setContent('');
    api.focus('end');
    const clipboardData = new DataTransfer();
    clipboardData.setData('text/plain', copiedText);
    page
      .getByRole('textbox', { name: 'Footnote editor' })
      .element()
      .dispatchEvent(
        new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData })
      );
    await vi.waitFor(() => expect(updates.at(-1)).toContain('[^note]: **Keep** this note.'));
    expect(container.querySelector('.composer-footnote-ref')?.textContent).toBe('[1]');
  });
  it.each([TipTapEditor, MarkdownEditor])(
    'inserts and edits a footnote through the shared toolbar command (%#)',
    async (Component) => {
      const readyApis: ComposerEditorApi[] = [];
      const updates: string[] = [];
      const { container } = render(Component, {
        props: {
          placeholder: 'Footnote editor',
          onReady: (api: ComposerEditorApi) => readyApis.push(api),
          onUpdate: (markdown: string) => updates.push(markdown)
        }
      });
      await vi.waitFor(() => expect(readyApis).toHaveLength(1));
      const api = readyApis[0]!;
      api.setContent('A message.');
      api.focus('end');
      api.toggleFormatting('footnote');
      api.insertText('A short note.');
      await vi.waitFor(() => {
        expect(updates.at(-1)).toContain('A message.[^1]');
        expect(updates.at(-1)).toContain('[^1]: A short note.');
      });
      if (Component === TipTapEditor) {
        expect(container.querySelector('.composer-footnote-ref')?.textContent).toBe('[1]');
        expect(container.querySelector('.composer-footnote')?.textContent).toBe('A short note.');
      }
    }
  );

  it.each(['mouse', 'keyboard'])(
    'loads repeated references and lets a marker focus its editable note with %s',
    async (navigation) => {
      const readyApis: ComposerEditorApi[] = [];
      const updates: string[] = [];
      const { container } = render(TipTapEditor, {
        props: {
          placeholder: 'Footnote editor',
          onReady: (api: ComposerEditorApi) => readyApis.push(api),
          onUpdate: (markdown: string) => updates.push(markdown)
        }
      });
      await vi.waitFor(() => expect(readyApis).toHaveLength(1));
      const api = readyApis[0]!;
      api.setContent('One[^one], two[^two], one again[^one].\n\n[^two]: Second.\n[^one]: First.');
      await vi.waitFor(() =>
        expect(container.querySelectorAll('.composer-footnote')).toHaveLength(2)
      );
      const references = container.querySelectorAll('.composer-footnote-ref');
      expect([...references].map((node) => node.textContent)).toEqual(['[1]', '[2]', '[1]']);
      if (navigation === 'mouse') {
        await userEvent.click(references[2]!);
      } else {
        (references[2] as HTMLElement).focus();
        await userEvent.keyboard('{Enter}');
      }
      api.insertText('Changed ');
      await vi.waitFor(() => expect(updates.at(-1)).toContain('[^one]: Changed First.'));
      expect(updates.at(-1)).toContain('one again[^one]');
      expect(updates.at(-1)).toContain('[^two]: Second.');
    }
  );

  it('undoes marker insertion and its new definition together', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Footnote editor',
        onReady: (api: ComposerEditorApi) => readyApis.push(api)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('Message.');
    api.focus('end');
    api.toggleFormatting('footnote');
    await expect.element(page.getByRole('textbox', { name: 'Footnote editor' })).toHaveFocus();
    await userEvent.keyboard(
      navigator.platform.startsWith('Mac') ? '{Meta>}z{/Meta}' : '{Control>}z{/Control}'
    );
    await vi.waitFor(() => expect(container.querySelector('.composer-footnote')).toBeNull());
    expect(container.querySelector('.composer-footnote-ref')).toBeNull();
    expect(api.getText()).toBe('Message.');
  });

  it('creates a note from typed syntax and renumbers later references', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Footnote editor',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('Existing[^old].\n\n[^old]: Old note.');
    api.focus('start');
    const editor = page.getByRole('textbox', { name: 'Footnote editor' });
    await userEvent.type(editor, 'New[[^new]');
    await userEvent.keyboard('New note.');
    await vi.waitFor(() => expect(updates.at(-1)).toContain('[^new]: New note.'));
    expect(
      [...container.querySelectorAll('.composer-footnote-ref')].map((node) => node.textContent)
    ).toEqual(['[1]', '[2]']);
    expect(
      [...container.querySelectorAll('.composer-footnote')].map((node) =>
        node.getAttribute('data-footnote-number')
      )
    ).toEqual(['1', '2']);
  });
});

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

describe('Composer editor focus', () => {
  it.each([TipTapEditor, MarkdownEditor])(
    'preserves selection on API focus and supports explicit positions (%#)',
    async (Component) => {
      const readyApis: ComposerEditorApi[] = [];
      const { container } = render(Component, {
        props: {
          placeholder: 'Focus test',
          onReady: (api: ComposerEditorApi) => readyApis.push(api)
        }
      });
      await vi.waitFor(() => expect(readyApis).toHaveLength(1));
      const api = readyApis[0]!;
      const editor = page.getByRole('textbox', { name: 'Focus test' }).element();
      api.setContent('First paragraph');
      api.focus('start');
      // TipTap applies DOM focus on the next animation frame, after its selection changes.
      await expect.element(editor).toHaveFocus();
      await expect.poll(() => api.getTextBeforeCursor()).toBe('');
      await userEvent.keyboard(
        '{ArrowRight}{ArrowRight}{ArrowRight}{Shift>}{ArrowRight}{ArrowRight}{/Shift}'
      );
      expect(window.getSelection()?.toString()).toBe('st');
      const outside = document.createElement('button');
      outside.textContent = 'Outside';
      container.append(outside);
      await userEvent.click(outside);
      api.focus();
      await expect.element(editor).toHaveFocus();
      await expect.poll(() => window.getSelection()?.toString()).toBe('st');
      await userEvent.keyboard('X');
      expect(api.getText()).toBe('FirX paragraph');
      api.focus('end');
      await expect.poll(() => api.getTextBeforeCursor()).toBe('FirX paragraph');
    }
  );
});

describe('TipTapEditor accessibility', () => {
  it('keeps its accessible name synchronized with the placeholder', async () => {
    const rendered = render(TipTapEditor, { props: { placeholder: 'Write a message' } });

    const editor = page.getByRole('textbox', { name: 'Write a message' });
    await expect.element(editor).toBeVisible();
    // An aria-label on a contenteditable div needs an explicit role.
    await expect.element(editor).toHaveAttribute('role', 'textbox');
    await expect.element(editor).toHaveAttribute('aria-multiline', 'true');

    await rendered.rerender({ placeholder: 'Edit your message' });

    await expect.element(page.getByRole('textbox', { name: 'Edit your message' })).toBeVisible();
  });

  it('shows a text caret when focused inside the non-selectable app shell', async () => {
    const { container } = render(TipTapEditor, {
      props: { placeholder: 'Write a message', autofocus: true }
    });
    const editor = page.getByRole('textbox', { name: 'Write a message' }).element();

    await vi.waitFor(() => expect(document.activeElement).toBe(editor));

    const style = getComputedStyle(editor);
    expect(style.userSelect).toBe('text');
    expect(style.caretColor).toBe(style.color);
    expect(container.querySelector('.tiptap-editor')?.classList).toContain('select-text');
  });
});

describe('TipTapEditor literal Markdown text', () => {
  it('preserves typed mentions and backslashes as source text', async () => {
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    const editor = page.getByRole('textbox', { name: 'Write a message' });

    await userEvent.click(editor);
    await userEvent.type(editor, '@chatto_bot \\o/');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('@chatto_bot \\o/'));
  });

  it.each(['@chatto_bot', '\\o/', 'C:\\Users\\foo', '¯\\_(ツ)_/¯'])(
    'preserves %s when restoring and editing a draft',
    async (source) => {
      const readyApis: ComposerEditorApi[] = [];
      const updates: string[] = [];
      render(TipTapEditor, {
        props: {
          placeholder: 'Write a message',
          onReady: (api: ComposerEditorApi) => readyApis.push(api),
          onUpdate: (markdown: string) => updates.push(markdown)
        }
      });
      await vi.waitFor(() => expect(readyApis).toHaveLength(1));
      const api = readyApis[0]!;

      api.setContent(source);
      expect(api.getText()).toBe(source);
      api.focus('end');
      api.insertText('!');

      await vi.waitFor(() => expect(updates.at(-1)).toBe(`${source}!`));
      expect(renderInlineMarkdown(updates.at(-1)!)).toContain(`${source}!`);
    }
  );

  it('keeps literal emphasis markers visible without creating formatting', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;

    api.insertText('*literal*');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('&#42;literal&#42;'));
    expect(renderInlineMarkdown(updates.at(-1)!)).toContain('*literal*');
    expect(renderInlineMarkdown(updates.at(-1)!)).not.toContain('<em>');
    expect(container.querySelector('em')).toBeNull();
    api.setContent(updates.at(-1)!);
    expect(api.getText()).toBe('*literal*');
  });

  it('keeps literal asterisks inside words from becoming emphasis', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;

    api.insertText('foo*bar*');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('foo&#42;bar&#42;'));
    expect(renderInlineMarkdown(updates.at(-1)!)).not.toContain('<em>');
  });

  it('keeps formatting and code while normalizing adjacent literal text', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const source = '**bold** `code_with_underscore` @chatto_bot \\o/';

    api.setContent(source);
    api.focus('end');
    api.insertText('!');

    await vi.waitFor(() => expect(updates.at(-1)).toBe(`${source}!`));
    const html = renderInlineMarkdown(updates.at(-1)!);
    expect(html).toContain('<strong>bold</strong>');
    expect(html).toContain('@chatto_bot \\o/');
    expect(html).toContain('<code>code_with_underscore</code>');
  });

  it('preserves a Markdown link with underscores in its label and URL', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    const source = '[chatto_bot](https://example.com/chatto_bot)';

    api.setContent(source);
    api.focus('end');
    api.insertText(' after');

    await vi.waitFor(() => expect(updates.at(-1)).toBe(`${source} after`));
    expect(renderInlineMarkdown(updates.at(-1)!)).toContain(
      'href="https://example.com/chatto_bot"'
    );
  });

  it('preserves literal text pasted into the Visual editor', async () => {
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    const editor = page.getByRole('textbox', { name: 'Write a message' }).element();
    const dataTransfer = new DataTransfer();
    dataTransfer.setData('text/plain', '@chatto_bot \\o/');

    editor.dispatchEvent(
      new ClipboardEvent('paste', {
        bubbles: true,
        cancelable: true,
        clipboardData: dataTransfer
      })
    );

    await vi.waitFor(() => expect(updates.at(-1)).toBe('@chatto_bot \\o/'));
  });
});

describe('TipTapEditor editability', () => {
  it('does not emit content changes when disabled or enabled', async () => {
    const onUpdate = vi.fn();
    const rendered = render(TipTapEditor, {
      props: { placeholder: 'Write a bio', onUpdate }
    });
    const editor = page.getByRole('textbox', { name: 'Write a bio' }).element();
    await expect.element(editor).toHaveAttribute('contenteditable', 'true');

    await rendered.rerender({ editable: false });
    await expect.element(editor).toHaveAttribute('contenteditable', 'false');
    expect(onUpdate).not.toHaveBeenCalled();

    await rendered.rerender({ editable: true });
    await expect.element(editor).toHaveAttribute('contenteditable', 'true');
    expect(onUpdate).not.toHaveBeenCalled();
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
    api.focus('end');
    await expect.element(editor).toHaveFocus();
    selectEditorContents(editor);
    editor.dispatchEvent(
      new KeyboardEvent('keydown', { key: '`', bubbles: true, cancelable: true })
    );

    await vi.waitFor(() => expect(container.querySelector('code')?.textContent).toBe('moo'));

    api.setContent('moo');
    api.focus('end');
    await expect.element(editor).toHaveFocus();
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

describe('TipTapEditor Markdown autolinks', () => {
  it('preserves underscores in restored Markdown autolinks', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;
    api.setContent('<https://example.com/chatto_bot>');
    api.focus('end');
    api.insertText(' after');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('<https://example.com/chatto_bot> after'));
    expect(renderInlineMarkdown(updates.at(-1)!)).toContain(
      'href="https://example.com/chatto_bot"'
    );
  });

  it('preserves a restored angle-bracket autolink after a later edit', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;

    api.setContent('<https://example.com/?a=1&b=2>');
    api.focus('end');
    api.insertText(' after');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('<https://example.com/?a=1&b=2> after'));
  });

  it('converts a typed angle-bracket URL into a preserved Markdown autolink', async () => {
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    const editor = page.getByRole('textbox', { name: 'Write a message' });

    await userEvent.click(editor);
    await userEvent.type(editor, '<https://example.com/story>');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('<https://example.com/story>'));
    await expect.element(editor).toHaveTextContent('https://example.com/story');
  });

  it('preserves a typed autolink when the closing angle bracket is missing', async () => {
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    const editor = page.getByRole('textbox', { name: 'Write a message' });

    await userEvent.click(editor);
    await userEvent.type(editor, '<https://example.com/unclosed');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('<https://example.com/unclosed'));
  });

  it('preserves a restored autolink with no closing angle bracket', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    const { container } = render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;

    api.setContent('<https://example.com/unclosed');
    api.focus('end');
    api.insertText(' after');

    await vi.waitFor(() => expect(updates.at(-1)).toBe('<https://example.com/unclosed after'));
    expect(container.querySelector('a')?.getAttribute('href')).toBe('https://example.com/unclosed');
  });

  it('preserves an angle-bracket URL pasted into the visual editor', async () => {
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    const editor = page.getByRole('textbox', { name: 'Write a message' }).element();
    const dataTransfer = new DataTransfer();
    dataTransfer.setData('text/plain', '<https://example.com/pasted>');

    editor.dispatchEvent(
      new ClipboardEvent('paste', {
        bubbles: true,
        cancelable: true,
        clipboardData: dataTransfer
      })
    );

    await vi.waitFor(() => expect(updates.at(-1)).toBe('<https://example.com/pasted>'));
    expect(editor.querySelector('a')?.getAttribute('href')).toBe('https://example.com/pasted');
  });

  it('uses a regular Markdown link after its destination changes', async () => {
    const readyApis: ComposerEditorApi[] = [];
    const updates: string[] = [];
    render(TipTapEditor, {
      props: {
        placeholder: 'Write a message',
        onReady: (api: ComposerEditorApi) => readyApis.push(api),
        onUpdate: (markdown: string) => updates.push(markdown)
      }
    });
    await vi.waitFor(() => expect(readyApis).toHaveLength(1));
    const api = readyApis[0]!;

    api.setContent('<https://example.com/original>');
    api.focus('end');
    const linkInput = page.getByRole('textbox', { name: 'Link URL' });
    await expect.element(linkInput).toBeVisible();
    await userEvent.fill(linkInput, 'https://example.com/changed');
    await userEvent.tab();

    await vi.waitFor(() =>
      expect(updates.at(-1)).toBe('[https://example.com/original](https://example.com/changed)')
    );
  });
});
