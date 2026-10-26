import { describe, expect, it } from 'vitest';
import { MarkdownManager } from '@tiptap/markdown';
import { getSchema } from '@tiptap/core';
import { EditorState } from '@tiptap/pm/state';
import { TEXT_DECORATIONS } from '@chatto/client/messaging/textDecorations';
import { renderInlineMarkdown, renderMarkdown } from './markdown';
import { matchTextDecoration } from './textDecorations';
import { createComposerExtensions } from './components/composer/extensions';
import { getSerializedMarkdown, parseMarkdownForEditor } from './components/composer/markdown';

function roundTrip(source: string) {
  const extensions = createComposerExtensions('');
  const manager = new MarkdownManager({ extensions });
  const json = parseMarkdownForEditor(source, (text) => manager.parse(text));
  const doc = getSchema(extensions).nodeFromJSON(json);
  doc.check();
  return getSerializedMarkdown({
    state: EditorState.create({ doc }),
    getMarkdown: () => manager.serialize(json)
  });
}

describe('inline text decorations', () => {
  it('serializes visual marks across hard breaks as separate inline pairs', async () => {
    const manager = new MarkdownManager({ extensions: createComposerExtensions('') });
    const marks = [{ type: 'text-wave' }];
    const source = manager.serialize({
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: 'first', marks },
            { type: 'hardBreak', marks },
            { type: 'text', text: 'second', marks }
          ]
        }
      ]
    });
    const html = await renderMarkdown(source);
    expect(html).not.toContain('&lt;text-wave&gt;');
    expect(html.match(/class="text-decoration text-wave"/g)).toHaveLength(2);
  });
  it.each(TEXT_DECORATIONS)('renders and round-trips %s', async (tag) => {
    const source = `Before <${tag}>hello **world** 👩🏽‍🚀 مرحبا</${tag}> after`;
    const html = await renderMarkdown(source);
    expect(html).toContain(
      `<span class="text-decoration ${tag}">hello <strong>world</strong> 👩🏽‍🚀 مرحبا</span>`
    );
    expect(renderInlineMarkdown(source)).toBe(html.trim().slice(3, -4));
    expect(await renderMarkdown(roundTrip(source))).toBe(html);
  });

  it.each([
    '<script>alert(1)</script>',
    '<color-blue onclick="alert(1)">no</color-blue>',
    '<color-blue style="color:red">no</color-blue>',
    '<color-blue/>',
    '<color-blue>unclosed',
    '<COLOR-BLUE>no</COLOR-BLUE>',
    '<color-ultraviolet>no</color-ultraviolet>',
    '<text-wave>two\nlines</text-wave>',
    '<text-wave>two\n\nparagraphs</text-wave>',
    '&lt;text-wave&gt;literal&lt;/text-wave&gt;',
    '`<text-wave>literal</text-wave>`',
    '```html\n<text-wave>literal</text-wave>\n```',
    '    <text-wave>literal</text-wave>',
    '<text-wave><color-blue>crossed</text-wave></color-blue>'
  ])('keeps unsupported or literal syntax inert: %s', async (source) => {
    const html = await renderMarkdown(source);
    expect(html).not.toContain('class="text-decoration');
    expect(html).not.toContain('<script');
    expect(await renderMarkdown(roundTrip(source))).not.toContain('class="text-decoration');
  });

  it('supports colors around effects, links, lists, quotes and table cells', async () => {
    const span = '<color-blue><text-glow>hello</text-glow></color-blue>';
    const html = await renderMarkdown(`> ${span}\n\n- ${span}\n\n| Header |\n| --- |\n| ${span} |`);
    expect(html.match(/class="text-decoration color-blue"/g)).toHaveLength(3);
    expect(await renderMarkdown(roundTrip(span))).toBe(await renderMarkdown(span));
    const link = '<text-glow>[hello](https://example.com)</text-glow>';
    const serialized = await renderMarkdown(roundTrip(link));
    expect(serialized).toContain('href="https://example.com"');
    expect(serialized).toContain('<span class="text-decoration text-glow">hello</span>');
  });

  it('does not use closing tags in code or link destinations', async () => {
    const source = '<text-wave>before `</text-wave>` after</text-wave>';
    expect(await renderMarkdown(source)).toContain('<code>&lt;/text-wave&gt;</code> after</span>');
    expect(
      matchTextDecoration('<text-wave>[link](https://example.com/</text-wave>)')
    ).toBeUndefined();
    expect(matchTextDecoration('<text-wave>``code ` </text-wave>``')).toBeUndefined();
  });

  it('bounds nesting and keeps malformed input local to its line', async () => {
    const nested = '<text-wave>'.repeat(5) + 'hello' + '</text-wave>'.repeat(5);
    expect(matchTextDecoration(nested)).toBeUndefined();
    const html = await renderMarkdown(nested);
    expect(html.match(/class="text-decoration/g)?.length).toBeLessThanOrEqual(4);
    expect(await renderMarkdown('<text-wave>broken\n<color-green>good</color-green>')).toContain(
      '<span class="text-decoration color-green">good</span>'
    );
  });

  it('does not generate active user HTML inside an effect', async () => {
    const html = await renderMarkdown('<text-glow><img src=x onerror=alert(1)> & "hi"</text-glow>');
    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;');
    expect(html).not.toContain('<img');
  });

  it('does not produce nested anchors inside decorated link labels', async () => {
    const html = await renderMarkdown(
      '[<text-wave>[inner](https://inner.example)</text-wave>](https://outer.example)'
    );
    expect(html).not.toMatch(/<a\b[^>]*>(?:(?!<\/a>)[\s\S])*<a\b/);
    expect(html).toContain('href="https://inner.example"');
  });
});
