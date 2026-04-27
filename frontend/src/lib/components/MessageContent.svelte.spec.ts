import { describe, it, expect, beforeAll } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessageContent, { renderMarkdown, rendererReady } from './MessageContent.svelte';
import { q } from '$lib/test-utils';
import type { RoomMember } from '$lib/mentions';
import { PresenceStatus } from '$lib/gql/graphql';

function renderMessage(body: string, members: RoomMember[] = []) {
  return render(MessageContent, { props: { body, members } });
}

function member(login: string): RoomMember {
  return {
    id: `u_${login}`,
    login,
    displayName: login,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}

describe('renderMarkdown', () => {
  // Wait for Shiki to initialize before running tests
  beforeAll(async () => {
    await rendererReady;
  });

  describe('allowed syntax', () => {
    it('renders bold text with **', async () => {
      const html = await renderMarkdown('**bold**');
      expect(html).toContain('<strong>bold</strong>');
    });

    it('renders bold text with __', async () => {
      const html = await renderMarkdown('__bold__');
      expect(html).toContain('<strong>bold</strong>');
    });

    it('renders italic text with *', async () => {
      const html = await renderMarkdown('*italic*');
      expect(html).toContain('<em>italic</em>');
    });

    it('renders italic text with _', async () => {
      const html = await renderMarkdown('_italic_');
      expect(html).toContain('<em>italic</em>');
    });

    it('renders inline code', async () => {
      const html = await renderMarkdown('`code`');
      expect(html).toContain('<code>code</code>');
    });

    it('renders links', async () => {
      const html = await renderMarkdown('[example](https://example.com)');
      expect(html).toContain('href="https://example.com"');
      expect(html).toContain('>example</a>');
    });

    it('renders blockquotes', async () => {
      const html = await renderMarkdown('> quoted text');
      expect(html).toContain('<blockquote>');
      expect(html).toContain('quoted text');
    });

    it('renders fenced code blocks', async () => {
      const html = await renderMarkdown('```\ncode block\n```');
      expect(html).toContain('<pre');
      expect(html).toContain('<code');
      expect(html).toContain('code block');
    });

    it('converts tabs to spaces in code blocks for consistent rendering', async () => {
      const html = await renderMarkdown('```go\nconst (\n\tFoo = 1\n)\n```');
      // Tabs should be converted to spaces to avoid CSS tab-stop issues with line numbers
      expect(html).not.toContain('\t');
      expect(html).toContain('    Foo');
    });

    it('renders code blocks with language hint', async () => {
      const html = await renderMarkdown('```javascript\nconst x = 1;\n```');
      // Shiki uses language-* class on the code element
      expect(html).toContain('language-javascript');
    });

    it('converts line breaks to <br>', async () => {
      const html = await renderMarkdown('line1\nline2');
      expect(html).toContain('<br>');
    });

    it('auto-links plain https URLs', async () => {
      const html = await renderMarkdown('Check out https://example.com for more');
      expect(html).toContain('href="https://example.com"');
      expect(html).toContain('>https://example.com</a>');
    });

    it('auto-links plain http URLs', async () => {
      const html = await renderMarkdown('Visit http://example.com today');
      expect(html).toContain('href="http://example.com"');
    });

    it('auto-links URLs with paths and query strings', async () => {
      const html = await renderMarkdown('See https://example.com/path?query=1&foo=bar');
      expect(html).toContain('href="https://example.com/path?query=1&amp;foo=bar"');
    });

    it('applies security attributes to auto-linked URLs', async () => {
      const html = await renderMarkdown('https://example.com');
      expect(html).toContain('target="_blank"');
      expect(html).toContain('rel="noopener noreferrer"');
    });

    it('auto-links www-prefixed bare domains', async () => {
      const html = await renderMarkdown('Visit www.example.com today');
      expect(html).toContain('href="http://www.example.com"');
      expect(html).toContain('>www.example.com</a>');
    });

    it('auto-links bare domains with .dev TLD', async () => {
      const html = await renderMarkdown('Check www.hmans.dev');
      expect(html).toContain('href="http://www.hmans.dev"');
      expect(html).toContain('>www.hmans.dev</a>');
    });

    it('auto-links bare domains with .io TLD', async () => {
      const html = await renderMarkdown('Try app.example.io');
      expect(html).toContain('href="http://app.example.io"');
    });

    it('applies security attributes to bare-domain auto-links', async () => {
      const html = await renderMarkdown('www.example.com');
      expect(html).toContain('target="_blank"');
      expect(html).toContain('rel="noopener noreferrer"');
    });

    it('renders unordered lists', async () => {
      const html = await renderMarkdown('- item 1\n- item 2');
      expect(html).toContain('<ul>');
      expect(html).toContain('<li>');
      expect(html).toContain('item 1');
      expect(html).toContain('item 2');
    });

    it('renders ordered lists', async () => {
      const html = await renderMarkdown('1. first\n2. second');
      expect(html).toContain('<ol>');
      expect(html).toContain('<li>');
      expect(html).toContain('first');
      expect(html).toContain('second');
    });

    it('renders nested lists', async () => {
      const html = await renderMarkdown('- outer\n  - inner');
      expect(html).toContain('<ul>');
      expect(html).toContain('<li>');
      expect(html).toContain('outer');
      expect(html).toContain('inner');
    });
  });

  describe('forbidden syntax (should render as literal text)', () => {
    it('does not render images as img tags', async () => {
      const html = await renderMarkdown('![alt](https://example.com/img.png)');
      // Image syntax is disabled, so no <img> tag should be rendered
      // markdown-it parses this as "!" followed by a link, which is safe
      expect(html).not.toContain('<img');
    });

    it('does not render headings with #', async () => {
      const html = await renderMarkdown('# Heading');
      expect(html).not.toContain('<h1');
      expect(html).toContain('# Heading');
    });

    it('does not render headings with ##', async () => {
      const html = await renderMarkdown('## Heading');
      expect(html).not.toContain('<h2');
      expect(html).toContain('## Heading');
    });

    it('does not render horizontal rules', async () => {
      const html = await renderMarkdown('---');
      expect(html).not.toContain('<hr');
    });

    it('does not render tables', async () => {
      const html = await renderMarkdown('| a | b |\n|---|---|\n| 1 | 2 |');
      expect(html).not.toContain('<table');
    });
  });

  describe('security - link validation', () => {
    it('adds target="_blank" to links', async () => {
      const html = await renderMarkdown('[link](https://example.com)');
      expect(html).toContain('target="_blank"');
    });

    it('adds rel="noopener noreferrer" to links', async () => {
      const html = await renderMarkdown('[link](https://example.com)');
      expect(html).toContain('rel="noopener noreferrer"');
    });

    it('does not create links for javascript: URLs', async () => {
      const html = await renderMarkdown('[click](javascript:alert(1))');
      // markdown-it doesn't parse javascript: as valid link, renders as literal text
      expect(html).not.toContain('href="javascript:');
    });

    it('does not create links for data: URLs', async () => {
      const html = await renderMarkdown('[click](data:text/html,<script>alert(1)</script>)');
      // markdown-it doesn't parse data: as valid link, renders as literal text
      expect(html).not.toContain('href="data:');
    });

    it('allows http:// URLs', async () => {
      const html = await renderMarkdown('[link](http://example.com)');
      expect(html).toContain('href="http://example.com"');
    });

    it('allows https:// URLs', async () => {
      const html = await renderMarkdown('[link](https://example.com)');
      expect(html).toContain('href="https://example.com"');
    });
  });

  describe('security - HTML sanitization', () => {
    it('escapes script tags', async () => {
      const html = await renderMarkdown('<script>alert(1)</script>');
      expect(html).not.toContain('<script>');
      expect(html).toContain('&lt;script&gt;');
    });

    it('escapes img tags with onerror', async () => {
      const html = await renderMarkdown('<img src=x onerror=alert(1)>');
      expect(html).not.toContain('<img');
    });

    it('escapes svg tags', async () => {
      const html = await renderMarkdown('<svg onload=alert(1)>');
      expect(html).not.toContain('<svg');
    });

    it('escapes anchor tags with onclick', async () => {
      const html = await renderMarkdown('<a href="#" onclick="alert(1)">click</a>');
      // HTML is escaped, so no actual <a> tag with onclick is created
      expect(html).not.toContain('<a ');
      expect(html).toContain('&lt;a');
    });
  });
});

describe('MessageContent component', () => {
  // Wait for Shiki to initialize before running tests
  beforeAll(async () => {
    await rendererReady;
  });

  it('renders markdown content', async () => {
    const { container } = renderMessage('**bold** and *italic*');

    // Wait for async markdown rendering by polling for the element
    await expect.poll(() => q(container, 'strong')).toBeTruthy();
    await expect.poll(() => q(container, 'em')).toBeTruthy();
  });

  it('applies prose classes for typography', async () => {
    const { container } = renderMessage('Hello world');

    const wrapper = q(container, '.prose');
    await expect.element(wrapper).toBeInTheDocument();
  });

  it('renders links with security attributes', async () => {
    const { container } = renderMessage('[link](https://example.com)');

    // Wait for async markdown rendering by polling for the element
    await expect.poll(() => q(container, 'a')).toBeTruthy();
    const link = q(container, 'a')!;
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  describe('mention wiring', () => {
    // wrapValidMentions itself is exhaustively tested in $lib/mentions.svelte.test.ts.
    // These tests assert that MessageContent actually invokes it — i.e., that the
    // wrapper class shows up in the rendered DOM when a matching member is present.
    it('wraps a known @mention in span.mention when members include the login', async () => {
      const { container } = renderMessage('Hello @alice!', [member('alice')]);
      await expect.poll(() => q(container, 'span.mention')).toBeTruthy();
      const span = q(container, 'span.mention')!;
      expect(span.textContent).toBe('@alice');
      expect(span.getAttribute('data-user-id')).toBe('u_alice');
    });

    it('does not wrap an @mention when no member matches', async () => {
      const { container } = renderMessage('Hello @nobody!', [member('alice')]);
      // Wait for markdown to render so we know the prose pass completed
      await expect.poll(() => q(container, '.prose')).toBeTruthy();
      expect(q(container, 'span.mention')).toBeNull();
    });
  });
});
