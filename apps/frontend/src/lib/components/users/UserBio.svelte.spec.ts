import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { q } from '$lib/test-utils';
import UserBio from './UserBio.svelte';
import '../../../app.css';

describe('UserBio', () => {
  it('renders supported Markdown through the shared safe boundary', async () => {
    const { container } = render(UserBio, {
      props: { bio: '**Builds useful bots.** Visit [Chatto](https://chatto.dev).' }
    });

    await vi.waitFor(
      () => {
        expect(q(container, 'strong')?.textContent).toBe('Builds useful bots.');
        expect(q(container, 'a')?.getAttribute('href')).toBe('https://chatto.dev');
      },
      { timeout: 5_000 }
    );
  });

  it('renders message Markdown blocks and timestamp controls', async () => {
    const { container } = render(UserBio, {
      props: {
        bio: '# About me\n\nFirst paragraph.\n\nSecond paragraph.\n\n- One\n- Two\n\n> A quote\n\n```js\nconst answer = 42;\n```\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n<t:1700000000:F>'
      }
    });
    await expect.poll(() => container.querySelectorAll('p').length).toBeGreaterThanOrEqual(4);
    await expect
      .poll(() => container.querySelector('pre code')?.textContent)
      .toContain('const answer');
    expect(q(container, 'h1')?.textContent).toBe('About me');
    expect(container.querySelectorAll('li')).toHaveLength(2);
    expect(q(container, 'blockquote')).not.toBeNull();
    expect(q(container, 'table')).not.toBeNull();
    expect(q(container, '.message-timestamp')).not.toBeNull();
    const paragraph = q(container, '.prose > .markdown-html > p') as HTMLElement;
    expect(getComputedStyle(paragraph).display).toBe('block');
  });

  it('does not render source HTML', async () => {
    const { container } = render(UserBio, {
      props: { bio: '<img src=x onerror=alert(1)> safe' }
    });

    await vi.waitFor(
      () => {
        expect(q(container, '[data-testid="user-bio"]')?.textContent).toContain('<img');
      },
      { timeout: 5_000 }
    );
    expect(q(container, 'img')).toBeNull();
  });
});
