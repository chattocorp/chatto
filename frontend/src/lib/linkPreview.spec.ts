import { describe, it, expect } from 'vitest';
import { extractURLs, isYouTubeURL, parseYouTubeVideoID } from './linkPreview';

describe('extractURLs', () => {
  describe('protocol URLs', () => {
    it('extracts https URLs', () => {
      expect(extractURLs('Check https://example.com')).toEqual(['https://example.com']);
    });

    it('extracts http URLs and keeps them as-is', () => {
      expect(extractURLs('Check http://example.com')).toEqual(['http://example.com']);
    });

    it('extracts URLs with paths and query strings', () => {
      expect(extractURLs('See https://example.com/path?q=1')).toEqual([
        'https://example.com/path?q=1'
      ]);
    });

    it('strips trailing punctuation from protocol URLs', () => {
      expect(extractURLs('See https://example.com/path!!')).toEqual([
        'https://example.com/path'
      ]);
    });

    it('strips wrapper closing parentheses but keeps balanced URL parentheses', () => {
      expect(extractURLs('See (https://example.com/path)')).toEqual([
        'https://example.com/path'
      ]);
      expect(extractURLs('See https://example.com/path_(v1)')).toEqual([
        'https://example.com/path_(v1)'
      ]);
    });
  });

  describe('bare-domain URLs', () => {
    it('detects www-prefixed URLs', () => {
      expect(extractURLs('Visit www.example.com')).toEqual(['https://www.example.com']);
    });

    it('detects bare domains with common TLDs', () => {
      expect(extractURLs('Visit example.com')).toEqual(['https://example.com']);
    });

    it('detects bare domains with newer TLDs like .dev', () => {
      expect(extractURLs('check www.hmans.dev')).toEqual(['https://www.hmans.dev']);
    });

    it('detects bare domains with .io TLD', () => {
      expect(extractURLs('try app.example.io')).toEqual(['https://app.example.io']);
    });

    it('detects bare domains with .app TLD', () => {
      expect(extractURLs('see myapp.app')).toEqual(['https://myapp.app']);
    });

    it('detects bare domains with paths', () => {
      expect(extractURLs('read www.hmans.dev/blog/chatto')).toEqual([
        'https://www.hmans.dev/blog/chatto'
      ]);
    });

    it('normalizes bare domains to https://', () => {
      const urls = extractURLs('www.example.com');
      expect(urls[0]).toMatch(/^https:\/\//);
    });
  });

  describe('deduplication and limits', () => {
    it('returns at most maxURLs results', () => {
      expect(extractURLs('https://a.com https://b.com https://c.com', 2)).toHaveLength(2);
    });

    it('defaults to 1 URL', () => {
      expect(extractURLs('https://a.com https://b.com')).toHaveLength(1);
    });

    it('deduplicates identical URLs', () => {
      expect(extractURLs('https://example.com https://example.com', 5)).toEqual([
        'https://example.com'
      ]);
    });

    it('deduplicates bare and protocol URLs pointing to the same host', () => {
      const urls = extractURLs('www.example.com and https://www.example.com', 5);
      expect(urls).toHaveLength(1);
    });

    it('deduplicates case-insensitive hosts and ignores fragments', () => {
      expect(extractURLs('https://Example.com/a#one https://example.com/a#two', 5)).toEqual([
        'https://Example.com/a#one'
      ]);
    });

    it('returns no URLs when maxURLs is zero or negative', () => {
      expect(extractURLs('https://example.com', 0)).toEqual([]);
      expect(extractURLs('https://example.com', -1)).toEqual([]);
    });
  });

  describe('edge cases', () => {
    it('returns empty array for text with no URLs', () => {
      expect(extractURLs('no urls here')).toEqual([]);
    });

    it('returns empty array for empty string', () => {
      expect(extractURLs('')).toEqual([]);
    });

    it('handles URL at start of text', () => {
      expect(extractURLs('https://example.com is cool')).toEqual(['https://example.com']);
    });

    it('handles URL at end of text', () => {
      expect(extractURLs('check out https://example.com')).toEqual(['https://example.com']);
    });

    it('ignores email addresses and non-http URLs', () => {
      expect(extractURLs('mail user@example.com')).toEqual([]);
      expect(extractURLs('fetch ftp://example.com/file')).toEqual([]);
    });
  });

  describe('markdown boundaries', () => {
    it('ignores URLs inside inline code', () => {
      expect(extractURLs('Run `curl https://example.com` first')).toEqual([]);
    });

    it('ignores URLs inside escaped-backtick inline code', () => {
      expect(extractURLs('Run \\`curl https://example.com\\` first')).toEqual([]);
    });

    it('detects URLs immediately after inline code', () => {
      expect(extractURLs('`curl`https://example.com')).toEqual(['https://example.com']);
    });

    it('ignores URLs inside fenced and indented code blocks', () => {
      expect(extractURLs('```\nhttps://example.com\n```\nhttps://outside.example')).toEqual([
        'https://outside.example'
      ]);
      expect(extractURLs('    https://example.com\nhttps://outside.example')).toEqual([
        'https://outside.example'
      ]);
    });

    it('ignores URLs inside blockquotes', () => {
      expect(extractURLs('> https://quoted.example\n\nhttps://outside.example')).toEqual([
        'https://outside.example'
      ]);
    });

    it('detects explicit markdown link destinations', () => {
      expect(extractURLs('Read [the docs](https://example.com/docs)')).toEqual([
        'https://example.com/docs'
      ]);
    });

    it('preserves order around excluded markdown regions', () => {
      expect(
        extractURLs(
          'https://a.example `https://skip.example` https://b.example\n> https://quote.example\n\nhttps://c.example',
          5
        )
      ).toEqual(['https://a.example', 'https://b.example', 'https://c.example']);
    });
  });
});

describe('parseYouTubeVideoID', () => {
  it('extracts valid YouTube video IDs from supported URL forms', () => {
    expect(parseYouTubeVideoID('https://www.youtube.com/watch?v=dQw4w9WgXcQ')).toBe(
      'dQw4w9WgXcQ'
    );
    expect(parseYouTubeVideoID('https://www.youtube.com/watch?feature=share&v=dQw4w9WgXcQ')).toBe(
      'dQw4w9WgXcQ'
    );
    expect(parseYouTubeVideoID('https://www.youtube.com/embed/dQw4w9WgXcQ')).toBe(
      'dQw4w9WgXcQ'
    );
    expect(parseYouTubeVideoID('https://youtu.be/dQw4w9WgXcQ?t=42')).toBe('dQw4w9WgXcQ');
    expect(parseYouTubeVideoID('https://m.youtube.com/shorts/dQw4w9WgXcQ')).toBe(
      'dQw4w9WgXcQ'
    );
  });

  it('rejects non-YouTube hosts and invalid video URL forms', () => {
    expect(parseYouTubeVideoID('https://notyoutube.com/watch?v=dQw4w9WgXcQ')).toBeNull();
    expect(
      parseYouTubeVideoID('https://evil.com/redirect?to=youtube.com/watch?v=dQw4w9WgXcQ')
    ).toBeNull();
    expect(parseYouTubeVideoID('https://youtu.be/short')).toBeNull();
    expect(parseYouTubeVideoID('https://youtu.be/dQw4w9WgXcQ/extra')).toBeNull();
    expect(parseYouTubeVideoID('ftp://www.youtube.com/watch?v=dQw4w9WgXcQ')).toBeNull();
  });

  it('backs isYouTubeURL with the same parser', () => {
    expect(isYouTubeURL('https://youtu.be/dQw4w9WgXcQ')).toBe(true);
    expect(isYouTubeURL('https://example.com/watch?v=dQw4w9WgXcQ')).toBe(false);
  });
});
