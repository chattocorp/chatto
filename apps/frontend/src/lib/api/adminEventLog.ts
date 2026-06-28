import { Timestamp } from '@bufbuild/protobuf';
import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AdminEventLogService } from '$lib/pb/chatto/api/v1/admin_event_log_connect';
import type { AdminEventLogEntry as APIAdminEventLogEntry } from '$lib/pb/chatto/api/v1/admin_event_log_pb';

export type AdminEventLogAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
};

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

export function createAdminEventLogAPI(config: AdminEventLogAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(AdminEventLogService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  return {
    async listEvents(input: {
      limit: number;
      before?: string | null;
      filter?: AdminEventLogFilter;
    }): Promise<AdminEventLogPage> {
      const response = await client.listEvents(
        {
          limit: input.limit,
          before: input.before ?? undefined,
          filter: eventLogFilterInput(input.filter ?? EMPTY_ADMIN_EVENT_LOG_FILTER)
        },
        { headers: headers() }
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

    async listEventTypes(): Promise<string[]> {
      const response = await client.listEventTypes({}, { headers: headers() });
      return [...response.eventTypes];
    },

    async getEvent(sequence: string): Promise<AdminEventLogEntry | null> {
      const response = await client.getEvent({ sequence }, { headers: headers() });
      return optionalAdminEventLogEntry(response.entry);
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

function optionalAdminEventLogEntry(
  entry: APIAdminEventLogEntry | undefined
): AdminEventLogEntry | null {
  return entry ? adminEventLogEntry(entry) : null;
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
