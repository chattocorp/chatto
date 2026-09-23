import {
  MessageSearchOrder,
  MessageSearchState,
  type MessageSearchAPI,
  type MessageSearchInput,
  type MessageSearchResult,
  type MessageSearchStatus
} from '$lib/api-client/messageSearch';
import { SvelteSet } from 'svelte/reactivity';
import type { MessageSearchQueryHandle } from '$lib/query/messageSearch';

const EMPTY_STATUS: MessageSearchStatus = {
  state: MessageSearchState.UNSPECIFIED,
  retryAfterMs: null
};
let nextSearchId = 0;

type MessageSearchOptions = {
  /** Keep the raw, user-visible query while submitting a normalized value. */
  preserveQuery?: boolean;
};

/** Server-scoped search input, availability, and privacy fences for transient query results. */
export class MessageSearchStore {
  status = $state<MessageSearchStatus>(EMPTY_STATUS);
  statusLoading = $state(false);
  statusLoaded = $state(false);
  statusError = $state(false);
  results = $state.raw<MessageSearchResult[]>([]);
  nextCursor = $state<string | null>(null);
  loading = $state(false);
  loadingMore = $state(false);
  error = $state(false);
  hasSearched = $state(false);
  query = $state('');
  order = $state(MessageSearchOrder.RELEVANCE);
  /** Advances whenever retained search plaintext must be purged by other consumers. */
  privacyRevision = $state(0);

  private requestId = 0;
  private statusRequestId = 0;
  private activeInput: Omit<MessageSearchInput, 'cursor'> | null = null;
  private statusPromise: Promise<void> | null = null;
  private searchQuery: MessageSearchQueryHandle | null = null;
  private readonly searchId = ++nextSearchId;
  private privacyInvalidationListeners = new SvelteSet<
    (matches: (result: MessageSearchResult) => boolean, force: boolean) => void
  >();

  constructor(
    private readonly api: MessageSearchAPI,
    private readonly canLoad: () => boolean = () => true,
    private readonly scope: { serverId: string; queryScope: string } = {
      serverId: 'isolated', queryScope: 'isolated'
    }
  ) {}

  get available(): boolean {
    return (
      this.status.state === MessageSearchState.READY ||
      this.status.state === MessageSearchState.DEGRADED
    );
  }

  async ensureStatus(): Promise<void> {
    if (!this.canLoad()) return;
    if (this.statusLoaded || this.statusPromise) return this.statusPromise ?? Promise.resolve();
    const requestId = ++this.statusRequestId;
    this.statusLoading = true;
    this.statusError = false;
    const promise = Promise.resolve()
      .then(() => this.api.getStatus())
      .then((status) => {
        if (requestId !== this.statusRequestId || !this.canLoad()) return;
        this.status = status;
        this.statusLoaded = true;
      })
      .catch(() => {
        if (requestId === this.statusRequestId) this.statusError = true;
      })
      .finally(() => {
        if (requestId !== this.statusRequestId) return;
        this.statusLoading = false;
        this.statusPromise = null;
      });
    this.statusPromise = promise;
    return promise;
  }

  async refreshStatus(): Promise<void> {
    this.statusLoaded = false;
    await this.ensureStatus();
  }

  async search(
    input: Omit<MessageSearchInput, 'cursor'>,
    { preserveQuery = false }: MessageSearchOptions = {}
  ): Promise<void> {
    if (!this.canLoad()) return;
    const requestId = ++this.requestId;
    this.searchQuery?.dispose();
    this.searchQuery = null;
    this.activeInput = { ...input };
    this.hasSearched = true;
    if (!preserveQuery) this.query = input.query;
    this.order = input.order;
    this.results = [];
    this.nextCursor = null;
    this.loading = true;
    this.loadingMore = false;
    this.error = false;
    try {
      // Keep TanStack out of the app shell until a user opens search.
      const { startMessageSearchQuery } = await import('$lib/query/messageSearch');
      if (requestId !== this.requestId) return;
      if (!this.canLoad()) {
        this.loading = false;
        return;
      }
      const searchQuery = startMessageSearchQuery(
        this.api,
        this.scope.serverId,
        this.scope.queryScope,
        this.searchId,
        input,
        (state) => {
          if (requestId !== this.requestId || !this.canLoad()) return;
          this.results = state.results;
          this.nextCursor = state.nextCursor;
          this.loading = state.loading;
          this.loadingMore = state.loadingMore;
          this.error = state.error;
        }
      );
      this.searchQuery = searchQuery;
      await searchQuery.refetch();
    } catch {
      if (requestId === this.requestId) {
        this.error = true;
        this.loading = false;
      }
    }
  }

