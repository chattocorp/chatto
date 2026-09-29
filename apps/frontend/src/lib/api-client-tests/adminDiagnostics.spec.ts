import { protoInt64, Timestamp } from '@bufbuild/protobuf';
import {
  AdminAssetCleanupHealth,
  AdminDurableWorkerHealth
} from '@chatto/api-types/admin/v1/diagnostics_pb';
import { Code } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminDiagnosticsService } from '@chatto/api-types/admin/v1/diagnostics_connect';
import { getAdminSystemInfo } from '$lib/api-client/adminDiagnostics';
import { fakeServer, mockService } from '$lib/test-utils';

const mocks = mockService(AdminDiagnosticsService);

function config() {
  return fakeServer((router) => router.service(AdminDiagnosticsService, mocks));
}

describe('getAdminSystemInfo', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('loads admin diagnostics and maps int64 and optional fields', async () => {
    mocks.getSystemInfo.mockReturnValue({
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
      projectionsAvailable: true,
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
      ],
      assetCleanup: {
        health: AdminAssetCleanupHealth.RETRYING,
        pendingCount: protoInt64.parse(2),
        oldestPendingAt: Timestamp.fromDate(new Date('2026-07-10T10:00:00Z')),
        passInProgress: false,
        lastPassAt: Timestamp.fromDate(new Date('2026-07-10T11:00:00Z')),
        lastSuccessfulPassAt: Timestamp.fromDate(new Date('2026-07-10T09:00:00Z')),
        updatedAt: Timestamp.fromDate(new Date('2026-07-10T11:00:05Z')),
        lastPassFailed: true,
        lastInspectedSequence: '41',
        latestDeletionSequence: '44'
      },
      durableWorkers: [
        {
          key: 'user_key_shredding',
          health: AdminDurableWorkerHealth.WORKING,
          pendingCount: '2',
          ackPendingCount: '1',
          waitingCount: '1',
          redeliveredCount: '3',
          lastDeliveredSequence: '44',
          ackFloorSequence: '41'
        }
      ]
    });

    const info = await getAdminSystemInfo(config());

    expect(info.connection.maxPayload).toBe(1048576);
    expect(info.account.storageUsed).toBe(750);
    expect(info.accountAvailable).toBe(true);
    expect(info.nats.totalMessages).toBe(12);
    expect(info.natsAvailable).toBe(true);
    expect(info.nats.streams[0].bytes).toBe(3456);
    expect(info.nats.consumers[0].pending).toBe(7);
    expect(info.stats.userCount).toBe(5);
    expect(info.statsAvailable).toBe(true);
    expect(info.projections[0].startupDurationSeconds).toBe(0.25);
    expect(info.projectionsAvailable).toBe(true);
    expect(info.projections[0].metrics[0].bytes).toBe(768);
    expect(info.projections[1].startupDurationSeconds).toBeNull();
    expect(info.assetCleanup).toEqual({
      available: true,
      health: 'retrying',
      pendingCount: 2,
      oldestPendingAt: new Date('2026-07-10T10:00:00Z'),
      passInProgress: false,
      lastPassAt: new Date('2026-07-10T11:00:00Z'),
      lastSuccessfulPassAt: new Date('2026-07-10T09:00:00Z'),
      updatedAt: new Date('2026-07-10T11:00:05Z'),
      lastPassFailed: true,
      lastInspectedSequence: '41',
      latestDeletionSequence: '44'
    });
    expect(info.durableWorkers).toEqual([
      {
        key: 'user_key_shredding',
        health: 'working',
        pendingCount: '2',
        ackPendingCount: '1',
        waitingCount: '1',
        redeliveredCount: '3',
        lastDeliveredSequence: '44',
        ackFloorSequence: '41'
      }
    ]);
  });

  it('maps missing nested sections to empty defaults', async () => {
    mocks.getSystemInfo.mockReturnValue({
      projections: []
    });

    const info = await getAdminSystemInfo(config());

    expect(info.connection.connected).toBe(false);
    expect(info.account.storageUsed).toBe(0);
    expect(info.accountAvailable).toBe(false);
    expect(info.natsAvailable).toBe(false);
    expect(info.nats.streams).toEqual([]);
    expect(info.stats.userCount).toBe(0);
    expect(info.statsAvailable).toBe(false);
    expect(info.projections).toEqual([]);
    expect(info.projectionsAvailable).toBe(true);
    expect(info.assetCleanup).toEqual({
      available: false,
      health: 'unavailable',
      pendingCount: 0,
      oldestPendingAt: null,
      passInProgress: false,
      lastPassAt: null,
      lastSuccessfulPassAt: null,
      updatedAt: null,
      lastPassFailed: false,
      lastInspectedSequence: '0',
      latestDeletionSequence: '0'
    });
    expect(info.durableWorkers).toEqual([]);
  });

  it('treats explicitly unavailable cleanup diagnostics as unavailable', async () => {
    mocks.getSystemInfo.mockReturnValue({
      projections: [],
      projectionsAvailable: false,
      assetCleanup: {
        health: AdminAssetCleanupHealth.UNAVAILABLE,
        pendingCount: protoInt64.parse(7),
        lastInspectedSequence: '41',
        latestDeletionSequence: '44'
      }
    });

    const info = await getAdminSystemInfo(config());

    expect(info.assetCleanup.available).toBe(false);
    expect(info.assetCleanup.health).toBe('unavailable');
    expect(info.projectionsAvailable).toBe(false);
  });

  it('cancels the diagnostics read with the caller signal', async () => {
    await expect(
      getAdminSystemInfo(config(), { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });
});
