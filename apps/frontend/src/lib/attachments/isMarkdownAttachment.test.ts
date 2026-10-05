import { describe, expect, it } from 'vitest';
import { isMarkdownAttachment } from './isMarkdownAttachment';

describe('isMarkdownAttachment', () => {
  it.each(['text/markdown', 'text/x-markdown', ' Text/Markdown ; charset=utf-8'])(
    'recognises %s without a Markdown extension',
    (type) => expect(isMarkdownAttachment(type, 'notes.txt')).toBe(true)
  );

  it.each(['', 'text/plain', 'application/octet-stream', 'TEXT/PLAIN; charset=utf-8'])(
    'uses Markdown extensions for generic type %s',
    (type) => {
      expect(isMarkdownAttachment(type, 'notes.MD')).toBe(true);
      expect(isMarkdownAttachment(type, 'notes.Markdown')).toBe(true);
      expect(isMarkdownAttachment(type, 'notes.txt')).toBe(false);
      expect(isMarkdownAttachment(type, 'notes.md.zip')).toBe(false);
    }
  );

  it.each(['text/html', 'application/xhtml+xml', 'image/png', 'application/pdf'])(
    'does not override specific type %s with a filename',
    (type) => expect(isMarkdownAttachment(type, 'notes.md')).toBe(false)
  );
});
