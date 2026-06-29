import {
  EMPTY_ADMIN_EVENT_LOG_FILTER,
  type AdminEventLogAPI,
  type AdminEventLogEntry,
  type AdminEventLogFilter,
  type AdminEventLogPage
} from '@chatto/api-client/adminEventLog';

export type { AdminEventLogEntry, AdminEventLogFilter } from '@chatto/api-client/adminEventLog';

const pageSize = 50;

function cloneFilter(filter: AdminEventLogFilter): AdminEventLogFilter {
  return { ...filter };
}

function hasActiveFilter(filter: AdminEventLogFilter): boolean {
  return Boolean(filter.eventType || filter.actorId || filter.createdAtFrom || filter.createdAtTo);
}

export class AdminEventLogStore {
  entries = $state.raw<AdminEventLogEntry[]>([]);
  totalCount = $state('0');
  scannedCount = $state(0);
  scanLimit = $state(pageSize);
  scanLimited = $state(false);
  hasOlder = $state(false);
  endCursor = $state<string | null>(null);
  loading = $state(true);
  loadingMore = $state(false);
  error = $state<string | null>(null);
  compatibilityMessage = $state<string | null>(null);
  activeFilter = $state<AdminEventLogFilter>(cloneFilter(EMPTY_ADMIN_EVENT_LOG_FILTER));

  eventTypes = $state.raw<string[]>([]);
  eventTypesLoading = $state(false);
  eventTypesUnsupported = $state(false);

  private requestId = 0;
  private eventTypesRequestId = 0;

  constructor(private readonly api: AdminEventLogAPI) {}

  get hasActiveFilter(): boolean {
    return hasActiveFilter(this.activeFilter);
  }

  async loadEventTypes(): Promise<void> {
    if (this.eventTypesLoading || this.eventTypes.length > 0) return;

    const currentRequest = ++this.eventTypesRequestId;
    this.eventTypesLoading = true;
    this.eventTypesUnsupported = false;
    try {
      const eventTypes = await this.api.listEventTypes();
      if (currentRequest !== this.eventTypesRequestId) return;
      this.eventTypes = eventTypes;
    } catch {
      if (currentRequest === this.eventTypesRequestId) {
        this.eventTypes = [];
      }
    } finally {
      if (currentRequest === this.eventTypesRequestId) {
        this.eventTypesLoading = false;
      }
    }
  }

  async loadFirstPage(filter: AdminEventLogFilter = EMPTY_ADMIN_EVENT_LOG_FILTER): Promise<void> {
    const currentRequest = ++this.requestId;
    this.loading = true;
    this.error = null;
    this.compatibilityMessage = null;
    this.entries = [];
    this.totalCount = '0';
    this.scannedCount = 0;
    this.scanLimit = pageSize;
    this.scanLimited = false;
    this.hasOlder = false;
    this.endCursor = null;
    this.activeFilter = cloneFilter(filter);

    try {
      const conn = await this.queryEventLog(null, filter);
      if (currentRequest !== this.requestId) return;
      this.applyConnection(conn, false);
    } catch (e) {
      if (currentRequest !== this.requestId) return;
      this.error = e instanceof Error ? e.message : 'Failed to load event log';
    } finally {
      if (currentRequest === this.requestId) {
        this.loading = false;
      }
    }
  }

  async loadMore(): Promise<void> {
    if (this.loading || this.loadingMore || !this.hasOlder) return;

    const before = this.endCursor ?? this.entries[this.entries.length - 1]?.sequence;
    if (!before) return;

    const currentRequest = ++this.requestId;
    const filter = cloneFilter(this.activeFilter);
    this.loadingMore = true;
    this.error = null;

    try {
      const conn = await this.queryEventLog(before, filter);
      if (currentRequest !== this.requestId) return;
      this.applyConnection(conn, true);
    } catch (e) {
      if (currentRequest !== this.requestId) return;
      this.error = e instanceof Error ? e.message : 'Failed to load older events';
    } finally {
      if (currentRequest === this.requestId) {
        this.loadingMore = false;
      }
    }
  }

  getEvent(sequence: string): Promise<AdminEventLogEntry | null> {
    return this.api.getEvent(sequence);
  }

  private queryEventLog(
    before: string | null,
    filter: AdminEventLogFilter
  ): Promise<AdminEventLogPage> {
    return this.api.listEvents({
      limit: pageSize,
      before,
      filter
    });
  }

  private applyConnection(conn: AdminEventLogPage, append: boolean): void {
    this.entries = append ? mergeEntries(this.entries, conn.entries) : conn.entries;
    this.totalCount = conn.totalCount;
    this.scannedCount = conn.scannedCount;
    this.scanLimit = conn.scanLimit;
    this.scanLimited = conn.scanLimited;
    this.hasOlder = conn.hasOlder;
    this.endCursor = conn.endCursor ?? null;
  }
}

function mergeEntries(
  existing: AdminEventLogEntry[],
  next: AdminEventLogEntry[]
): AdminEventLogEntry[] {
  const seen = new Set(existing.map((entry) => entry.sequence));
  return [...existing, ...next.filter((entry) => !seen.has(entry.sequence))];
}
