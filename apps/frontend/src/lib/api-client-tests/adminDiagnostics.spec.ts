import { protoInt64 } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getAdminSystemInfo } from '$lib/api-client/adminDiagnostics';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getSystemInfo: vi.fn()
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

describe('getAdminSystemInfo', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.getSystemInfo.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getSystemInfo: mocks.getSystemInfo
    });
  });

  it('loads admin diagnostics and maps int64 and optional fields', async () => {
    mocks.getSystemInfo.mockResolvedValue({
      systemInfo: {
        connection: {
          connected: true,
          serverId: 'nats-server-id',
          serverName: 'nats-server',
          version: '2.11.0',
          maxPayload: protoInt64.parse(1048576),
          rtt: '2ms'
        },
        account: {
          memory: protoInt64.parse(1000),
          memoryUsed: protoInt64.parse(250),
          storage: protoInt64.parse(2000),
          storageUsed: protoInt64.parse(750),
          streams: 10,
          streamsUsed: 3,
          consumers: 20,
          consumersUsed: 4
        },
        nats: {
          totalMessages: protoInt64.parse(12),
          totalBytes: protoInt64.parse(3456),
          totalConsumerPending: protoInt64.parse(7),
          totalAckPending: 2,
          streams: [
            {
              name: 'EVT',
              description: 'events',
              subjects: ['EVT.>'],
              storage: 'File',
              messages: protoInt64.parse(12),
              bytes: protoInt64.parse(3456),
              firstSequence: '1',
              lastSequence: '12',
              consumerCount: 1,
              replicas: 1,
              clusterLeader: 'leader'
            }
          ],
          consumers: [
            {
              stream: 'EVT',
              name: 'projection',
              durable: 'projection',
              filterSubject: 'EVT.>',
              filterSubjects: [],
              ackPolicy: 'Explicit',
              pullBased: true,
              pushBound: false,
              pending: protoInt64.parse(7),
              ackPending: 2,
              redelivered: 1,
              waiting: 0,
              deliveredConsumerSequence: '10',
              deliveredStreamSequence: '10',
              ackFloorConsumerSequence: '8',
              ackFloorStreamSequence: '8'
            }
          ]
        },
        stats: {
          userCount: 5,
          channelRoomCount: 3,
          dmRoomCount: 2
        }
      },
      projections: [
        {
          key: 'rooms',
          name: 'Rooms',
          subjects: ['EVT.room.>'],
          started: true,
          startupDurationSeconds: 0.25,
          lastAppliedSequence: '12',
          matchingStreamSequence: '12',
          streamLastSequence: '14',
          lag: protoInt64.parse(0),
          failed: false,
          failedSequence: '0',
          failure: '',
          entryCount: protoInt64.parse(3),
          estimatedBytes: protoInt64.parse(768),
          averageEntryBytes: protoInt64.parse(256),
          metrics: [
            {
              name: 'rooms',
              value: protoInt64.parse(3),
              bytes: protoInt64.parse(768)
            }
          ]
        },
        {
          key: 'pending',
          name: 'Pending',
          subjects: [],
          started: false,
          lastAppliedSequence: '0',
          matchingStreamSequence: '0',
          streamLastSequence: '14',
          lag: protoInt64.parse(0),
          failed: false,
          failedSequence: '0',
          failure: '',
          entryCount: protoInt64.zero,
          estimatedBytes: protoInt64.zero,
          averageEntryBytes: protoInt64.zero,
          metrics: []
        }
      ]
    });

    const info = await getAdminSystemInfo({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.getSystemInfo).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(info.connection.maxPayload).toBe(1048576);
    expect(info.account.storageUsed).toBe(750);
    expect(info.nats.totalMessages).toBe(12);
    expect(info.nats.streams[0].bytes).toBe(3456);
    expect(info.nats.consumers[0].pending).toBe(7);
    expect(info.stats.userCount).toBe(5);
    expect(info.projections[0].startupDurationSeconds).toBe(0.25);
    expect(info.projections[0].metrics[0].bytes).toBe(768);
    expect(info.projections[1].startupDurationSeconds).toBeNull();
  });

  it('maps missing nested sections to empty defaults and omits auth headers without a token', async () => {
    mocks.getSystemInfo.mockResolvedValue({
      projections: []
    });

    const info = await getAdminSystemInfo({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    expect(mocks.getSystemInfo).toHaveBeenCalledWith({}, { headers: undefined });
    expect(info.connection.connected).toBe(false);
    expect(info.account.storageUsed).toBe(0);
    expect(info.nats.streams).toEqual([]);
    expect(info.stats.userCount).toBe(0);
    expect(info.projections).toEqual([]);
  });
});
