import { test, expect } from './setup';
import { loginAndEnterRoom, withServerUser } from './fixtures/serverUser';
import { TIMEOUTS } from './constants';

test('warm room posts share all reconciliation reads', async ({ page }) => {
  const { roomPage } = await loginAndEnterRoom(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  for (let index = 0; index < 2; index++) {
    const body = `Request accounting ${index}`;
    await roomPage.messageInput.fill(body);
    // Finish typing and initial hydration before measuring the posting cycle.
    await page.waitForLoadState('networkidle');
    const methods: string[] = [];
    const record = (request: import('@playwright/test').Request) => {
      if (request.url().includes('/api/connect/')) methods.push(request.url().split('/').at(-1)!);
    };
    page.on('request', record);
    await roomPage.messageInput.press('Control+Enter');
    await expect(roomPage.getMessage(body).locator).toBeVisible();
    // Measure through the read-marker settling interval, including delayed
    // follow-up requests. This is a deliberate wall-clock traffic measurement.
    await page.waitForTimeout(TIMEOUTS.SERVER_MUTATION_SYNC);
    await page.waitForLoadState('networkidle');
    page.off('request', record);
    expect(methods.sort()).toEqual([
      'BatchGetMessages', 'CreateMessage', 'ListNotificationOccurrences', 'ListRooms'
    ]);
  }
  expect(errors).toEqual([]);
});

test('a known remote author does not cause a user read on each received post', async ({ page, browser, serverURL }) => {
  const { roomPage } = await loginAndEnterRoom(page);
  await withServerUser(browser, serverURL, async ({ chatPage, roomPage: sender }) => {
    await chatPage.enterRoom('general');
    await sender.sendMessage('Warm remote author');
    await expect(roomPage.getMessage('Warm remote author').locator).toBeVisible();
    await page.waitForLoadState('networkidle');
    const users: string[] = [];
    page.on('request', (request) => {
      if (request.url().endsWith('/BatchGetUsers')) users.push(request.url());
    });
    await sender.sendMessage('Reuse remote author');
    await expect(roomPage.getMessage('Reuse remote author').locator).toBeVisible();
    await page.waitForTimeout(TIMEOUTS.SERVER_MUTATION_SYNC);
    await page.waitForLoadState('networkidle');
    expect(users).toEqual([]);
  });
});

test('warm thread replies reuse authors across command and realtime hydration', async ({ page }) => {
  const { roomPage } = await loginAndEnterRoom(page);
  const root = await roomPage.sendMessage('Thread request accounting');
  await root.openThread();
  await roomPage.postThreadReply('Warm the thread');
  await page.waitForLoadState('networkidle');
  await roomPage.threadReplyInput.fill('Measured thread reply');
  await page.waitForLoadState('networkidle');
  const methods: string[] = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/connect/')) methods.push(request.url().split('/').at(-1)!);
  });
  await roomPage.threadReplyInput.press('Control+Enter');
  await expect(page.getByText('Measured thread reply', { exact: true })).toBeVisible();
  // Include read-marker and notification reconciliation in the measurement.
  await page.waitForTimeout(TIMEOUTS.SERVER_MUTATION_SYNC);
  await page.waitForLoadState('networkidle');
  const counts = Object.fromEntries([...new Set(methods)].map((method) =>
    [method, methods.filter((value) => value === method).length]
  ));
  expect(Object.keys(counts).sort()).toEqual([
    'BatchGetMessages', 'CreateMessage', 'ListNotificationOccurrences', 'ListRooms', 'MarkThreadAsRead'
  ]);
  expect(counts.CreateMessage).toBe(1);
  expect(counts.MarkThreadAsRead).toBe(1);
  // A read-marker response that arrives after the post read starts needs a
  // follow-up read. It must not reload windows or refetch known users.
  for (const method of ['BatchGetMessages', 'ListRooms', 'ListNotificationOccurrences']) {
    expect(counts[method]).toBeLessThanOrEqual(2);
  }
});
