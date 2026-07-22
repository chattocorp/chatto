import {
  MessageSearchOrder,
  MessageSearchState,
  type MessageSearchAPI,
  type MessageSearchInput,
  type MessageSearchResult,
  type MessageSearchStatus
} from '$lib/api-client/messageSearch';
import { SvelteSet } from 'svelte/reactivity';

const EMPTY_STATUS: MessageSearchStatus = {
  state: MessageSearchState.UNSPECIFIED,
  retryAfterMs: null
};

/** Server-scoped search availability and transient query results. */
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

  private requestId = 0;
  private statusRequestId = 0;
  private activeInput: Omit<MessageSearchInput, 'cursor'> | null = null;
  private statusPromise: Promise<void> | null = null;

  constructor(private readonly api: MessageSearchAPI) {}

  get available(): boolean {
    return (
      this.status.state === MessageSearchState.READY ||
      this.status.state === MessageSearchState.DEGRADED
    );
  }

  async ensureStatus(): Promise<void> {
    if (this.statusLoaded || this.statusPromise) return this.statusPromise ?? Promise.resolve();
    const requestId = ++this.statusRequestId;
    this.statusLoading = true;
    this.statusError = false;
    const promise = Promise.resolve()
      .then(() => this.api.getStatus())
      .then((status) => {
        if (requestId !== this.statusRequestId) return;
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

  async search(input: Omit<MessageSearchInput, 'cursor'>): Promise<void> {
    const requestId = ++this.requestId;
    this.activeInput = { ...input };
    this.hasSearched = true;
    this.query = input.query;
    this.order = input.order;
    this.results = [];
    this.nextCursor = null;
    this.loading = true;
    this.loadingMore = false;
    this.error = false;
    try {
      const page = await this.api.searchMessages(input);
      if (requestId !== this.requestId) return;
      this.results = page.results;
      this.nextCursor = page.nextCursor;
    } catch {
      if (requestId === this.requestId) this.error = true;
    } finally {
      if (requestId === this.requestId) this.loading = false;
    }
  }

  async loadMore(): Promise<void> {
    if (this.loading || this.loadingMore || !this.nextCursor || !this.activeInput) return;
    const requestId = ++this.requestId;
    const cursor = this.nextCursor;
    this.loadingMore = true;
    this.error = false;
    try {
      const page = await this.api.searchMessages({ ...this.activeInput, cursor });
      if (requestId !== this.requestId) return;
      const seen = new SvelteSet(this.results.map((result) => result.id));
      this.results = [...this.results, ...page.results.filter((result) => !seen.has(result.id))];
      this.nextCursor = page.nextCursor;
    } catch {
      if (requestId === this.requestId) this.error = true;
    } finally {
      if (requestId === this.requestId) this.loadingMore = false;
    }
  }

  clearResults(): void {
    this.requestId++;
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

  /** Purge one room's retained plaintext and fence older responses. */
  invalidateRoom(roomId: string): void {
    this.refreshAfterInvalidation((result) => result.roomId === roomId, false);
  }

  /** Re-run the search after projected room access is revoked. */
  revokeRoom(roomId: string): void {
    this.refreshAfterInvalidation((result) => result.roomId === roomId, true);
  }

  /** Purge one message's retained plaintext and fence older responses. */
  invalidateMessage(roomId: string, messageId: string, force = false): void {
    this.refreshAfterInvalidation(
      (result) => result.roomId === roomId && result.id === messageId,
      force
    );
  }

  /** Purge one author's retained plaintext after projected account removal. */
  invalidateAuthor(authorId: string): void {
    this.refreshAfterInvalidation((result) => result.actorId === authorId, true);
  }

  /** Refetch retained results after a content-free realtime refresh fence. */
  refreshRetainedResults(): void {
    this.refreshAfterInvalidation(() => false, true);
  }

  private refreshAfterInvalidation(
    matches: (result: MessageSearchResult) => boolean,
    force: boolean
  ): void {
    const remaining = this.results.filter((result) => !matches(result));
    if (!force && remaining.length === this.results.length) return;
    const input = this.activeInput;
    this.requestId++;
    this.results = remaining;
    this.nextCursor = null;
    this.activeInput = null;
    this.loading = false;
    this.loadingMore = false;
    this.error = false;
    if (input && this.hasSearched) void this.search(input);
  }

  reset(): void {
    this.clearResults();
    this.statusRequestId++;
    this.status = EMPTY_STATUS;
    this.statusLoaded = false;
    this.statusLoading = false;
    this.statusError = false;
    this.statusPromise = null;
  }
}

export { MessageSearchOrder, MessageSearchState };
