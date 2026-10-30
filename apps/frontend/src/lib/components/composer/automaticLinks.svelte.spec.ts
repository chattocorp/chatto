import { Editor } from '@tiptap/core';
import { describe, expect, it } from 'vitest';
import { createComposerExtensions } from './extensions';
import { getSerializedMarkdown } from './markdown';

describe('composer automatic links', () => {
  it.each([
    'https://known.example.test',
    'https://known.example.test/a_b',
    '**https://known.example.test**',
    '<https://known.example.test>',
    '[https://known.example.test](https://known.example.test)'
  ])('preserves the source distinction for %s', (source) => {
    const editor = new Editor({
      extensions: createComposerExtensions(''),
      content: source,
      contentType: 'markdown'
    });
    try {
      expect(getSerializedMarkdown(editor)).toBe(source);
    } finally {
      editor.destroy();
    }
  });
  it.each([
    '/chat/chat.chatto.run/R8rLgzhPkYja7Gs',
    'agents.md',
    'readme.md',
    'foo.py',
    'agents.md:8080',
    'example.com',
    '//example.com',
    'user@example.com',
    'ftp://example.com/file',
    'ftps://example.com/file',
    'ssh://example.com',
    'tel:+123456789',
    'sms:+123456789',
    'vscode://file/test',
    'file:///etc/passwd',
    'data:text/html,test',
    'javascript:alert(1)',
    'blob:https://example.com/id'
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

  it.each(['www.example.com', 'https://example.com', 'mailto:foo@example.com'])(
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

  it('preserves a root-relative link when restoring and editing Markdown', () => {
    const href = '/chat/chat.chatto.run/R8rLgzhPkYja7Gs';
    const editor = new Editor({
      extensions: createComposerExtensions(''),
      content: `[link](${href})`,
      contentType: 'markdown'
    });
    try {
      expect(editor.getHTML()).toContain(`href="${href}"`);
      expect(editor.getMarkdown()).toBe(`[link](${href})`);
      editor.commands.setContent(`<p><a href="${href}">link</a></p>`);
      expect(editor.getHTML()).toContain(`href="${href}"`);
      expect(editor.commands.setLink({ href })).toBe(true);
    } finally {
      editor.destroy();
    }
  });
});

it.each([
  '//example.com/path',
  '/\\example.com/path',
  'ftp://example.com/file',
  'ftps://example.com/file',
  'ssh://example.com',
  'tel:+123456789',
  'sms:+123456789',
  'vscode://file/test',
  'file:///etc/passwd',
  'data:text/html,test',
  'javascript:alert(1)',
  'blob:https://example.com/id'
])('rejects explicit and pasted HTML links to %s', (href) => {
  const editor = new Editor({
    extensions: createComposerExtensions(''),
    content: `[label](${href})`,
    contentType: 'markdown'
  });
  try {
    expect(editor.getHTML()).not.toContain('<a ');
    editor.commands.setContent(`<p><a href="${href}">label</a></p>`);
    expect(editor.getHTML()).not.toContain('<a ');
    expect(editor.commands.setLink({ href })).toBe(false);
  } finally {
    editor.destroy();
  }
});

it('preserves formatting after a prose colon', () => {
  const editor = new Editor({
    extensions: createComposerExtensions(''),
    content: 'Note:**bold**',
    contentType: 'markdown'
  });
  try {
    expect(editor.getHTML()).toContain('Note:<strong>bold</strong>');
  } finally {
    editor.destroy();
  }
});
