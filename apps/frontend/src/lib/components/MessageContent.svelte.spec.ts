import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import '../../app.css';
import { q } from '$lib/test-utils';
import type { RoomMember } from '$lib/mentions';
import { PresenceStatus } from '$lib/render/types';
import type { TimeFormatSettings } from '$lib/utils/formatTime';

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  segmentToServerId: vi.fn((segment: string) => (segment === '-' ? 'origin' : null)),
  store: {
    currentUser: {
      user: undefined as { login: string } | undefined
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) =>
    serverId === 'origin' ? '-' : serverId === 'chatto-run' ? 'chat.chatto.run' : serverId,
  segmentToServerId: mocks.segmentToServerId
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    tryGetStore: () => mocks.store,
    getServer: (serverId: string) =>
      serverId === 'origin'
        ? { id: 'origin', url: window.location.origin }
        : serverId === 'chatto-run'
          ? { id: 'chatto-run', url: 'https://chat.chatto.run' }
          : undefined,
    get servers() {
      return [
        { id: 'origin', url: window.location.origin },
        { id: 'chatto-run', url: 'https://chat.chatto.run' }
      ];
    }
  }
}));

import MessageContent, { renderMarkdown } from './MessageContent.svelte';

const channelRoomId = 'R123456789abcde';
const dmRoomId = 'abcdef12345678';
const messageId = 'Eabc123DEF456gh';
const threadRootEventId = 'Ethread12345678';

function renderMessage(body: string, members: RoomMember[] = [], roleHandles: string[] = []) {
  return render(MessageContent, { props: { body, members, roleHandles } });
}

const utc24Settings: TimeFormatSettings = {
  get effectiveTimezone() {
    return 'UTC';
  },
  get effectiveHour12() {
    return false;
  }
};
let originalShowPopover: typeof HTMLElement.prototype.showPopover;
let originalHidePopover: typeof HTMLElement.prototype.hidePopover;

function member(login: string): RoomMember {
  return {
    id: `u_${login}`,
    login,
    displayName: login,
    avatarUrl: null,
    presenceStatus: PresenceStatus.Offline
  };
}

function computedColorFor(property: string): string {
  const element = document.createElement('div');
  element.style.color = `var(${property})`;
  document.body.appendChild(element);
  const color = window.getComputedStyle(element).color;
  element.remove();
  return color;
}

function computedBorderColorFor(property: string): string {
  const element = document.createElement('div');
  element.style.borderLeft = `1px solid var(${property})`;
  document.body.appendChild(element);
  const color = window.getComputedStyle(element).borderLeftColor;
  element.remove();
  return color;
}

