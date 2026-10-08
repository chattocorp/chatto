import { describe, expect, it } from 'vitest';
import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';
import { highestLimitUsage, overallHealth, systemHealthChecks } from './systemHealth';

function projection(overrides: Partial<AdminSystemInfo['projections'][number]> = {}) {
  return {
    key: 'rooms',
    name: 'Rooms',
    subjects: [],
    started: true,
    startupDurationSeconds: 0.1,
    lastAppliedSequence: '10',
    matchingStreamSequence: '10',
    streamLastSequence: '10',
    lag: 0,
    failed: false,
    failedSequence: '0',
    failure: '',
    entryCount: 1,
    estimatedBytes: 1,
    averageEntryBytes: 1,
    metrics: [],
    ...overrides
  };
}

function worker(health: AdminSystemInfo['durableWorkers'][number]['health']) {
  return {
    key: `worker-${health}`,
    health,
    pendingCount: '0',
    ackPendingCount: '0',
    waitingCount: '1',
    redeliveredCount: '0',
    lastDeliveredSequence: '0',
    ackFloorSequence: '0'
  };
}

function info(overrides: Partial<AdminSystemInfo> = {}): AdminSystemInfo {
  return {
    connection: {
      connected: true,
      serverId: 'N1',
      serverName: 'nats-1',
      version: '2.11.0',
      maxPayload: 1024,
      rtt: '1ms'
    },
    account: {
      memory: -1,
      memoryUsed: 0,
      storage: -1,
      storageUsed: 0,
      streams: -1,
      streamsUsed: 1,
      consumers: -1,
      consumersUsed: 1
    },
    accountAvailable: true,
    nats: {
      totalMessages: 0,
      totalBytes: 0,
      totalConsumerPending: 0,
      totalAckPending: 0,
      streams: [],
      consumers: []
    },
    natsAvailable: true,
    stats: { userCount: 0, channelRoomCount: 0, dmRoomCount: 0 },
    statsAvailable: true,
    projections: [projection()],
    projectionsAvailable: true,
    assetCleanup: {
      available: true,
      health: 'healthy',
      pendingCount: 0,
      oldestPendingAt: null,
      passInProgress: false,
      lastPassAt: null,
      lastSuccessfulPassAt: null,
      updatedAt: null,
      lastPassFailed: false,
      lastInspectedSequence: '0',
      latestDeletionSequence: '0'
    },
    durableWorkers: [worker('healthy'), worker('inactive')],
    ...overrides
  };
}

function statusOf(snapshot: AdminSystemInfo) {
  return Object.fromEntries(systemHealthChecks(snapshot).map((check) => [check.id, check.status]));
}

describe('systemHealthChecks', () => {
  it('reports a healthy snapshot as ok', () => {
    const checks = systemHealthChecks(info());

    expect(checks.map((check) => check.status)).toEqual(['ok', 'ok', 'ok', 'ok', 'ok']);
    expect(overallHealth(checks)).toBe('ok');
    expect(checks.find((check) => check.id === 'workers')?.detail).toEqual({
      kind: 'workers_running',
      count: 1
    });
  });

  it('treats a lost broker connection, failed projections, and stalled workers as critical', () => {
    const snapshot = info({
      connection: { ...info().connection, connected: false },
      projections: [projection(), projection({ key: 'users', failed: true })],
      durableWorkers: [worker('healthy'), worker('stalled'), worker('unavailable')]
    });

    expect(statusOf(snapshot)).toMatchObject({
      broker: 'critical',
      projections: 'critical',
      workers: 'critical'
    });
    expect(systemHealthChecks(snapshot).find((check) => check.id === 'workers')?.detail).toEqual({
      kind: 'workers_failing',
      count: 2
    });
    expect(overallHealth(systemHealthChecks(snapshot))).toBe('critical');
  });

  it('keeps projection lag and consumer backlog as details of an ok status', () => {
    const snapshot = info({
      projections: [projection({ lag: 4 }), projection({ key: 'users', started: false })],
      nats: { ...info().nats, totalConsumerPending: 3, totalAckPending: 2 }
    });
    const checks = systemHealthChecks(snapshot);

    expect(overallHealth(checks)).toBe('ok');
    expect(checks.find((check) => check.id === 'projections')?.detail).toEqual({
      kind: 'projections_catching_up',
      count: 2
    });
    expect(checks.find((check) => check.id === 'backlog')?.detail).toEqual({
      kind: 'backlog_waiting',
      count: 5
    });
  });

  it('raises account usage near a limit to warning, then critical', () => {
    const warning = info({ account: { ...info().account, storage: 100, storageUsed: 80 } });
    const critical = info({ account: { ...info().account, consumers: 10, consumersUsed: 9 } });

    expect(statusOf(warning).storage).toBe('warning');
    expect(overallHealth(systemHealthChecks(warning))).toBe('warning');
    expect(statusOf(critical).storage).toBe('critical');
    expect(systemHealthChecks(critical).find((check) => check.id === 'storage')?.detail).toEqual({
      kind: 'storage_used',
      percent: 90
    });
  });

  it('marks unreported data as unknown without raising the overall status', () => {
    const snapshot = info({
      accountAvailable: false,
      natsAvailable: false,
      projectionsAvailable: false,
      // Without consumer state, the server reports every required worker as unavailable.
      durableWorkers: [worker('unavailable'), worker('inactive')]
    });

    expect(statusOf(snapshot)).toEqual({
      broker: 'ok',
      projections: 'unknown',
      workers: 'unknown',
      backlog: 'unknown',
      storage: 'unknown'
    });
    expect(overallHealth(systemHealthChecks(snapshot))).toBe('ok');
  });
});

describe('systemHealthChecks edge cases', () => {
  it('keeps unconfirmed workers ok but does not count them as running', () => {
    const checks = systemHealthChecks(
      info({ durableWorkers: [worker('healthy'), worker('unconfirmed'), worker('working')] })
    );
    const workers = checks.find((check) => check.id === 'workers');

    expect(workers).toEqual({
      id: 'workers',
      status: 'ok',
      detail: { kind: 'workers_unconfirmed', count: 1 }
    });
  });

  it('uses inclusive thresholds and handles usage above a limit', () => {
    const at = (storageUsed: number) =>
      statusOf(info({ account: { ...info().account, storage: 100, storageUsed } })).storage;

    expect(at(74)).toBe('ok');
    expect(at(75)).toBe('warning');
    expect(at(89)).toBe('warning');
    expect(at(90)).toBe('critical');
    expect(at(120)).toBe('critical');
  });

  it('treats an empty projection list as unreported', () => {
    expect(statusOf(info({ projections: [] })).projections).toBe('unknown');
  });
});

describe('highestLimitUsage', () => {
  it('ignores unlimited resources and returns null when nothing is limited', () => {
    expect(highestLimitUsage(info().account)).toBeNull();
    expect(
      highestLimitUsage({
        ...info().account,
        memory: 200,
        memoryUsed: 50,
        streams: 4,
        streamsUsed: 1
      })
    ).toBe(0.25);
  });
});
