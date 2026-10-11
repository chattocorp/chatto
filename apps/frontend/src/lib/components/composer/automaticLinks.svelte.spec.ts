import { Editor } from '@tiptap/core';
import { describe, expect, it } from 'vitest';
import { createComposerExtensions } from './extensions';

describe('composer automatic links', () => {
  it.each([
    'agents.md',
    'readme.md',
    'foo.py',
    'agents.md:8080',
    'example.com',
    '//example.com',
    'user@example.com'
  ])('keeps %s plain when restoring and typing', (text) => {
    const editor = new Editor({
      extensions: createComposerExtensions(''),
      content: text,
      contentType: 'markdown'
    });
    try {
      expect(editor.getHTML()).not.toContain('<a ');
      editor.commands.clearContent();
      editor.commands.insertContent(text + ' ');
      expect(editor.getHTML()).not.toContain('<a ');
    } finally {
      editor.destroy();
    }
  });

  it.each(['www.example.com', 'https://example.com', 'ftp://example.com/file'])(
    'links typed %s',
    (text) => {
      const editor = new Editor({ extensions: createComposerExtensions('') });
      try {
        editor.commands.insertContent(text + ' ');
        expect(editor.getHTML()).toContain('<a ');
      } finally {
        editor.destroy();
      }
    }
  );

  it.each([
    'www.example.com',
    'https://example.com',
    '[agents.md](https://example.com/agents.md)',
    '<https://example.com>'
  ])('preserves %s as a link', (text) => {
    const editor = new Editor({
      extensions: createComposerExtensions(''),
      content: text,
      contentType: 'markdown'
    });
    try {
      expect(editor.getHTML()).toContain('<a ');
    } finally {
      editor.destroy();
    }
  });
});
