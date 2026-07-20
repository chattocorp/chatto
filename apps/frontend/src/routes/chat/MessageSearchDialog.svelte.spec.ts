import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { MessageSearchState } from '$lib/state/server/messageSearch.svelte';
import MessageSearchDialog from './MessageSearchDialog.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    ensureStatus: vi.fn(),
    clearResults: vi.fn(),
    search: vi.fn(),
    loadMore: vi.fn()
  }
}));

vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));
vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      currentUser: {
        user: {
          settings: null
        }
      },
      messageSearch: {
        status: {
          state: MessageSearchState.READY,
          indexedEventCount: null,
          targetEventCount: null
        },
        statusLoading: false,
        statusLoaded: true,
        statusError: false,
        available: true,
        results: [],
        nextCursor: null,
        loading: false,
        loadingMore: false,
        error: false,
        ensureStatus: mocks.ensureStatus,
        refreshStatus: vi.fn(),
        clearResults: mocks.clearResults,
        search: mocks.search,
        loadMore: mocks.loadMore
      }
    })
  }
}));

describe('MessageSearchDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('mounts from the global modal container without a nested user-settings context', async () => {
    const { container } = render(MessageSearchDialog, {
      props: { serverId: 'origin', onclose: vi.fn() }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('dialog[open]')).not.toBeNull();
    });
    expect(container.textContent).toContain('Search messages');
    expect(mocks.ensureStatus).toHaveBeenCalledOnce();
  });
});
