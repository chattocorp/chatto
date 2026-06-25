import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { titleState } from '$lib/state/globals.svelte';
import PageTitle from './PageTitle.svelte';

beforeEach(() => {
  titleState.clearPageTitle();
});

afterEach(() => {
  titleState.clearPageTitle();
});

describe('PageTitle', () => {
  it('sets the global page title while mounted', () => {
    const rendered = render(PageTitle, { props: { title: 'Overview' } });
    flushSync();

    expect(titleState.pageTitle).toBe('Overview');

    rendered.unmount();
  });

  it('updates the global page title when the prop changes', async () => {
    const rendered = render(PageTitle, { props: { title: 'Overview' } });
    flushSync();

    await rendered.rerender({ title: '#general - Test Space' });
    flushSync();

    expect(titleState.pageTitle).toBe('#general - Test Space');

    rendered.unmount();
  });

  it('clears the global page title on unmount', () => {
    const rendered = render(PageTitle, { props: { title: 'Overview' } });
    flushSync();

    rendered.unmount();
    flushSync();

    expect(titleState.pageTitle).toBeNull();
  });
});
