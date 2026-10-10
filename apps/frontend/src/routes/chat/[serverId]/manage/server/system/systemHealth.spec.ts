import { describe, expect, it } from 'vitest';
import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';
import {
  highestLimitUsage,
  liveKitWebhooksRejected,
  overallHealth,
  systemHealthChecks
} from './systemHealth';

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
    livekit: liveKit(),
    ...overrides
  };
}

function liveKit(overrides: Partial<AdminSystemInfo['livekit']> = {}): AdminSystemInfo['livekit'] {
  return {
    connectionState: 'not_configured',
    connectionError: '',
    enabled: false,
    configured: false,
    url: '',
    apiKey: '',
    separateWebhookKey: false,
    webhookUrl: '',
    insecureUrl: false,
    lastWebhookAt: null,
    lastRejectedWebhookAt: null,
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

  it('rounds the shown percentage down so it matches the status', () => {
    const checks = systemHealthChecks(
      info({ account: { ...info().account, storage: 1000, storageUsed: 746 } })
    );

    expect(checks.find((check) => check.id === 'storage')).toEqual({
      id: 'storage',
      status: 'ok',
      detail: { kind: 'storage_used', percent: 74 }
    });

    const exact = systemHealthChecks(
      info({ account: { ...info().account, streams: 100, streamsUsed: 29 } })
    );
    expect(exact.find((check) => check.id === 'storage')?.detail).toEqual({
      kind: 'storage_used',
      percent: 29
    });
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

describe('voice calls health check', () => {
  const voiceCalls = (livekit: AdminSystemInfo['livekit']) =>
    systemHealthChecks(info({ livekit })).find((check) => check.id === 'voice_calls');

  it('omits the check when LiveKit is not configured or not reported', () => {
    expect(voiceCalls(liveKit())).toBeUndefined();
    expect(voiceCalls(liveKit({ connectionState: 'unavailable' }))).toBeUndefined();
  });

  it('reports a failed LiveKit API check as critical', () => {
    for (const connectionState of ['unreachable', 'unauthorized', 'error'] as const) {
      expect(voiceCalls(liveKit({ connectionState, configured: true }))).toEqual({
        id: 'voice_calls',
        status: 'critical',
        detail: { kind: `livekit_${connectionState}` }
      });
    }
  });

  it('warns about an insecure URL and rejected webhooks, otherwise ok', () => {
    const ok = liveKit({ connectionState: 'ok', configured: true });

    expect(voiceCalls(ok)?.status).toBe('ok');
    expect(voiceCalls({ ...ok, insecureUrl: true })?.detail).toEqual({
      kind: 'livekit_insecure_url'
    });
    expect(
      voiceCalls({ ...ok, lastRejectedWebhookAt: new Date('2026-07-10T12:00:00Z') })?.detail
    ).toEqual({ kind: 'livekit_webhooks_rejected' });
  });
});

describe('liveKitWebhooksRejected', () => {
  const earlier = new Date('2026-07-10T11:00:00Z');
  const later = new Date('2026-07-10T12:00:00Z');

  it('is true only when the last webhook was rejected', () => {
    expect(liveKitWebhooksRejected(liveKit())).toBe(false);
    expect(liveKitWebhooksRejected(liveKit({ lastRejectedWebhookAt: earlier }))).toBe(true);
    expect(
      liveKitWebhooksRejected(liveKit({ lastWebhookAt: earlier, lastRejectedWebhookAt: later }))
    ).toBe(true);
    expect(
      liveKitWebhooksRejected(liveKit({ lastWebhookAt: later, lastRejectedWebhookAt: earlier }))
    ).toBe(false);
  });
});