  async loadMore(): Promise<void> {
    if (!this.canLoad()) return;
    if (this.loading || this.loadingMore || !this.nextCursor || !this.searchQuery) return;
    await this.searchQuery.fetchNextPage();
  }

  clearResults(): void {
    this.requestId++;
    this.searchQuery?.dispose();
    this.searchQuery = null;
    this.activeInput = null;
    this.results = [];
    this.nextCursor = null;
    this.loading = false;
    this.loadingMore = false;
    this.error = false;
    this.hasSearched = false;
    this.query = '';
    this.order = MessageSearchOrder.RELEVANCE;
  }

  /** Fence older responses and hide results while a replacement query is being composed. */
  prepareQueryChange(): void {
    this.requestId++;
    this.searchQuery?.dispose();
    this.searchQuery = null;
    this.activeInput = null;
    this.results = [];
    this.nextCursor = null;
    this.loading = false;
    this.loadingMore = false;
    this.error = false;
    this.hasSearched = false;
  }

  /** Re-run the active search after an authority change, preserving its query. */
  refreshPermissions(): void {
    this.refreshAfterInvalidation(() => true, true);
    this.invalidatePrivacyConsumers(() => true, true);
  }

  /** Purge one room's retained plaintext and fence older responses. */
  invalidateRoom(roomId: string): void {
    const matches = (result: MessageSearchResult) => result.roomId === roomId;
    this.refreshAfterInvalidation(matches, true);
    this.invalidatePrivacyConsumers(matches, true);
  }

  /** Re-run the search after projected room access is revoked. */
  revokeRoom(roomId: string): void {
    const matches = (result: MessageSearchResult) => result.roomId === roomId;
    this.refreshAfterInvalidation(matches, true);
    this.invalidatePrivacyConsumers(matches, true);
  }

  /** Purge one message's retained plaintext and fence older responses. */
  invalidateMessage(roomId: string, messageId: string, force = false): void {
    const matches = (result: MessageSearchResult) =>
      result.roomId === roomId && result.id === messageId;
    this.refreshAfterInvalidation(matches, force);
    this.invalidatePrivacyConsumers(matches, force);
  }

  /** Purge one author's retained plaintext after projected account removal. */
  invalidateAuthor(authorId: string): void {
    const matches = (result: MessageSearchResult) => result.actorId === authorId;
    this.refreshAfterInvalidation(matches, true);
    this.invalidatePrivacyConsumers(matches, true);
  }

  /** Refetch retained results after a content-free realtime refresh fence. */
  refreshRetainedResults(): void {
    const matches = () => false;
    this.refreshAfterInvalidation(matches, true);
    this.invalidatePrivacyConsumers(matches, true);
  }

  private refreshAfterInvalidation(
    matches: (result: MessageSearchResult) => boolean,
    force: boolean
  ): void {
    const remaining = this.results.filter((result) => !matches(result));
    const hasInFlightRequest = this.loading || this.loadingMore;
    if (!force && remaining.length === this.results.length && !hasInFlightRequest) return;
    const input = this.activeInput;
    this.requestId++;
    this.searchQuery?.dispose();
    this.searchQuery = null;
    this.results = remaining;
    this.nextCursor = null;
    this.activeInput = null;
    this.loading = false;
    this.loadingMore = false;
    this.error = false;
    if (input && this.hasSearched) void this.search(input, { preserveQuery: true });
  }

  /** Subscribe another transient plaintext consumer to realtime privacy fences. */
  subscribePrivacyInvalidation(
    listener: (matches: (result: MessageSearchResult) => boolean, force: boolean) => void
  ): () => void {
    this.privacyInvalidationListeners.add(listener);
    return () => this.privacyInvalidationListeners.delete(listener);
  }

  private invalidatePrivacyConsumers(
    matches: (result: MessageSearchResult) => boolean,
    force: boolean
  ): void {
    this.privacyRevision++;
    for (const listener of this.privacyInvalidationListeners) {
      try {
        listener(matches, force);
      } catch {
        // Auxiliary plaintext consumers must not interrupt the owning store's
        // authoritative privacy fence or other consumers' notifications.
      }
    }
  }

  reset(): void {
    this.clearResults();
    this.statusRequestId++;
    this.status = EMPTY_STATUS;
    this.statusLoaded = false;
    this.statusLoading = false;
    this.statusError = false;
    this.statusPromise = null;
    this.invalidatePrivacyConsumers(() => false, true);
  }
}

export { MessageSearchOrder, MessageSearchState };
