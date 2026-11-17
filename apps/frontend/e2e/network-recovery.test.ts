import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import type { WebSocket } from '@playwright/test';

/** Exercise HTTP failures independently of the realtime WebSocket. */
test('retries dropped timeline reads, shows sustained trouble, and recovers without a click', async ({
  page,
  chatPage,
  roomPage
}) => {
  test.setTimeout(60_000);
  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  const body = `Network recovery ${Date.now()}`;
  await roomPage.sendMessage(body);

  let attempts = 0;
  let fail = true;
  const sockets: WebSocket[] = [];
  page.on('websocket', (socket) => {
    if (socket.url().includes('/api/realtime')) sockets.push(socket);
  });
  await page.route('**/api/connect/chatto.api.v1.RoomService/GetRoomEvents', async (route) => {
    attempts++;
    if (fail) await route.abort('connectionfailed');
    else await route.continue();
  });
  await page.reload();

  const warning = page.getByRole('banner').getByRole('img', { name: 'Connection unavailable' });
  await expect(warning).toBeVisible();
  await expect(page.getByRole('alert').getByText('Network error. Please try again.')).toBeVisible();
  // Three attempts exhaust one read. Further attempts prove the mounted
  // error surface retries without a manual click while realtime stays open.
  await expect.poll(() => attempts).toBeGreaterThanOrEqual(9);
  await expect(warning).toHaveClass(/text-warning/);
  expect(sockets).toHaveLength(1);
  expect(sockets[0]!.isClosed()).toBe(false);

  fail = false;
  await expect(roomPage.getMessage(body).locator).toBeVisible();
  await expect(warning).toHaveCount(0);
  await expect(page.getByRole('alert')).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});

test('does not repeat a message write when its accepted response is lost', async ({
  page,
  chatPage,
  roomPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  let writes = 0;
  let responseLost = false;
  await page.route('**/api/connect/chatto.api.v1.MessageService/CreateMessage', async (route) => {
    writes++;
    // Commit the write, then lose its response. Retrying an arbitrary mutation
    // in this situation could apply it twice.
    const response = await route.fetch();
    expect(response.ok()).toBe(true);
    await route.abort('connectionfailed');
    responseLost = true;
  });
  const body = `Lost write response ${Date.now()}`;
  await roomPage.messageInput.fill(body);
  await roomPage.messageInput.press('Control+Enter');
  await expect.poll(() => responseLost).toBe(true);
  // Observe the full automatic retry window: the two read delays including
  // maximum jitter total 2.5 seconds. This is a wall-clock absence assertion.
  await page.waitForTimeout(3_000);
  expect(writes).toBe(1);
  await expect(roomPage.getMessage(body).locator).toHaveCount(1);
});

test('absorbs two dropped reads on a throttled connection without a manual retry', async ({
  page,
  context,
  chatPage,
  roomPage
}) => {
  test.setTimeout(60_000);
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  const body = `Throttled recovery ${Date.now()}`;
  await roomPage.sendMessage(body);
  let attempts = 0;
  await page.route('**/api/connect/chatto.api.v1.RoomService/GetRoomEvents', async (route) => {
    attempts++;
    if (attempts <= 2) await route.abort('connectionfailed');
    else await route.continue();
  });
  const cdp = await context.newCDPSession(page);
  await cdp.send('Network.enable');
  // Controlled latency and bandwidth, with deterministic request loss above.
  await cdp.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 400,
    downloadThroughput: 100_000,
    uploadThroughput: 50_000
  });
  try {
    await page.reload();
    await expect(roomPage.getMessage(body).locator).toBeVisible({ timeout: 30_000 });
    expect(attempts).toBe(3);
    await expect(
      page.getByRole('banner').getByRole('img', { name: 'Connection unavailable' })
    ).toHaveCount(0);
    await expect(page.getByRole('alert')).toHaveCount(0);
  } finally {
    await cdp.detach();
  }
});
