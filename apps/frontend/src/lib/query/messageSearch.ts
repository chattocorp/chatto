// SPDX-License-Identifier: Apache-2.0

import { InfiniteQueryObserver } from '@tanstack/svelte-query';
import type {
  MessageSearchAPI,
  MessageSearchInput,
  MessageSearchPage,
  MessageSearchResult
} from '$lib/api-client/messageSearch';
import { queryClient } from './client';

type SearchInput = Omit<MessageSearchInput, 'cursor'>;

export type MessageSearchQueryState = {
  results: MessageSearchResult[];
  nextCursor: string | null;
  loading: boolean;
  loadingMore: boolean;
  error: boolean;
};

/** One retained search owns one infinite query and removes its plaintext on disposal. */
export function startMessageSearchQuery(
  api: MessageSearchAPI,
  serverId: string,
  queryScope: string,
  searchId: number,
  input: SearchInput,
  publish: (state: MessageSearchQueryState) => void
) {
  const queryKey = ['server', serverId, 'session', queryScope, 'message-search', searchId, input] as const;
  const observer = new InfiniteQueryObserver(queryClient, {
    queryKey,
    queryFn: ({ pageParam, signal }) =>
      api.searchMessages(
        pageParam ? { ...input, cursor: pageParam } : input,
        { signal }
      ),
    initialPageParam: null as string | null,
    getNextPageParam: (lastPage: MessageSearchPage) => lastPage.nextCursor ?? undefined,
    enabled: false,
    gcTime: 0,
    retry: false
  });
  const update = () => {
    const current = observer.getCurrentResult();
    const pages = current.data?.pages ?? [];
    const seen = new Set<string>();
    publish({
      results: pages.flatMap((page) => page.results.filter((result) => {
        if (seen.has(result.id)) return false;
        seen.add(result.id);
        return true;
      })),
      nextCursor: current.hasNextPage ? pages.at(-1)?.nextCursor ?? null : null,
      loading: current.isPending && current.isFetching,
      loadingMore: current.isFetchingNextPage,
      error: current.isError
    });
  };
  const unsubscribe = observer.subscribe(update);
  update();

  return {
    async refetch(): Promise<void> {
      await observer.refetch();
    },
    async fetchNextPage(): Promise<void> {
      await observer.fetchNextPage();
    },
    dispose(): void {
      unsubscribe();
      // Search results are transient plaintext. Do not retain old terms in
      // inactive query entries after input changes or an authority reset.
      queryClient.removeQueries({ queryKey, exact: true });
    }
  };
}

export type MessageSearchQueryHandle = ReturnType<typeof startMessageSearchQuery>;
