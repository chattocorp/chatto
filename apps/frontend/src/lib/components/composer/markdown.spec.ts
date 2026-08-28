import { describe, expect, it } from 'vitest';
import { Schema } from '@tiptap/pm/model';
import {
  buildQuoteContent,
  createClipboardContent,
  isHttpMarkdownAutolink,
  prepareMarkdownForEditor
} from './markdown';

describe('isHttpMarkdownAutolink', () => {
  it('accepts only complete angle-bracket HTTP(S) URLs', () => {
    expect(isHttpMarkdownAutolink('<https://example.com/story>')).toBe(true);
    expect(isHttpMarkdownAutolink('<HTTP://example.com/story>')).toBe(true);
    expect(isHttpMarkdownAutolink('<example.com>')).toBe(false);
    expect(isHttpMarkdownAutolink('<https://example.com/story')).toBe(false);
    expect(isHttpMarkdownAutolink('See <https://example.com/story>')).toBe(false);
  });
});

describe('prepareMarkdownForEditor', () => {
  it('escapes HTML-looking prose without changing link destinations', () => {
    expect(
      prepareMarkdownForEditor(
        'Use <section> & [open](https://example.com/?first=1&second=2) safely'
      )
    ).toBe('Use &lt;section> &amp; [open](https://example.com/?first=1&second=2) safely');
  });

  it('preserves HTML-looking text inside inline and fenced code', () => {
    const markdown = ['`<inline> & value`', '', '```html', '<section> & value', '```'].join('\n');

    expect(prepareMarkdownForEditor(markdown)).toBe(markdown);
  });

  it('keeps GFM table source editable without changing its visible text', () => {
    const prepared = prepareMarkdownForEditor('| One | Two |\n| --- | --- |\n| A | B |');

    expect(prepared).toBe('| One | Two |\n| -\u2060-- | --- |\n| A | B |');
  });
});

describe('createClipboardContent', () => {
  it('preserves destination marks while converting Markdown links and line breaks', () => {
    const schema = new Schema({
      nodes: {
        doc: { content: 'block+' },
        paragraph: { content: 'inline*', group: 'block' },
        text: { group: 'inline' },
        hardBreak: { inline: true, group: 'inline' }
      },
      marks: {
        strong: {},
        link: { attrs: { href: {} }, inclusive: false }
      }
    });
    const strong = schema.marks.strong.create();

    const content = createClipboardContent(
      'Before [Chatto](https://chatto.dev)\nafter\n\nSecond',
      schema,
      [strong]
    );

    expect(content.toJSON()).toEqual([
      {
        type: 'paragraph',
        content: [
          { type: 'text', marks: [{ type: 'strong' }], text: 'Before ' },
          {
            type: 'text',
            marks: [{ type: 'strong' }, { type: 'link', attrs: { href: 'https://chatto.dev' } }],
            text: 'Chatto'
          },
          { type: 'hardBreak' },
          { type: 'text', marks: [{ type: 'strong' }], text: 'after' }
        ]
      },
      {
        type: 'paragraph',
        content: [{ type: 'text', marks: [{ type: 'strong' }], text: 'Second' }]
      }
    ]);
  });
});

describe('buildQuoteContent', () => {
  it('builds nested TipTap blockquote content without flattening line breaks', () => {
    expect(
      buildQuoteContent([
        { quoteDepth: 0, text: 'Root' },
        { quoteDepth: 1, text: 'Nested\nline' },
        { quoteDepth: 2, text: 'Deep' },
        { quoteDepth: 0, text: 'Tail' }
      ])
    ).toEqual([
      {
        type: 'paragraph',
        content: [{ type: 'text', text: 'Root' }]
      },
      {
        type: 'blockquote',
        content: [
          {
            type: 'paragraph',
            content: [{ type: 'text', text: 'Nested' }]
          },
          {
            type: 'paragraph',
            content: [{ type: 'text', text: 'line' }]
          },
          {
            type: 'blockquote',
            content: [
              {
                type: 'paragraph',
                content: [{ type: 'text', text: 'Deep' }]
              }
            ]
          }
        ]
      },
      {
        type: 'paragraph',
        content: [{ type: 'text', text: 'Tail' }]
      }
    ]);
  });
});
