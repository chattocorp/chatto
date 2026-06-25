import type { Client, OperationResult } from '@urql/svelte';
import { graphql } from '$lib/gql';
import type {
  AdminEventLogFilteredQuery,
  AdminEventLogFilteredQueryVariables,
  AdminEventLogLegacyQuery,
  AdminEventLogLegacyQueryVariables
} from '$lib/gql/graphql';
import {
  isUnsupportedGraphQLArgumentError,
  isUnsupportedGraphQLFieldError,
  isUnsupportedGraphQLTypeError
} from '$lib/gql/compatibility';

export type AdminEventLogFilter = {
  eventType: string;
  actorId: string;
  createdAtFrom: string;
  createdAtTo: string;
};

const EMPTY_FILTER: AdminEventLogFilter = {
  eventType: '',
  actorId: '',
  createdAtFrom: '',
  createdAtTo: ''
};

const pageSize = 50;

const AdminEventLogFilteredDocument = graphql(`
  query AdminEventLogFiltered($limit: Int, $before: String, $filter: EventLogFilterInput) {
    admin {
      eventLog(limit: $limit, before: $before, filter: $filter) {
        entries {
          sequence
          subject
          aggregateType
          aggregateId
          eventType
          eventId
          actorId
          createdAt
        }
        hasOlder
        endCursor
        totalCount
        scannedCount
        scanLimit
        scanLimited
      }
    }
  }
`);

const AdminEventLogLegacyDocument = graphql(`
  query AdminEventLogLegacy($limit: Int, $before: String) {
    admin {
      eventLog(limit: $limit, before: $before) {
        entries {
          sequence
          subject
          aggregateType
          aggregateId
          eventType
          eventId
          actorId
          createdAt
        }
        hasOlder
        endCursor
        totalCount
      }
    }
  }
`);

const AdminEventLogEventTypesDocument = graphql(`
  query AdminEventLogEventTypes {
    admin {
      eventLogEventTypes
    }
  }
`);

type FilteredConnection = NonNullable<AdminEventLogFilteredQuery['admin']>['eventLog'];
type LegacyConnection = NonNullable<AdminEventLogLegacyQuery['admin']>['eventLog'];
export type AdminEventLogEntry = FilteredConnection['entries'][number];

function cloneFilter(filter: AdminEventLogFilter): AdminEventLogFilter {
  return { ...filter };
}

function hasActiveFilter(filter: AdminEventLogFilter): boolean {
  return Boolean(filter.eventType || filter.actorId || filter.createdAtFrom || filter.createdAtTo);
}

function filterInput(filter: AdminEventLogFilter) {
  if (!hasActiveFilter(filter)) return null;
  return {
    eventType: filter.eventType || null,
    actorId: filter.actorId || null,
    createdAtFrom: filter.createdAtFrom || null,
    createdAtTo: filter.createdAtTo || null
  };
}

function isEventLogCompatibilityError(error: unknown): boolean {
  return (
    isUnsupportedGraphQLArgumentError(error, 'filter') ||
    isUnsupportedGraphQLTypeError(error, 'EventLogFilterInput') ||
    isUnsupportedGraphQLFieldError(error, 'scannedCount') ||
    isUnsupportedGraphQLFieldError(error, 'scanLimit') ||
    isUnsupportedGraphQLFieldError(error, 'scanLimited')
  );
}

function legacyToFilteredConnection(conn: LegacyConnection): FilteredConnection {
  return {
    ...conn,
    scannedCount: conn.entries.length,
    scanLimit: pageSize,
    scanLimited: false
  };
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
  activeFilter = $state<AdminEventLogFilter>(cloneFilter(EMPTY_FILTER));

  eventTypes = $state.raw<string[]>([]);
  eventTypesLoading = $state(false);
  eventTypesUnsupported = $state(false);

  private requestId = 0;
  private eventTypesRequestId = 0;
  private filterUnsupported = false;

  constructor(private readonly client: Client) {}

  get hasActiveFilter(): boolean {
    return hasActiveFilter(this.activeFilter);
  }

  async loadEventTypes(): Promise<void> {
    if (this.eventTypesLoading || this.eventTypesUnsupported || this.eventTypes.length > 0) return;

    const currentRequest = ++this.eventTypesRequestId;
    this.eventTypesLoading = true;
    try {
      const result = await this.client.query(AdminEventLogEventTypesDocument, {}).toPromise();
      if (currentRequest !== this.eventTypesRequestId) return;
      if (result.error) {
        if (isUnsupportedGraphQLFieldError(result.error, 'eventLogEventTypes')) {
          this.eventTypesUnsupported = true;
          this.eventTypes = [];
          return;
        }
        throw new Error(result.error.message);
      }
      this.eventTypes = result.data?.admin?.eventLogEventTypes ?? [];
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

  async loadFirstPage(filter: AdminEventLogFilter = EMPTY_FILTER): Promise<void> {
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

  private async queryEventLog(
    before: string | null,
    filter: AdminEventLogFilter
  ): Promise<FilteredConnection> {
    if (this.filterUnsupported) {
      return this.queryLegacyEventLog(before);
    }

    const result = await this.client
      .query(AdminEventLogFilteredDocument, {
        limit: pageSize,
        before,
        filter: filterInput(filter)
      } satisfies AdminEventLogFilteredQueryVariables)
      .toPromise();

    if (result.error && isEventLogCompatibilityError(result.error)) {
      this.filterUnsupported = true;
      this.compatibilityMessage =
        'This server does not support Event Log filters yet. Showing the unfiltered log.';
      return this.queryLegacyEventLog(before);
    }

    return this.connectionFromFilteredResult(result);
  }

  private async queryLegacyEventLog(before: string | null): Promise<FilteredConnection> {
    const result = await this.client
      .query(AdminEventLogLegacyDocument, {
        limit: pageSize,
        before
      } satisfies AdminEventLogLegacyQueryVariables)
      .toPromise();

    const conn = this.connectionFromLegacyResult(result);
    return legacyToFilteredConnection(conn);
  }

  private connectionFromFilteredResult(
    result: OperationResult<AdminEventLogFilteredQuery, AdminEventLogFilteredQueryVariables>
  ): FilteredConnection {
    if (result.error) {
      throw new Error(result.error.message);
    }
    const conn = result.data?.admin?.eventLog;
    if (!conn) {
      throw new Error('Event log unavailable (audit permission required)');
    }
    return conn;
  }

  private connectionFromLegacyResult(
    result: OperationResult<AdminEventLogLegacyQuery, AdminEventLogLegacyQueryVariables>
  ): LegacyConnection {
    if (result.error) {
      throw new Error(result.error.message);
    }
    const conn = result.data?.admin?.eventLog;
    if (!conn) {
      throw new Error('Event log unavailable (audit permission required)');
    }
    return conn;
  }

  private applyConnection(conn: FilteredConnection, append: boolean): void {
    this.entries = append ? mergeEntries(this.entries, conn.entries) : conn.entries;
    this.totalCount = String(conn.totalCount);
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
