import { describe, expect, it, vi } from 'vitest';
import type { Client } from '@urql/svelte';
import { AdminEventLogStore, type AdminEventLogFilter } from './adminEventLog.svelte';

function makeEntry(sequence: string, eventType = 'LoginSucceededEvent') {
  return {
    __typename: 'EventLogEntry' as const,
    sequence,
    subject: `evt.auth.server.${sequence}`,
    aggregateType: 'auth',
    aggregateId: 'server',
    eventType,
    eventId: `event-${sequence}`,
    actorId: 'actor-1',
    createdAt: '2026-01-01T12:00:00Z'
  };
}

function makeFilteredConnection(entries = [makeEntry('10')]) {
  return {
    __typename: 'EventLogConnection' as const,
    entries,
    hasOlder: true,
    endCursor: entries.at(-1)?.sequence ?? null,
    totalCount: 42,
    scannedCount: 100,
    scanLimit: 5000,
    scanLimited: false
  };
}

function makeLegacyConnection(entries = [makeEntry('9')]) {
  return {
    __typename: 'EventLogConnection' as const,
    entries,
    hasOlder: false,
    endCursor: entries.at(-1)?.sequence ?? null,
    totalCount: 42
  };
}

function makeClient(results: unknown[]): Client & { query: ReturnType<typeof vi.fn> } {
  const queue = [...results];
  return {
    query: vi.fn().mockImplementation(() => ({
      toPromise: vi.fn().mockResolvedValue(queue.shift())
    }))
  } as unknown as Client & { query: ReturnType<typeof vi.fn> };
}

const filter: AdminEventLogFilter = {
  eventType: 'LoginSucceededEvent',
  actorId: 'actor-1',
  createdAtFrom: '2026-01-01T00:00:00.000Z',
  createdAtTo: '2026-01-02T00:00:00.000Z'
};

describe('AdminEventLogStore', () => {
  it('loads filtered pages with scan metadata', async () => {
    const client = makeClient([
      {
        data: { admin: { eventLog: makeFilteredConnection() } },
        error: null
      }
    ]);
    const store = new AdminEventLogStore(client);

    await store.loadFirstPage(filter);

    expect(client.query).toHaveBeenCalledWith(expect.anything(), {
      limit: 50,
      before: null,
      filter: {
        eventType: 'LoginSucceededEvent',
        actorId: 'actor-1',
        createdAtFrom: '2026-01-01T00:00:00.000Z',
        createdAtTo: '2026-01-02T00:00:00.000Z'
      }
    });
    expect(store.entries).toHaveLength(1);
    expect(store.totalCount).toBe('42');
    expect(store.scannedCount).toBe(100);
    expect(store.scanLimit).toBe(5000);
    expect(store.hasOlder).toBe(true);
  });

  it('keeps load-more available when a filtered scan window is capped', async () => {
    const client = makeClient([
      {
        data: {
          admin: {
            eventLog: {
              ...makeFilteredConnection([]),
              hasOlder: true,
              endCursor: '101',
              scanLimited: true
            }
          }
        },
        error: null
      },
      {
        data: { admin: { eventLog: makeFilteredConnection([makeEntry('90')]) } },
        error: null
      }
    ]);
    const store = new AdminEventLogStore(client);

    await store.loadFirstPage(filter);

    expect(store.entries).toHaveLength(0);
    expect(store.scanLimited).toBe(true);
    expect(store.endCursor).toBe('101');
    expect(store.hasOlder).toBe(true);

    await store.loadMore();

    expect(client.query).toHaveBeenNthCalledWith(2, expect.anything(), {
      limit: 50,
      before: '101',
      filter: {
        eventType: 'LoginSucceededEvent',
        actorId: 'actor-1',
        createdAtFrom: '2026-01-01T00:00:00.000Z',
        createdAtTo: '2026-01-02T00:00:00.000Z'
      }
    });
    expect(store.entries[0].sequence).toBe('90');
  });

  it('falls back to the legacy unfiltered query when filters are unsupported', async () => {
    const client = makeClient([
      {
        data: null,
        error: { message: 'Unknown argument "filter" on field "AdminQueries.eventLog".' }
      },
      {
        data: { admin: { eventLog: makeLegacyConnection() } },
        error: null
      }
    ]);
    const store = new AdminEventLogStore(client);

    await store.loadFirstPage(filter);

    expect(client.query).toHaveBeenCalledTimes(2);
    expect(client.query).toHaveBeenNthCalledWith(2, expect.anything(), {
      limit: 50,
      before: null
    });
    expect(store.compatibilityMessage).toContain('does not support Event Log filters');
    expect(store.entries[0].sequence).toBe('9');
    expect(store.scannedCount).toBe(1);
    expect(store.scanLimited).toBe(false);
  });

  it('treats event type suggestions as optional for older servers', async () => {
    const client = makeClient([
      {
        data: null,
        error: { message: 'Cannot query field "eventLogEventTypes" on type "AdminQueries".' }
      }
    ]);
    const store = new AdminEventLogStore(client);

    await store.loadEventTypes();

    expect(store.eventTypesUnsupported).toBe(true);
    expect(store.eventTypes).toEqual([]);
  });
});
