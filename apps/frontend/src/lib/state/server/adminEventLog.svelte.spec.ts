import { describe, expect, it, vi } from 'vitest';
import { AdminEventLogStore, type AdminEventLogFilter } from './adminEventLog.svelte';
import type {
  AdminEventLogAPI,
  AdminEventLogEntry,
  AdminEventLogPage
} from '@chatto/api-client/adminEventLog';

function makeEntry(sequence: string, eventType = 'LoginSucceededEvent'): AdminEventLogEntry {
  return {
    sequence,
    subject: `evt.auth.server.${sequence}`,
    aggregateType: 'auth',
    aggregateId: 'server',
    eventType,
    eventId: `event-${sequence}`,
    actorId: 'actor-1',
    createdAt: '2026-01-01T12:00:00.000Z',
    payloadJson: `{"id":"event-${sequence}"}`
  };
}

function makePage(entries = [makeEntry('10')]): AdminEventLogPage {
  return {
    entries,
    hasOlder: true,
    endCursor: entries.at(-1)?.sequence ?? null,
    totalCount: '42',
    scannedCount: 100,
    scanLimit: 5000,
    scanLimited: false
  };
}

function makeAPI(overrides: Partial<AdminEventLogAPI> = {}): AdminEventLogAPI {
  return {
    listEvents: vi.fn().mockResolvedValue(makePage()),
    listEventTypes: vi.fn().mockResolvedValue(['LoginSucceededEvent']),
    getEvent: vi.fn().mockResolvedValue(makeEntry('10')),
    ...overrides
  };
}

const filter: AdminEventLogFilter = {
  eventType: 'LoginSucceededEvent',
  actorId: 'actor-1',
  createdAtFrom: '2026-01-01T00:00:00.000Z',
  createdAtTo: '2026-01-02T00:00:00.000Z'
};

describe('AdminEventLogStore', () => {
  it('loads filtered pages with scan metadata', async () => {
    const api = makeAPI();
    const store = new AdminEventLogStore(api);

    await store.loadFirstPage(filter);

    expect(api.listEvents).toHaveBeenCalledWith({
      limit: 50,
      before: null,
      filter
    });
    expect(store.entries).toHaveLength(1);
    expect(store.totalCount).toBe('42');
    expect(store.scannedCount).toBe(100);
    expect(store.scanLimit).toBe(5000);
    expect(store.hasOlder).toBe(true);
    expect(store.compatibilityMessage).toBeNull();
  });

  it('keeps load-more available when a filtered scan window is capped', async () => {
    const api = makeAPI({
      listEvents: vi
        .fn()
        .mockResolvedValueOnce({
          ...makePage([]),
          hasOlder: true,
          endCursor: '101',
          scanLimited: true
        })
        .mockResolvedValueOnce(makePage([makeEntry('90')]))
    });
    const store = new AdminEventLogStore(api);

    await store.loadFirstPage(filter);

    expect(store.entries).toHaveLength(0);
    expect(store.scanLimited).toBe(true);
    expect(store.endCursor).toBe('101');
    expect(store.hasOlder).toBe(true);

    await store.loadMore();

    expect(api.listEvents).toHaveBeenNthCalledWith(2, {
      limit: 50,
      before: '101',
      filter
    });
    expect(store.entries[0].sequence).toBe('90');
  });

  it('loads event type suggestions when available', async () => {
    const api = makeAPI({
      listEventTypes: vi.fn().mockResolvedValue(['LoginSucceededEvent', 'decode-error'])
    });
    const store = new AdminEventLogStore(api);

    await store.loadEventTypes();

    expect(store.eventTypesUnsupported).toBe(false);
    expect(store.eventTypes).toEqual(['LoginSucceededEvent', 'decode-error']);
  });

  it('treats event type suggestions as optional', async () => {
    const api = makeAPI({
      listEventTypes: vi.fn().mockRejectedValue(new Error('permission denied'))
    });
    const store = new AdminEventLogStore(api);

    await store.loadEventTypes();

    expect(store.eventTypesUnsupported).toBe(false);
    expect(store.eventTypes).toEqual([]);
  });

  it('loads a single event through the shared API', async () => {
    const api = makeAPI({
      getEvent: vi.fn().mockResolvedValue(makeEntry('77', 'UserJoinedRoomEvent'))
    });
    const store = new AdminEventLogStore(api);

    await expect(store.getEvent('77')).resolves.toMatchObject({
      sequence: '77',
      eventType: 'UserJoinedRoomEvent'
    });
    expect(api.getEvent).toHaveBeenCalledWith('77');
  });
});
