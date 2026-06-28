import { protoInt64, Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createAdminEventLogAPI } from './adminEventLog';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listEvents: vi.fn(),
  listEventTypes: vi.fn(),
  getEvent: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

function apiEntry(sequence: string) {
  return {
    sequence,
    subject: `evt.room.room-1.${sequence}`,
    aggregateType: 'room',
    aggregateId: 'room-1',
    eventType: 'UserJoinedRoomEvent',
    eventId: `event-${sequence}`,
    actorId: 'actor-1',
    createdAt: Timestamp.fromDate(new Date('2026-01-01T12:00:00.000Z')),
    payloadJson: `{"id":"event-${sequence}"}`
  };
}

describe('createAdminEventLogAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listEvents.mockReset();
    mocks.listEventTypes.mockReset();
    mocks.getEvent.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listEvents: mocks.listEvents,
      listEventTypes: mocks.listEventTypes,
      getEvent: mocks.getEvent
    });
  });

  it('lists filtered events and maps int64 and timestamps', async () => {
    mocks.listEvents.mockResolvedValue({
      entries: [apiEntry('12')],
      hasOlder: true,
      endCursor: '12',
      totalCount: protoInt64.parse('9007199254740993'),
      scannedCount: 50,
      scanLimit: 5000,
      scanLimited: true
    });
    const api = createAdminEventLogAPI({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    });

    const page = await api.listEvents({
      limit: 50,
      before: '20',
      filter: {
        eventType: 'UserJoinedRoomEvent',
        actorId: 'actor-1',
        createdAtFrom: '2026-01-01T00:00:00.000Z',
        createdAtTo: '2026-01-02T00:00:00.000Z'
      }
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.listEvents).toHaveBeenCalledWith(
      {
        limit: 50,
        before: '20',
        filter: {
          eventType: 'UserJoinedRoomEvent',
          actorId: 'actor-1',
          createdAtFrom: Timestamp.fromDate(new Date('2026-01-01T00:00:00.000Z')),
          createdAtTo: Timestamp.fromDate(new Date('2026-01-02T00:00:00.000Z'))
        }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(page.totalCount).toBe('9007199254740993');
    expect(page.entries[0]).toMatchObject({
      sequence: '12',
      eventType: 'UserJoinedRoomEvent',
      createdAt: '2026-01-01T12:00:00.000Z'
    });
    expect(page.scanLimited).toBe(true);
  });

  it('omits empty filters and auth headers', async () => {
    mocks.listEvents.mockResolvedValue({
      entries: [],
      hasOlder: false,
      totalCount: protoInt64.zero,
      scannedCount: 0,
      scanLimit: 50,
      scanLimited: false
    });
    const api = createAdminEventLogAPI({ baseUrl: '/api/connect', bearerToken: null });

    const page = await api.listEvents({ limit: 50 });

    expect(mocks.listEvents).toHaveBeenCalledWith(
      {
        limit: 50,
        before: undefined,
        filter: undefined
      },
      { headers: undefined }
    );
    expect(page.entries).toEqual([]);
    expect(page.endCursor).toBeNull();
  });

  it('lists event types and gets one event', async () => {
    mocks.listEventTypes.mockResolvedValue({
      eventTypes: ['UserJoinedRoomEvent', 'decode-error']
    });
    mocks.getEvent.mockResolvedValue({
      entry: apiEntry('7')
    });
    const api = createAdminEventLogAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listEventTypes()).resolves.toEqual(['UserJoinedRoomEvent', 'decode-error']);
    await expect(api.getEvent('7')).resolves.toMatchObject({
      sequence: '7',
      payloadJson: '{"id":"event-7"}'
    });
    expect(mocks.listEventTypes).toHaveBeenCalledWith({}, { headers: undefined });
    expect(mocks.getEvent).toHaveBeenCalledWith({ sequence: '7' }, { headers: undefined });
  });
});
