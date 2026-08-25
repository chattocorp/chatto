import { describe, expect, it } from 'vitest';
import { createComposerExtensions } from './extensions';

describe('createComposerExtensions', () => {
  it('builds the complete composer extension set with the active placeholder', () => {
    const extensions = createComposerExtensions('Write a message');

    expect(extensions.map(({ name }) => name)).toEqual([
      'starterKit',
      'link',
      'markdown',
      'codeBlock',
      'selectedTextInlineCodeShortcut',
      'markdownLinkInputRule',
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
