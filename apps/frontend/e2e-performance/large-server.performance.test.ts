import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import {
  expect,
  test,
  type APIRequestContext,
  type Browser,
  type TestInfo
} from '@playwright/test';
import { connectPost } from '../e2e/fixtures/connectHelpers';
import { startServer, stopServer, type ServerInfo } from '../e2e/fixtures/server';
import { composerTestStorageState, loginAsAdmin } from '../e2e/fixtures/testUser';
import { RoomPage } from '../e2e/pages';
import * as routes from '../e2e/routes';

interface PerformanceFixtureManifest {
  version: string;
  syntheticUsers: number;
  messages: number;
  roomId: string;
  roomName: string;
  firstUserLogin: string;
  lastUserLogin: string;
  lastMessageId: string;
  lastMessageBody: string;
  seedDurationMs: number;
}

interface ListMembersResponse {
  members?: Array<{ user?: { login?: string } }>;
  page?: { totalCount?: number | string; hasMore?: boolean };
}

interface PerformanceMeasurements {
  measurementVersion: string;
  sampleCount: number;
  fixtureVersion: string;
  syntheticUsers: number;
  messages: number;
  seedDurationMs: number;
  startupMode: 'fresh' | 'cold-replay';
  serverStartupMs: number;
  memberListApiMs: number;
  memberSearchApiMs: number;
  membersPageMs: number;
  roomPageMs: number;
  realtimeDeliveryMs: number;
}

interface PerformanceSample {
  memberListApiMs: number;
  memberSearchApiMs: number;
  membersPageMs: number;
  roomPageMs: number;
  realtimeDeliveryMs: number;
}

interface SampleStatistics {
  median: number;
  minimum: number;
  maximum: number;
}

const sampledMetricNames = [
  'memberListApiMs',
  'memberSearchApiMs',
  'membersPageMs',
  'roomPageMs',
  'realtimeDeliveryMs'
] as const;

const performanceMeasurementVersion = 'large-e2e-median-v1';

const syntheticUsers = integerEnvironment('CHATTO_E2E_PERF_USERS', 2048);
const messages = integerEnvironment('CHATTO_E2E_PERF_MESSAGES', 50_000);
const seedTimeoutMs = integerEnvironment('CHATTO_E2E_PERF_SEED_TIMEOUT_MS', 20 * 60_000);
const coldRestart = booleanEnvironment('CHATTO_E2E_PERF_COLD_RESTART', false);
const sampleCount = boundedIntegerEnvironment('CHATTO_E2E_PERF_SAMPLES', 5, 1, 20);

const ceilings = {
  serverStartupMs: integerEnvironment('CHATTO_E2E_PERF_MAX_STARTUP_MS', 120_000),
  memberListApiMs: integerEnvironment('CHATTO_E2E_PERF_MAX_MEMBER_LIST_API_MS', 15_000),
  memberSearchApiMs: integerEnvironment('CHATTO_E2E_PERF_MAX_MEMBER_SEARCH_API_MS', 15_000),
  membersPageMs: integerEnvironment('CHATTO_E2E_PERF_MAX_MEMBERS_PAGE_MS', 30_000),
  roomPageMs: integerEnvironment('CHATTO_E2E_PERF_MAX_ROOM_PAGE_MS', 30_000),
  realtimeDeliveryMs: integerEnvironment('CHATTO_E2E_PERF_MAX_REALTIME_MS', 15_000)
};

