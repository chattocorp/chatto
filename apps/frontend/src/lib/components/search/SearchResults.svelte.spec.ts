import { describe, expect, it, vi } from 'vitest';
import { createRawSnippet } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { ComponentProps } from 'svelte';
import SearchResults from './SearchResults.svelte';
import { m } from '$lib/i18n/messages';

type SearchStore = ComponentProps<typeof SearchResults>['store'];

function searchStore(overrides: Partial<SearchStore> = {}): SearchStore {
  return {
    error: false,
    loading: false,
    loadingMore: false,
    hasSearched: false,
    results: [],
    nextCursor: null,
    loadMore: vi.fn().mockResolvedValue(undefined),
    ...overrides
  };
}

const children = createRawSnippet(() => ({ render: () => '<span>Search result</span>' }));

describe('SearchResults', () => {
  it('preserves the full page prompt and the shorter sidebar prompt', async () => {
    const view = render(SearchResults, { store: searchStore(), children });
    await expect.element(view.getByText(m('search.prompt.description'))).toBeVisible();
    await view.rerender({ compact: true });
    await expect.element(view.getByText(m('search.prompt.title'))).toBeVisible();
    await expect.element(view.getByText(m('search.prompt.description'))).not.toBeInTheDocument();
  });

  it('shows loading before empty results in the sidebar and errors before loading', async () => {
    const view = render(SearchResults, {
      store: searchStore({ hasSearched: true, loading: true }),
      compact: true,
      children
    });
    await expect.element(view.getByText(m('search.searching'))).toBeVisible();
    await expect.element(view.getByText(m('search.no_results.title'))).not.toBeInTheDocument();
    await view.rerender({ store: searchStore({ hasSearched: true, loading: true, error: true }) });
    await expect.element(view.getByText(m('search.error.title'))).toBeVisible();
    await expect.element(view.getByText(m('search.searching'))).not.toBeInTheDocument();
  });

  it('loads a visible continuation even when the first page has no visible results', async () => {
    const store = searchStore({ hasSearched: true, nextCursor: 'next-page' });
    const view = render(SearchResults, { store, children });
    await expect.poll(() => vi.mocked(store.loadMore).mock.calls.length).toBe(1);
    await expect.element(view.getByText(m('search.no_results.title'))).not.toBeInTheDocument();
    await view.rerender({ store: searchStore({ hasSearched: true }) });
    await expect.element(view.getByText(m('search.no_results.title'))).toBeVisible();
  });
});
