import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PageTitle from './PageTitle.svelte';

vi.mock('$lib/render/pageTitle', () => ({
  formatPageTitle: (title = '', scope = 'route') => {
    const name = scope === 'app' ? 'Chatto' : 'Test Server';
    return title ? `${title} · ${name}` : name;
  }
}));

afterEach(() => {
  document.title = '';
});

describe('PageTitle', () => {
  it('renders a title and updates it when props change', async () => {
    const rendered = render(PageTitle, { props: { title: 'Overview' } });
    flushSync();
    expect(document.title).toBe('Overview · Test Server');

    await rendered.rerender({ title: 'Appearance', scope: 'app' });
    expect(document.title).toBe('Appearance · Chatto');
    expect(document.head.querySelectorAll('title')).toHaveLength(1);
    rendered.unmount();
  });

  it('renders the fallback when no page title is supplied', () => {
    const rendered = render(PageTitle);
    flushSync();
    expect(document.title).toBe('Test Server');
    rendered.unmount();
  });
});