test('large loaded server stays responsive across directory, timeline, and realtime', async ({
  browser,
  request
}, testInfo) => {
  testInfo.annotations.push({
    type: 'fixture',
    description: `${syntheticUsers} synthetic users and ${messages} encrypted messages`
  });

  const prepared = await prepareFixture(request, testInfo);
  const { fixture, server } = prepared;

  try {
    const samples: PerformanceSample[] = [];
    for (let sample = 1; sample <= sampleCount; sample++) {
      samples.push(await measureLargeServer(browser, server, fixture, sample));
    }
    const statistics = summarizeSamples(samples);
    const measurements: PerformanceMeasurements = {
      measurementVersion: performanceMeasurementVersion,
      sampleCount,
      fixtureVersion: fixture.version,
      syntheticUsers: fixture.syntheticUsers,
      messages: fixture.messages,
      seedDurationMs: fixture.seedDurationMs,
      startupMode: coldRestart ? 'cold-replay' : 'fresh',
      serverStartupMs: server.startupDurationMs,
      memberListApiMs: statistics.memberListApiMs.median,
      memberSearchApiMs: statistics.memberSearchApiMs.median,
      membersPageMs: statistics.membersPageMs.median,
      roomPageMs: statistics.roomPageMs.median,
      realtimeDeliveryMs: statistics.realtimeDeliveryMs.median
    };
    await attachMeasurements(testInfo, measurements, samples, statistics);
    await attachServerMetrics(request, testInfo, server);

    expect(
      measurements.serverStartupMs,
      `${measurements.startupMode} server startup`
    ).toBeLessThanOrEqual(ceilings.serverStartupMs);
    expect(measurements.memberListApiMs, 'unfiltered member-list API').toBeLessThanOrEqual(
      ceilings.memberListApiMs
    );
    expect(measurements.memberSearchApiMs, 'member-search API').toBeLessThanOrEqual(
      ceilings.memberSearchApiMs
    );
    expect(measurements.membersPageMs, 'members page ready').toBeLessThanOrEqual(
      ceilings.membersPageMs
    );
    expect(measurements.roomPageMs, 'large timeline page ready').toBeLessThanOrEqual(
      ceilings.roomPageMs
    );
    expect(
      measurements.realtimeDeliveryMs,
      'receiver-visible realtime message'
    ).toBeLessThanOrEqual(ceilings.realtimeDeliveryMs);
  } finally {
    await stopServer(server, testInfo);
  }
});

async function prepareFixture(
  request: APIRequestContext,
  testInfo: TestInfo
): Promise<{ fixture: PerformanceFixtureManifest; server: ServerInfo }> {
  const seedServer = await startServer(testInfo, {
    instanceId: 'performance',
    metrics: true,
    startupTimeoutMs: 60_000
  });
  let fixtureReady = false;
  try {
    const response = await request.post(`${seedServer.baseURL}/auth/test/seed-performance`, {
      data: { users: syntheticUsers, messages },
      timeout: seedTimeoutMs
    });
    if (!response.ok()) {
      throw new Error(
        `performance fixture seed failed: ${response.status()} ${await response.text()}`
      );
    }
    const fixture = (await response.json()) as PerformanceFixtureManifest;
    expect(fixture.syntheticUsers).toBe(syntheticUsers);
    expect(fixture.messages).toBe(messages);
    expect(fixture.roomId).not.toBe('');
    expect(fixture.lastMessageId).not.toBe('');
    fixtureReady = true;
    if (!coldRestart) return { fixture, server: seedServer };

    seedServer.preserveDataDirectory = true;
    await stopServer(seedServer, testInfo);
    const server = await startServer(testInfo, {
      instanceId: 'performance-replay',
      dataDirectory: seedServer.dataDir,
      reuseDataDirectory: true,
      metrics: true,
      startupTimeoutMs: ceilings.serverStartupMs
    });
    return { fixture, server };
  } finally {
    if (!fixtureReady) await stopServer(seedServer, testInfo);
  }
}

