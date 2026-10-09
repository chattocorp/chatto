import { MarkdownManager } from '@tiptap/markdown';
import { getSchema } from '@tiptap/core';
import { EditorState } from '@tiptap/pm/state';
import { describe, expect, it } from 'vitest';
import { renderMarkdown } from '$lib/markdown';
import { createComposerExtensions } from './extensions';
import { getSerializedMarkdown, parseMarkdownForEditor } from './markdown';

async function roundTrip(source: string) {
  const extensions = createComposerExtensions('Write a message');
  const manager = new MarkdownManager({ extensions, markedOptions: { breaks: true } });
  const json = parseMarkdownForEditor(source, (markdown) => manager.parse(markdown));
  const doc = getSchema(extensions).nodeFromJSON(json);
  doc.check();
  const serialized = getSerializedMarkdown({
    state: EditorState.create({ doc }),
    getMarkdown: () => manager.serialize(json)
  });
  return { json, serialized, html: await renderMarkdown(serialized) };
}

describe('Visual editor footnote Markdown', () => {
  it('retains Markdown tables within note definitions', async () => {
    const { html } = await roundTrip(
      'Table[^data].\n\n[^data]: | Name | Value |\n    | --- | --- |\n    | **Answer** | 42 |'
    );
    expect(html).toContain('<table>');
    expect(html).toContain('<strong>Answer</strong>');
    expect(html).toContain('42');
  });
  it('collects definitions from quotes and lists without leaving invalid containers', async () => {
    const { html } = await roundTrip(
      'Quote[^quote]. List[^list].\n\n> [^quote]: In quote.\n\n- [^list]: In list.'
    );
    expect(html).toContain('In quote.');
    expect(html).toContain('In list.');
  });

  it('retains code indentation on the first line of a note', async () => {
    const { json } = await roundTrip('Code[^code].\n\n[^code]:     const answer = 42;');
    const note = json.content?.find((node) => node.type === 'footnoteDefinition');
    expect(note?.content?.[0].type).toBe('codeBlock');
  });
  it('retains labels, repeated references, and reference-order numbering', async () => {
    const source =
      'First[^later], second[^earlier], first again[^later].\n\n[^earlier]: Two.\n[^later]: One.';
    const { json, serialized, html } = await roundTrip(source);
    expect(serialized).toContain('First[^later], second[^earlier], first again[^later].');
    expect(
      json.content?.filter((node) => node.type === 'footnoteDefinition').map((node) => node.attrs)
    ).toEqual([
      { label: 'later', number: 1 },
      { label: 'earlier', number: 2 }
    ]);
    expect(html).toContain('id="fnref1:1"');
    expect(html).toContain('One.');
    expect(html).toContain('Two.');
  });

  it('retains formatted paragraphs, lists, links, and code inside notes', async () => {
    const source = [
      'Text[^details].',
      '',
      '[^details]: **Bold** and *italic* with [a link](https://example.com).',
      '',
      '    Second paragraph.',
      '',
      '    - One',
      '    - Two',
      '',
      '    ```js',
      '    const note = "[^literal]";',
      '    ```'
    ].join('\n');
    const { html, serialized } = await roundTrip(source);
    expect(html).toContain('<strong>Bold</strong>');
    expect(html).toContain('<em>italic</em>');
    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('Second paragraph.');
    expect(html).toContain('<ul>');
    expect(serialized).toContain('const note = "[^literal]";');
    expect((await roundTrip(serialized)).html).toBe(html);
  });

  it('keeps URL-only definitions from becoming link definitions', async () => {
    const { serialized, html } = await roundTrip('See[^url].\n\n[^url]: https://example.com');
    expect(serialized).toContain('[^url]:');
    expect(html).toContain('class="footnote-item"');
    expect(html).toContain('https://example.com');
  });

  it('keeps undefined references and code examples literal', async () => {
    const { json, html } = await roundTrip(
      '**Unknown[^missing]**. `[^code]`\n\n```md\n[^code]: Example\n```'
    );
    expect(json.content?.some((node) => node.type === 'footnoteDefinition')).toBe(false);
    expect(html).not.toContain('class="footnote-ref"');
    expect(html).toContain('<strong>Unknown[^missing]</strong>');
    expect(html).toContain('[^code]: Example');
  });

  it('retains lazy continuation and unused definitions without consuming following text', async () => {
    const { serialized, html } = await roundTrip(
      'Text[^note].\n\n[^note]: First line\ncontinuation.\n\nFollowing text.\n\n[^unused]: Keep me.'
    );
    expect(serialized).toContain('Following text.');
    expect(serialized).toContain('[^unused]: Keep me.');
    expect(html).toContain('First line');
    expect(html).toContain('continuation.');
  });
});