beforeEach(() => {
  mocks.goto.mockReset();
  mocks.segmentToServerId.mockReset();
  mocks.segmentToServerId.mockImplementation((segment: string) =>
    segment === '-' ? 'origin' : null
  );
  vi.spyOn(window, 'open').mockImplementation(() => null);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

beforeAll(() => {
  originalShowPopover = HTMLElement.prototype.showPopover;
  originalHidePopover = HTMLElement.prototype.hidePopover;

  HTMLElement.prototype.showPopover = function showPopover() {
    this.setAttribute('popover-open', '');
  };
  HTMLElement.prototype.hidePopover = function hidePopover() {
    this.removeAttribute('popover-open');
  };
});

afterAll(() => {
  HTMLElement.prototype.showPopover = originalShowPopover;
  HTMLElement.prototype.hidePopover = originalHidePopover;
});

describe('renderMarkdown', () => {
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

    it('does not add new-window attributes to allow-listed same-origin chat URLs', async () => {
      const href = `${window.location.origin}/chat/-/${channelRoomId}`;
      const html = await renderMarkdown(`[room](${href})`);
      expect(html).toContain(`href="${href}"`);
      expect(html).not.toContain('target="_blank"');
      expect(html).not.toContain('rel="noopener noreferrer"');
    });

    it('keeps new-window attributes on non-allow-listed same-origin URLs', async () => {
      const href = `${window.location.origin}/chat/-/settings`;
      const html = await renderMarkdown(`[settings](${href})`);
      expect(html).toContain(`href="${href}"`);
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

    it('renders ATX headings h1 through h6', async () => {
      for (let level = 1; level <= 6; level++) {
        const hashes = '#'.repeat(level);
        const html = await renderMarkdown(`${hashes} Heading ${level}`);
        expect(html).toContain(`<h${level}>Heading ${level}</h${level}>`);
      }
    });

    it('renders encoded trailing hashes as literal heading text', async () => {
      const html = await renderMarkdown('# test &#35;');
      expect(html).toContain('<h1>test #</h1>');
    });

    it('does not render setext (underline-style) headings', async () => {
      const html = await renderMarkdown('Heading\n===');
      expect(html).not.toContain('<h1');
      expect(html).not.toContain('<h2');
    });
  });

  describe('forbidden syntax (should render as literal text)', () => {
    it('does not render images as img tags', async () => {
      const html = await renderMarkdown('![alt](https://example.com/img.png)');
      // Image syntax is disabled, so no <img> tag should be rendered
      // markdown-it parses this as "!" followed by a link, which is safe
      expect(html).not.toContain('<img');
    });

    it('does not render horizontal rules', async () => {
      const html = await renderMarkdown('---');
      expect(html).not.toContain('<hr');
    });

    it('renders GFM tables', async () => {
      const html = await renderMarkdown('| a | b |\n|---|---|\n| 1 | 2 |');
      expect(html).toContain('<div class="table-scroll" tabindex="0"><table>');
      expect(html).toContain('<th>a</th>');
      expect(html).toContain('<td>1</td>');
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
  it('does not render encoded non-breaking spaces as tall message content', async () => {
    const body = `Magnets, how\n\ndo they work?\n\n- ONCE\n- TWICE\n- THRICE\n\nA haiku by a professional chef,\n\nthis was\n${'&nbsp;\n'.repeat(500)}`;
    const { container } = renderMessage(body);

    await expect.poll(() => q(container, '.prose p')).toBeTruthy();
    const content = q(container, '.prose')!;
    expect(content.textContent).not.toContain('&nbsp;');
    expect(content.clientHeight).toBeLessThan(500);
  });

  it('renders markdown content', async () => {
    const { container } = renderMessage('**bold** and *italic*');

    // Wait for async markdown rendering by polling for the element
    await expect.poll(() => q(container, 'strong')).toBeTruthy();
    await expect.poll(() => q(container, 'em')).toBeTruthy();
  });

  it('renders bold content followed immediately by text', async () => {
    const { container } = renderMessage('fsdfsd **fsdf**fdsf');

    await expect.poll(() => q(container, 'strong')).toBeTruthy();
    expect(q(container, 'strong')?.textContent).toBe('fsdf');
    expect(container.textContent).toContain('fsdfsd fsdffdsf');
  });

  it('renders bold content followed immediately by text in edited messages', async () => {
    const { container } = render(MessageContent, {
      props: {
        body: 'fsdfsd **fsdf**fdsf',
        edited: true
      }
    });

    await expect.poll(() => q(container, 'strong')).toBeTruthy();
    expect(q(container, 'strong')?.textContent).toBe('fsdf');
    expect(container.textContent).toContain('fsdfsd fsdffdsf (edited)');
  });

  it('renders message timestamp tokens as localized time elements', async () => {
    const { container } = render(MessageContent, {
      props: {
        body: 'Call at <t:1745764200:F>',
        timestampSettings: utc24Settings,
        timestampLocale: 'en-US'
      }
    });

    await expect.poll(() => q(container, 'button.message-timestamp')).toBeTruthy();
    const button = q(container, 'button.message-timestamp')!;
    const time = q(container, 'button.message-timestamp time')!;
    expect(button.querySelector('.message-timestamp-icon')).toBeTruthy();
    expect(button.getAttribute('data-timestamp-epoch')).toBe('1745764200');
    expect(button.getAttribute('aria-haspopup')).toBe('dialog');
    expect(button.getAttribute('aria-label')).toBeNull();
    expect(time.getAttribute('datetime')).toBe('2025-04-27T14:30:00.000Z');
    expect(time.textContent).toMatch(/April\s*27,?\s*2025/);
    expect(time.textContent).toContain('14:30');
  });

  it('opens timestamp details from a rendered timestamp', async () => {
    const { container } = render(MessageContent, {
      props: {
        body: 'Call at <t:1745764200:F>',
        timestampSettings: utc24Settings,
        timestampLocale: 'en-US'
      }
    });

    await expect.poll(() => q(container, 'button.message-timestamp')).toBeTruthy();
    (q(container, 'button.message-timestamp') as HTMLButtonElement).click();

    await expect
      .poll(() => q(container, '[data-testid="message-timestamp-details"]')?.textContent)
      .toContain('Date and time');
    const details = q(container, '[data-testid="message-timestamp-details"]')!;
    expect(details.textContent).toContain('Local');
    expect(details.textContent).toContain('Relative');
    expect(details.textContent).not.toContain('UTC');
    expect(details.textContent).not.toContain('1745764200');
  });

  it('updates the relative timestamp detail while the popover is open', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-04-27T14:29:59Z'));
    const { container } = render(MessageContent, {
      props: {
        body: 'Call at <t:1745764200:F>',
        timestampSettings: utc24Settings,
        timestampLocale: 'en-US'
      }
    });

    await expect.poll(() => q(container, 'button.message-timestamp')).toBeTruthy();
    (q(container, 'button.message-timestamp') as HTMLButtonElement).click();

    await expect
      .poll(() => q(container, '[data-testid="message-timestamp-details"]')?.textContent)
      .toContain('in 1 second');

    await vi.advanceTimersByTimeAsync(1000);

    await expect
      .poll(() => q(container, '[data-testid="message-timestamp-details"]')?.textContent)
      .toContain('now');
  });

  it('leaves invalid timestamp tokens literal', async () => {
    const { container } = render(MessageContent, {
      props: {
        body: 'Call at <t:abc:F> or <t:1745764200:R>',
        timestampSettings: utc24Settings,
        timestampLocale: 'en-US'
      }
    });

    await expect.poll(() => container.textContent).toContain('<t:abc:F>');
    expect(container.textContent).toContain('<t:1745764200:R>');
    expect(q(container, '.message-timestamp')).toBeNull();
  });

  it('does not render timestamp tokens inside code or blockquotes', async () => {
    const { container } = render(MessageContent, {
      props: {
        body: '`<t:1745764200:F>`\n\n> <t:1745764200:F>',
        timestampSettings: utc24Settings,
        timestampLocale: 'en-US'
      }
    });

    await expect.poll(() => q(container, 'code')).toBeTruthy();
    expect(q(container, '.message-timestamp')).toBeNull();
    expect(container.textContent).toContain('<t:1745764200:F>');
  });

  it('does not render timestamp tokens inside links', async () => {
    const { container } = render(MessageContent, {
      props: {
        body: '[<t:1745764200:F>](https://example.com)',
        timestampSettings: utc24Settings,
        timestampLocale: 'en-US'
      }
    });

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    expect(q(container, '.message-timestamp')).toBeNull();
    expect(q(container, 'a')?.textContent).toContain('<t:1745764200:F>');
  });

  it('applies prose classes for typography', async () => {
    const { container } = renderMessage('Hello world');

    const wrapper = q(container, '.prose');
    await expect.element(wrapper).toBeInTheDocument();
  });

  it('styles blockquotes as distinct quote blocks', async () => {
    const { container } = renderMessage('> quoted text\n>\n> second line');

    await expect.poll(() => q(container, 'blockquote')).toBeTruthy();

    const blockquote = q(container, 'blockquote')!;
    const styles = window.getComputedStyle(blockquote);
    expect(styles.borderLeftColor).not.toBe(computedBorderColorFor('--color-border'));
    expect(styles.backgroundImage).toBe('none');
    expect(styles.color).not.toBe(computedColorFor('--color-muted'));
  });

  it('shows an inset focus indicator on keyboard-scrollable tables', async () => {
    const { container } = renderMessage(
      '| Name | Role | Location |\n| --- | --- | --- |\n| Ada Lovelace | Administrator | London, United Kingdom |'
    );

    await expect.poll(() => q(container, '.table-scroll')).toBeTruthy();
    const wrapper = q(container, '.prose')!;
    const scroller = q(container, '.table-scroll')!;
    wrapper.setAttribute('style', 'width: 240px');
    await expect.poll(() => scroller.scrollWidth).toBeGreaterThan(scroller.clientWidth);
    await userEvent.tab();

    expect(document.activeElement).toBe(scroller);
    expect(window.getComputedStyle(scroller).overflowX).toBe('auto');
    expect(window.getComputedStyle(scroller).outlineStyle).toBe('solid');
    expect(window.getComputedStyle(scroller).outlineOffset).toBe('-2px');
  });

  it('renders links with security attributes', async () => {
    const { container } = renderMessage('[link](https://example.com)');

    // Wait for async markdown rendering by polling for the element
    await expect.poll(() => q(container, 'a')).toBeTruthy();
    const link = q(container, 'a')!;
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('navigates allow-listed room links in the current tab', async () => {
    const href = `${window.location.origin}/chat/-/${channelRoomId}`;
    const { container } = renderMessage(`[room](${href})`);

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    q(container, 'a')!.click();

    expect(mocks.goto).toHaveBeenCalledWith(`/chat/-/${channelRoomId}`);
    expect(window.open).not.toHaveBeenCalled();
  });

  it('navigates allow-listed DM room links in the current tab', async () => {
    const href = `${window.location.origin}/chat/-/${dmRoomId}`;
    const { container } = renderMessage(`[dm](${href})`);

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    q(container, 'a')!.click();

    expect(mocks.goto).toHaveBeenCalledWith(`/chat/-/${dmRoomId}`);
    expect(window.open).not.toHaveBeenCalled();
  });

  it('navigates allow-listed thread links in the current tab', async () => {
    const href = `${window.location.origin}/chat/-/${channelRoomId}/${threadRootEventId}`;
    const { container } = renderMessage(`[thread](${href})`);

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    q(container, 'a')!.click();

    expect(mocks.goto).toHaveBeenCalledWith(`/chat/-/${channelRoomId}/${threadRootEventId}`);
    expect(window.open).not.toHaveBeenCalled();
  });

  it('navigates allow-listed message links through the message resolver route', async () => {
    const href = `${window.location.origin}/chat/-/${channelRoomId}/m/${messageId}`;
    const { container } = renderMessage(`[message](${href})`);

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    q(container, 'a')!.click();

    expect(mocks.goto).toHaveBeenCalledWith(`/chat/-/${channelRoomId}/m/${messageId}`);
    expect(window.open).not.toHaveBeenCalled();
  });

  it('navigates registered external Chatto message URLs in the current tab', async () => {
    const href = `https://chat.chatto.run/chat/-/${channelRoomId}/m/${messageId}`;
    const { container } = renderMessage(`[message](${href})`);

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    q(container, 'a')!.click();

    expect(mocks.goto).toHaveBeenCalledWith(
      `/chat/chat.chatto.run/${channelRoomId}/m/${messageId}`
    );
    expect(window.open).not.toHaveBeenCalled();
  });

  it('opens same-origin non-allow-listed links in a new window', async () => {
    const href = `${window.location.origin}/chat/-/settings`;
    const { container } = renderMessage(`[settings](${href})`);

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    q(container, 'a')!.click();

    expect(mocks.goto).not.toHaveBeenCalled();
    expect(window.open).toHaveBeenCalledWith(href, '_blank', 'noopener,noreferrer');
  });

  it('opens external links in a new window', async () => {
    const href = 'https://example.com/docs';
    const { container } = renderMessage(`[external](${href})`);

    await expect.poll(() => q(container, 'a')).toBeTruthy();
    q(container, 'a')!.click();

    expect(mocks.goto).not.toHaveBeenCalled();
    expect(window.open).toHaveBeenCalledWith(href, '_blank', 'noopener,noreferrer');
  });

  it('renders fenced code blocks with highlighted markup and hides raw fences', async () => {
    const { container } = renderMessage('```javascript\nconst x = 1;\n```');

    await expect.poll(() => q(container, 'pre.hljs')).toBeTruthy();

    const pre = q(container, 'pre.hljs')!;
    const code = q(container, 'pre.hljs code.language-javascript')!;
    expect(pre.getAttribute('data-language')).toBe('javascript');
    expect(code.textContent).toContain('const x = 1;');
    expect(container.textContent).not.toContain('```javascript');
  });

  it('keeps long fenced code blocks scrollable inside the code element', async () => {
    const longLine = `const result = ${'veryLongVariableName + '.repeat(20)}"end"`;
    const { container } = renderMessage(`\`\`\`javascript\n${longLine}\n\`\`\``);

    await expect.poll(() => q(container, 'pre.hljs code.language-javascript')).toBeTruthy();

    const pre = q(container, 'pre.hljs')!;
    const code = q(container, 'pre.hljs code.language-javascript')!;
    expect(pre.getAttribute('data-language')).toBe('javascript');
    expect(code.textContent).toContain(longLine);
    expect(container.textContent).not.toContain('```javascript');
    expect(window.getComputedStyle(pre).overflowX).toBe('hidden');
    expect(window.getComputedStyle(code).overflowX).toBe('auto');
  });

  it('renders a highlighted code block after leading text', async () => {
    const { container } = renderMessage(
      'Check this out:\n```javascript\nconsole.log("hello");\n```'
    );

    await expect.poll(() => q(container, 'pre.hljs')).toBeTruthy();
    expect(container.textContent).toContain('Check this out:');
    expect(q(container, 'pre.hljs code.language-javascript')?.textContent).toContain(
      'console.log("hello");'
    );
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

    it('does not wrap an @mention inside escaped-backtick inline code', async () => {
      const { container } = renderMessage('\\`@alice\\`', [member('alice')]);
      await expect.poll(() => q(container, 'code')).toBeTruthy();
      expect(q(container, 'span.mention')).toBeNull();
    });

    it('does not wrap an @mention split by markdown formatting', async () => {
      const { container } = renderMessage('@*alice*', [member('alice')]);
      await expect.poll(() => q(container, 'em')).toBeTruthy();
      expect(q(container, 'span.mention')).toBeNull();
    });

    it('wraps an @mention immediately after escaped-backtick inline code', async () => {
      const { container } = renderMessage('\\`cmd\\`@alice', [member('alice')]);
      await expect.poll(() => q(container, 'span.mention')).toBeTruthy();
      expect(q(container, 'code')?.textContent).toContain('cmd');
      expect(q(container, 'span.mention')?.textContent).toBe('@alice');
    });

    it('wraps a known role mention when role handles include the name', async () => {
      const { container } = renderMessage('Hello @admin!', [], ['admin']);
      await expect.poll(() => q(container, 'span.mention-role')).toBeTruthy();
      const span = q(container, 'span.mention-role')!;
      expect(span.textContent).toBe('@admin');
      expect(span.getAttribute('data-role-name')).toBe('admin');
    });
  });
});