async function measureLargeServer(
  browser: Browser,
  server: ServerInfo,
  fixture: PerformanceFixtureManifest,
  sample: number
): Promise<PerformanceSample> {
  const storageState = composerTestStorageState(server.baseURL);
  const context = await browser.newContext({ baseURL: server.baseURL, storageState });
  const receiverContext = await browser.newContext({ baseURL: server.baseURL, storageState });
  try {
    const page = await context.newPage();
    await loginAsAdmin(page);

    const memberListStarted = performance.now();
    const members = await connectPost<ListMembersResponse>(
      page,
      'chatto.admin.v1.AdminUserService/ListMembers',
      { page: { limit: 100 } }
    );
    const memberListApiMs = performance.now() - memberListStarted;
    const totalMembers = Number(members.page?.totalCount ?? 0);
    expect(totalMembers).toBeGreaterThanOrEqual(fixture.syntheticUsers + 1);
    expect(members.members?.length).toBe(Math.min(100, totalMembers));

    const memberSearchStarted = performance.now();
    const memberSearch = await connectPost<ListMembersResponse>(
      page,
      'chatto.admin.v1.AdminUserService/ListMembers',
      { search: fixture.lastUserLogin, page: { limit: 20 } }
    );
    const memberSearchApiMs = performance.now() - memberSearchStarted;
    expect(Number(memberSearch.page?.totalCount)).toBe(1);
    expect(memberSearch.members?.[0]?.user?.login).toBe(fixture.lastUserLogin);

    const membersPageStarted = performance.now();
    await page.goto(routes.serverAdminMembers);
    await expect(page.getByRole('heading', { name: 'Members', exact: true })).toBeVisible();
    await expect(page.getByText(`@${fixture.firstUserLogin}`, { exact: true })).toBeVisible();
    const membersPageMs = performance.now() - membersPageStarted;

    const roomPageStarted = performance.now();
    await page.goto(routes.room(fixture.roomId));
    const senderRoom = new RoomPage(page);
    await senderRoom.expectMessageVisible(fixture.lastMessageBody);
    const roomPageMs = performance.now() - roomPageStarted;

    const receiverPage = await receiverContext.newPage();
    await loginAsAdmin(receiverPage);
    await receiverPage.goto(routes.room(fixture.roomId));
    const receiverRoom = new RoomPage(receiverPage);
    await receiverRoom.expectMessageVisible(fixture.lastMessageBody);

    const liveBody = `Performance live delivery sample ${sample} ${Date.now()}`;
    const realtimeStarted = performance.now();
    await senderRoom.sendMessage(liveBody);
    await receiverRoom.expectMessageVisible(liveBody);
    const realtimeDeliveryMs = performance.now() - realtimeStarted;

    return {
      memberListApiMs,
      memberSearchApiMs,
      membersPageMs,
      roomPageMs,
      realtimeDeliveryMs
    };
  } finally {
    await receiverContext.close();
    await context.close();
  }
}

async function attachMeasurements(
  testInfo: TestInfo,
  measurements: PerformanceMeasurements,
  samples: PerformanceSample[],
  statistics: Record<(typeof sampledMetricNames)[number], SampleStatistics>
): Promise<void> {
  const json = `${JSON.stringify({ measurements, samples, statistics, ceilings }, null, 2)}\n`;
  const attachmentPath = testInfo.outputPath('performance-results.json');
  writeFileSync(attachmentPath, json);
  const exportPath = process.env.CHATTO_E2E_PERF_RESULT_PATH;
  if (exportPath) {
    const absoluteExportPath = resolve(exportPath);
    mkdirSync(dirname(absoluteExportPath), { recursive: true });
    writeFileSync(absoluteExportPath, json);
  }
  await testInfo.attach('performance-results', {
    path: attachmentPath,
    contentType: 'application/json'
  });
}

function summarizeSamples(
  samples: PerformanceSample[]
): Record<(typeof sampledMetricNames)[number], SampleStatistics> {
  return Object.fromEntries(
    sampledMetricNames.map((metric) => {
      const values = samples.map((sample) => sample[metric]).sort((a, b) => a - b);
      return [
        metric,
        {
          median: median(values),
          minimum: values[0],
          maximum: values[values.length - 1]
        }
      ];
    })
  ) as Record<(typeof sampledMetricNames)[number], SampleStatistics>;
}

function median(sortedValues: number[]): number {
  const middle = Math.floor(sortedValues.length / 2);
  if (sortedValues.length % 2 === 1) return sortedValues[middle];
  return (sortedValues[middle - 1] + sortedValues[middle]) / 2;
}

async function attachServerMetrics(
  request: APIRequestContext,
  testInfo: TestInfo,
  server: ServerInfo
): Promise<void> {
  if (!server.metricsURL) return;
  const response = await request.get(`${server.metricsURL}/metrics`);
  if (!response.ok()) return;
  await testInfo.attach('server-metrics', {
    body: await response.body(),
    contentType: 'text/plain'
  });
}

function integerEnvironment(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const value = Number.parseInt(raw, 10);
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer, got ${JSON.stringify(raw)}`);
  }
  return value;
}

function boundedIntegerEnvironment(
  name: string,
  fallback: number,
  minimum: number,
  maximum: number
): number {
  const value = integerEnvironment(name, fallback);
  if (value < minimum || value > maximum) {
    throw new Error(`${name} must be between ${minimum} and ${maximum}, got ${value}`);
  }
  return value;
}

function booleanEnvironment(name: string, fallback: boolean): boolean {
  const raw = process.env[name];
  if (!raw) return fallback;
  if (raw === 'true' || raw === '1') return true;
  if (raw === 'false' || raw === '0') return false;
  throw new Error(`${name} must be true, false, 1, or 0, got ${JSON.stringify(raw)}`);
}
