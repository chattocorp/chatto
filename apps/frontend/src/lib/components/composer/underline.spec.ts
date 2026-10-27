import { MarkdownManager } from '@tiptap/markdown';
import { getSchema } from '@tiptap/core';
import { EditorState } from '@tiptap/pm/state';
import { describe, expect, it } from 'vitest';
import { renderInlineMarkdown, renderMarkdown } from '$lib/markdown';
import { createComposerExtensions } from './extensions';
import { getSerializedMarkdown, parseMarkdownForEditor } from './markdown';

describe('underline Markdown', () => {
  it.each([
    ['__underlined__', '<u>underlined</u>'],
    ['__**bold** and *italic*__', '<u><strong>bold</strong> and <em>italic</em></u>'],
    ['**__both__**', '<strong><u>both</u></strong>'],
    ['___both___', '<em><u>both</u></em>'],
    ['[__label__](https://example.com/__path__)', '<u>label</u>']
  ])('renders and round-trips %s', async (source, expected) => {
    expect(await renderMarkdown(source)).toContain(expected);
    expect(renderInlineMarkdown(source)).toContain(expected);
    const extensions = createComposerExtensions('');
    const manager = new MarkdownManager({ extensions });
    const json = parseMarkdownForEditor(source, (text) => manager.parse(text));
    const doc = getSchema(extensions).nodeFromJSON(json);
    doc.check();
    const serialized = getSerializedMarkdown({
      state: EditorState.create({ doc }),
      getMarkdown: () => manager.serialize(json)
    });
    expect(await renderMarkdown(serialized)).toContain(expected);
    expect(serialized).toContain('__');
  });

  it('keeps inline code literal inside an underline span in the visual editor', () => {
    const extensions = createComposerExtensions('');
    const manager = new MarkdownManager({ extensions });
    const json = parseMarkdownForEditor('__before `__code__` after__', (text) =>
      manager.parse(text)
    );
    const doc = getSchema(extensions).nodeFromJSON(json);
    doc.check();
    expect(json.content?.[0].content).toEqual([
      { type: 'text', text: 'before ', marks: [{ type: 'underline' }] },
      { type: 'text', text: '__code__', marks: [{ type: 'code' }] },
      { type: 'text', text: ' after', marks: [{ type: 'underline' }] }
    ]);
  });

  it.each([
    '++literal++',
    '`__code__`',
    '``__code with ` backtick__``',
    '```text\n__code__\n```',
    '~~~\n__code__\n~~~',
    '    __code__',
    'word__inside__word',
    '__ unfinished',
    '&#95;&#95;literal&#95;&#95;',
    '[label](https://example.com/__path__)',
    '<https://example.com/__path__>'
  ])('leaves code and non-formatting underscores unchanged: %s', async (source) => {
    expect(await renderMarkdown(source)).not.toContain('<u>');
    const extensions = createComposerExtensions('');
    const manager = new MarkdownManager({ extensions });
    const json = parseMarkdownForEditor(source, (text) => manager.parse(text));
    expect(JSON.stringify(json)).not.toContain('"type":"underline"');
  });

  it('supports underline in quotes, lists, tables, and footnotes', async () => {
    const html = await renderMarkdown(
      '> __quote__\n\n- __item__\n\n| Column |\n| --- |\n| __cell__ |\n\nNote[^1].\n\n[^1]: __note__'
    );
    for (const text of ['quote', 'item', 'cell', 'note']) {
      expect(html).toContain(`<u>${text}</u>`);
    }
  });
});
