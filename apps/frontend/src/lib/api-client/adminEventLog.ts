import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { AdminEventLogService } from '@chatto/api-types/admin/v1/event_log_connect';
import type { AdminEventLogEntry as APIAdminEventLogEntry } from '@chatto/api-types/admin/v1/event_log_pb';

export type AdminEventLogFilter = {
  eventType: string;
  actorId: string;
  createdAtFrom: string;
  createdAtTo: string;
};

export type AdminEventLogEntry = {
  sequence: string;
  subject: string;
  aggregateType: string;
  aggregateId: string;
  eventType: string;
  eventId: string;
  actorId: string;
  createdAt: string;
  payloadJson: string;
};

export type AdminEventLogPage = {
  entries: AdminEventLogEntry[];
  hasOlder: boolean;
  endCursor: string | null;
  totalCount: string;
  scannedCount: number;
  scanLimit: number;
  scanLimited: boolean;
};

export type AdminEventLogAPI = ReturnType<typeof createAdminEventLogAPI>;

export const EMPTY_ADMIN_EVENT_LOG_FILTER: AdminEventLogFilter = {
  eventType: '',
  actorId: '',
  createdAtFrom: '',
  createdAtTo: ''
};

export function createAdminEventLogAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminEventLogService, config);

  return {
    async listEvents(
      input: {
        limit: number;
        before?: string | null;
        filter?: AdminEventLogFilter;
      },
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminEventLogPage> {
      const response = await client.listEvents(
        {
          limit: input.limit,
          before: input.before ?? undefined,
          filter: eventLogFilterInput(input.filter ?? EMPTY_ADMIN_EVENT_LOG_FILTER)
        },
        { ...(options.signal ? { signal: options.signal } : {}) }
      );
      return {
        entries: response.entries.map(adminEventLogEntry),
        hasOlder: response.hasOlder,
        endCursor: response.endCursor ?? null,
        totalCount: String(response.totalCount),
        scannedCount: response.scannedCount,
        scanLimit: response.scanLimit,
        scanLimited: response.scanLimited
      };
    },

    async listEventTypes(options: { signal?: AbortSignal } = {}): Promise<string[]> {
      const response = await client.listEventTypes(
        {},
        { ...(options.signal ? { signal: options.signal } : {}) }
      );
      return [...response.eventTypes];
    },

    async getEvent(
      sequence: string,
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminEventLogEntry | null> {
      try {
        const response = await client.getEvent(
          { sequence },
          { ...(options.signal ? { signal: options.signal } : {}) }
        );
        return response.entry ? adminEventLogEntry(response.entry) : null;
      } catch (error) {
        if (error instanceof ConnectError && error.code === Code.NotFound) return null;
        throw error;
      }
    }
  };
}

function eventLogFilterInput(filter: AdminEventLogFilter) {
  if (!hasActiveEventLogFilter(filter)) return undefined;
  return {
    eventType: filter.eventType || undefined,
    actorId: filter.actorId || undefined,
    createdAtFrom: timestampFromISO(filter.createdAtFrom),
    createdAtTo: timestampFromISO(filter.createdAtTo)
  };
}

function hasActiveEventLogFilter(filter: AdminEventLogFilter): boolean {
  return Boolean(filter.eventType || filter.actorId || filter.createdAtFrom || filter.createdAtTo);
}

function timestampFromISO(value: string): Timestamp | undefined {
  if (!value) return undefined;
  return Timestamp.fromDate(new Date(value));
}

function adminEventLogEntry(entry: APIAdminEventLogEntry): AdminEventLogEntry {
  return {
    sequence: entry.sequence,
    subject: entry.subject,
    aggregateType: entry.aggregateType,
    aggregateId: entry.aggregateId,
    eventType: entry.eventType,
    eventId: entry.eventId,
    actorId: entry.actorId,
    createdAt: entry.createdAt?.toDate().toISOString() ?? '',
    payloadJson: entry.payloadJson
  };
}
