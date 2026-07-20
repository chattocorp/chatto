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
  indexedEventCount: null,
  targetEventCount: null,
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
  query = $state('');
  order = $state(MessageSearchOrder.RELEVANCE);

  private requestId = 0;
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
    this.statusLoading = true;
    this.statusError = false;
    const promise = (async () => {
      try {
        this.status = await this.api.getStatus();
        this.statusLoaded = true;
      } catch {
        this.statusError = true;
      } finally {
        this.statusLoading = false;
        this.statusPromise = null;
      }
    })();
    this.statusPromise = promise;
    return promise;
  }

  async refreshStatus(): Promise<void> {
    this.statusLoaded = false;
    await this.ensureStatus();
  }

  async search(input: Omit<MessageSearchInput, 'cursor'>): Promise<void> {
    const requestId = ++this.requestId;
    this.activeInput = { ...input, roomIds: [...input.roomIds] };
    this.query = input.query;
    this.order = input.order;
    this.results = [];
    this.nextCursor = null;
    this.loading = true;
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
    this.query = '';
    this.order = MessageSearchOrder.RELEVANCE;
  }

  reset(): void {
    this.clearResults();
    this.status = EMPTY_STATUS;
    this.statusLoaded = false;
    this.statusLoading = false;
    this.statusError = false;
    this.statusPromise = null;
  }
}

export { MessageSearchOrder, MessageSearchState };
