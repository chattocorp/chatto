import { describe, expect, it } from 'vitest';
import { createComposerExtensions } from './extensions';
import { MarkdownManager } from '@tiptap/markdown';
import { getSchema } from '@tiptap/core';
import { EditorState } from '@tiptap/pm/state';
import { getSerializedMarkdown, parseMarkdownForEditor } from './markdown';

describe('createComposerExtensions', () => {
  it.each([
    '```svg rendered\n<svg xmlns="http://www.w3.org/2000/svg"/>\n```',
    '```mermaid\nflowchart LR\nA[One] --> B[Two]\n```',
    '```mermaid rendered\nflowchart LR\nA --> B\n```',
    '```ansi rendered\nART\n```'
  ])('preserves diagram source through the visual editor (%#)', (source) => {
    const extensions = createComposerExtensions('');
    const manager = new MarkdownManager({ extensions });
    const json = parseMarkdownForEditor(source, (text) => manager.parse(text));
    const schema = getSchema(extensions);
    const state = EditorState.create({ schema, doc: schema.nodeFromJSON(json) });
    expect(getSerializedMarkdown({ state, getMarkdown: () => manager.serialize(json) })).toBe(
      source
    );
  });
  it('builds the complete composer extension set with the active placeholder', () => {
    const extensions = createComposerExtensions('Write a message');

    expect(extensions.map(({ name }) => name)).toEqual([
      'starterKit',
      'bold',
      'underline',
      'link',
      'markdown',
      'unsupportedLinkProtocol',
      'footnoteReference',
      'footnoteDefinition',
      'codeBlock',
      'selectedTextInlineCodeShortcut',
      'markdownLinkInputRule',
      'markdownAutolinkInputRule',
      'normalizeMarkdownAutolinks',
      'completedMarkdownCodeFence',
      'markdownListMarkerAfterHardBreak',
      'trailingParagraphAfterCodeBlock',
      'clearMarksOnEmptyDocument',
      'placeholder'
    ]);
    expect(extensions.at(-1)?.options.placeholder).toBe('Write a message');
  });

  it('creates isolated extension instances for every editor', () => {
    const first = createComposerExtensions('First');
    const second = createComposerExtensions('Second');

    expect(first).toHaveLength(second.length);
    first.forEach((extension, index) => {
      expect(extension).not.toBe(second[index]);
    });
  });
});
