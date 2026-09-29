import { protoInt64, Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminEventLogService } from '@chatto/api-types/admin/v1/event_log_connect';
import { createAdminEventLogAPI } from './adminEventLog';
import { fakeServer, mockService, receivedRequest } from '@chatto/client/testing/fakeServer';

const mocks = mockService(AdminEventLogService);

function eventLogAPI() {
  return createAdminEventLogAPI(
    fakeServer((router) => router.service(AdminEventLogService, mocks))
  );
}

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
    vi.resetAllMocks();
  });

  it('lists filtered events and maps int64 and timestamps', async () => {
    mocks.listEvents.mockReturnValue({
      entries: [apiEntry('12')],
      hasOlder: true,
      endCursor: '12',
      totalCount: protoInt64.parse('9007199254740993'),
      scannedCount: 50,
      scanLimit: 5000,
      scanLimited: true
    });
    const api = eventLogAPI();

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

    expect(receivedRequest(mocks.listEvents)).toMatchObject({
      limit: 50,
      before: '20',
      filter: {
        eventType: 'UserJoinedRoomEvent',
        actorId: 'actor-1',
        createdAtFrom: Timestamp.fromDate(new Date('2026-01-01T00:00:00.000Z')),
        createdAtTo: Timestamp.fromDate(new Date('2026-01-02T00:00:00.000Z'))
      }
    });
    expect(page.totalCount).toBe('9007199254740993');
    expect(page.entries[0]).toMatchObject({
      sequence: '12',
      eventType: 'UserJoinedRoomEvent',
      createdAt: '2026-01-01T12:00:00.000Z'
    });
    expect(page.scanLimited).toBe(true);
  });

  it('omits empty filters', async () => {
    mocks.listEvents.mockReturnValue({
      entries: [],
      hasOlder: false,
      totalCount: protoInt64.zero,
      scannedCount: 0,
      scanLimit: 50,
      scanLimited: false
    });
    const api = eventLogAPI();

    const page = await api.listEvents({ limit: 50 });

    expect(receivedRequest(mocks.listEvents)).toMatchObject({ limit: 50, before: undefined });
    expect(receivedRequest(mocks.listEvents)?.filter).toBeUndefined();
    expect(page.entries).toEqual([]);
    expect(page.endCursor).toBeNull();
  });

  it('lists event types and gets one event', async () => {
    mocks.listEventTypes.mockReturnValue({
      eventTypes: ['UserJoinedRoomEvent', 'decode-error']
    });
    mocks.getEvent.mockReturnValue({
      entry: apiEntry('7')
    });
    const api = eventLogAPI();

    await expect(api.listEventTypes()).resolves.toEqual(['UserJoinedRoomEvent', 'decode-error']);
    await expect(api.getEvent('7')).resolves.toMatchObject({
      sequence: '7',
      payloadJson: '{"id":"event-7"}'
    });
    expect(mocks.listEventTypes).toHaveBeenCalledOnce();
    expect(receivedRequest(mocks.getEvent)).toMatchObject({ sequence: '7' });
  });

  it('maps a missing event to null', async () => {
    mocks.getEvent.mockImplementation(() => {
      throw new ConnectError('not found', Code.NotFound);
    });
    const api = eventLogAPI();

    await expect(api.getEvent('404')).resolves.toBeNull();
  });
});
