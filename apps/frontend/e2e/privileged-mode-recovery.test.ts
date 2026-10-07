// SPDX-License-Identifier: Apache-2.0

/** Exercise shield transitions across the real API, session store and realtime projection. */
import type { Page } from '@playwright/test';
import {
  RealtimeRecovery,
  RealtimeCloseCode,
  RealtimeServerFrame,
  RealtimeSubscribe
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { test as base, expect } from './setup';
import { createAndLoginTestUser, loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import {
  connectPost,
  connectPostResponse,
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect,
  postMessageViaConnect
} from './fixtures/connectHelpers';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { seedData } from './fixtures/seed';
import * as routes from './routes';

const viewerService = 'chatto.api.v1.ViewerService';
const readDenied = 'You do not have permission to read messages in this room.';

// Collect exceptions from every tab in the fixture context. HTTP failures are
// checked through UI errors rather than expected console network noise.
const test = base.extend<{ browserErrors: string[] }>({
  browserErrors: [
    async ({ context }, use) => {
      const errors: string[] = [];
      const observe = (page: Page) => page.on('pageerror', (error) => errors.push(error.message));
      context.pages().forEach(observe);
      context.on('page', observe);
      await use(errors);
      expect(errors, 'Unhandled browser exceptions').toEqual([]);
    },
    { auto: true }
  ]
});

/** Click the actual shield and its confirmation; wait until the operation finishes. */
async function activate(page: Page): Promise<void> {
  await page.getByTestId('privileged-mode-toggle').click();
  await page
    .getByRole('dialog', { name: 'Enable privileged mode' })
    .getByRole('button', { name: 'Enable privileged mode' })
    .click();
  await expect(page.getByRole('button', { name: 'Disable privileged mode' })).toBeEnabled();
}

async function deactivate(page: Page): Promise<void> {
  await page.getByRole('button', { name: 'Disable privileged mode' }).click();
  await expect(page.getByRole('button', { name: 'Enable privileged mode' })).toBeEnabled();
}

/** Keep membership while making message read/post authority depend on elevation. */
async function restrictedRoom(page: Page): Promise<{ id: string; body: string }> {
  const id = await createRoomViaConnect(
    page,
    'shield-vault',
    await getDefaultRoomGroupIdViaConnect(page)
  );
  const body = 'A message visible only with the owner override';
  await connectPost(page, 'chatto.api.v1.RoomService/JoinRoom', { roomId: id });
  await postMessageViaConnect(page, id, body);
  for (const permission of ['message.read', 'message.post']) {
    await connectPost(page, 'chatto.admin.v1.AdminPermissionService/SetRolePermission', {
      roleName: 'everyone',
      permission,
      decision: 'PERMISSION_DECISION_DENY',
      scope: { kind: 'PERMISSION_SCOPE_KIND_ROOM', id }
    });
  }
  return { id, body };
}

/** Check backend denial too, so hiding content or controls alone cannot pass. */
async function expectRestrictedAccessDenied(page: Page, roomId: string): Promise<void> {
  for (const [procedure, data] of [
    ['chatto.api.v1.RoomService/GetRoomEvents', { roomId, limit: 10 }],
    ['chatto.api.v1.MessageService/CreateMessage', { roomId, body: 'Must be denied' }]
  ] as const) {
    const response = await connectPostResponse(page, procedure, data);
    expect(response.status()).toBe(403);
    expect(await response.json()).toMatchObject({ code: 'permission_denied' });
  }
}

test('confirmation can be cancelled and pending activation prevents duplicate requests', async ({
  page
}) => {
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page, { activatePrivilegedMode: false });
  await page.goto(routes.serverOverview);
  let calls = 0;
  let release: (() => void) | undefined;
  const held = new Promise<void>((resolve) => (release = resolve));
  await page.route(`**/${viewerService}/ActivatePrivilegedMode`, async (route) => {
    calls++;
    await held;
    await route.continue();
  });

  const shield = page.getByTestId('privileged-mode-toggle');
  await shield.click();
  const dialog = page.getByRole('dialog', { name: 'Enable privileged mode' });
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(calls).toBe(0);
  await expect(page.getByRole('button', { name: 'New Group' })).not.toBeVisible();

  try {
    await shield.click();
    const confirm = dialog.getByRole('button', { name: 'Enable privileged mode' });
    await confirm.click();
    await expect.poll(() => calls).toBe(1);
    await expect(shield).toBeDisabled();
    await expect(confirm).toBeDisabled();
    // A second programmatic click on a disabled native control must send no RPC.
    await confirm.evaluate((button: HTMLButtonElement) => button.click());
    expect(calls).toBe(1);
    release!();
    await expect(dialog).not.toBeVisible();
    await expect(page.getByRole('button', { name: 'Disable privileged mode' })).toBeEnabled();
    await expect(page.getByRole('button', { name: 'New Group' })).toBeVisible();
    expect(calls).toBe(1);
  } finally {
    release!();
  }
});

for (const active of [true, false]) {
  test(`failed ${active ? 'activation' : 'deactivation'} preserves authority and can be retried`, async ({
    page
  }) => {
    await page.goto(routes.root);
    await loginAsAdminAndUsePrimaryServer(page, { activatePrivilegedMode: !active });
    await page.goto(routes.serverOverview);
    let failures = 0;
    const procedure = active ? 'ActivatePrivilegedMode' : 'DeactivatePrivilegedMode';
    await page.route(`**/${viewerService}/${procedure}`, async (route) => {
      if (failures++ === 0) {
        await route.fulfill({
          status: 503,
          contentType: 'application/json',
          body: JSON.stringify({ code: 'unavailable', message: 'Injected test failure' })
        });
      } else await route.continue();
    });
    await page.getByTestId('privileged-mode-toggle').click();
    const dialog = page.getByRole('dialog', { name: 'Enable privileged mode' });
    if (active) await dialog.getByRole('button', { name: 'Enable privileged mode' }).click();
    await expect(page.getByText('Something went wrong', { exact: true })).toBeVisible();
    await expect(page.getByTestId('privileged-mode-toggle')).toBeEnabled();
    const viewer = await connectPost<{ privilegedMode?: { active?: boolean } }>(
      page,
      `${viewerService}/GetViewer`
    );
    expect(viewer.privilegedMode?.active ?? false).toBe(!active);
    await expect(page.getByRole('button', { name: 'New Group' })).toBeVisible({ visible: !active });
    if (active) {
      await dialog.getByRole('button', { name: 'Enable privileged mode' }).click();
      await expect(page.getByRole('button', { name: 'Disable privileged mode' })).toBeEnabled();
    } else await deactivate(page);
    await expect(page.getByRole('button', { name: 'New Group' })).toBeVisible({ visible: active });
    expect(failures).toBe(2);
  });
}

for (const recovery of ['resume', 'snapshot', 'interrupted resume'] as const) {
  test(`open restricted room loses and regains read/post access through ${recovery}`, async ({
    page,
    roomPage
  }) => {
    let armed = false;
    let interrupted = false;
    const recoveries: RealtimeRecovery[] = [];
    await page.routeWebSocket('**/api/realtime', (socket) => {
      const server = socket.connectToServer();
      let observing = false;
      socket.onMessage((message) => {
        if (armed && typeof message !== 'string') {
          observing = true;
          const subscribe = RealtimeSubscribe.fromBinary(message);
          expect(subscribe.resumeCursor).toBeTruthy();
          if (recovery === 'snapshot') subscribe.resumeCursor = 'expired-shield-test';
          server.send(Buffer.from(subscribe.toBinary()));
        } else server.send(message);
      });
      server.onMessage((message) => {
        if (observing && typeof message !== 'string') {
          const frame = RealtimeServerFrame.fromBinary(message).frame;
          if (frame.case === 'caughtUp') recoveries.push(frame.value.recovery);
          if (frame.case === 'caughtUp' && recovery === 'interrupted resume' && !interrupted) {
            interrupted = true;
            void socket.close();
            return;
          }
        }
        socket.send(message);
      });
    });
    await page.goto(routes.root);
    await loginAsAdminAndUsePrimaryServer(page);
    const room = await restrictedRoom(page);
    await page.goto(routes.room(room.id));
    await waitForRoomReady(page);
    await expect(page.getByText(room.body, { exact: true })).toBeVisible();
    const composer = await roomPage.messageInput.elementHandle();
    const shell = await page.getByTestId('room-main-pane').elementHandle();
    const timeOrigin = await page.evaluate(() => performance.timeOrigin);
    armed = true;
    await deactivate(page);
    await expect(page.getByText(readDenied, { exact: true })).toBeVisible();
    await expect(page.getByText(room.body, { exact: true })).toHaveCount(0);
    await expect(page.locator('[role="article"]')).toHaveCount(0);
    await expect(roomPage.messageInput).toHaveAttribute('contenteditable', 'false');
    await expectRestrictedAccessDenied(page, room.id);
    expect(await composer!.evaluate((node) => node.isConnected)).toBe(true);
    expect(await shell!.evaluate((node) => node.isConnected)).toBe(true);
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(timeOrigin);
    await activate(page);
    await expect(page.getByText(readDenied, { exact: true })).not.toBeVisible();
    await expect(page.getByText(room.body, { exact: true })).toHaveCount(1);
    await roomPage.sendMessage(`Posting restored after ${recovery}`);
    expect(await shell!.evaluate((node) => node.isConnected)).toBe(true);
    expect(recoveries).toContain(
      recovery === 'snapshot' ? RealtimeRecovery.SNAPSHOT : RealtimeRecovery.RESUMED
    );
    expect(recoveries.length).toBeGreaterThanOrEqual(2);
    if (recovery === 'interrupted resume') expect(interrupted).toBe(true);
  });
}

test('shield resumes do not reload cached profiles or unrelated resources', async ({ page }) => {
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page, { activatePrivilegedMode: false });
  const seeded = await seedData(page.request, { seed: 46, users: 12, rooms: 1, messages: 36 });
  const room = seeded.rooms[0];
  await connectPost(page, 'chatto.api.v1.RoomService/JoinRoom', { roomId: room.id });
  await page.goto(routes.room(room.id));
  await waitForRoomReady(page);
  // Render a real message author, so the profile-reload assertion cannot pass
  // with an empty client user cache. Keep those profiles across navigation.
  const lastMessage = seeded.messages.at(-1)!;
  await expect(page.getByText(lastMessage.body, { exact: true })).toBeVisible();
  const author = seeded.users.find((user) => user.id === lastMessage.authorId)!;
  await expect(page.getByText(author.displayName, { exact: true }).first()).toBeVisible();
  await page.getByRole('link', { name: 'Overview', exact: true }).click();
  await expect(page.getByRole('button', { name: 'New Group' })).not.toBeVisible();
  const requests: string[] = [];
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().includes('/api/connect/')) {
      requests.push(new URL(request.url()).pathname.split('/').pop()!);
    }
  });
  for (const active of [true, false, true]) {
    requests.length = 0;
    if (active) await activate(page);
    else await deactivate(page);
    await expect(page.getByTestId('server-subscription-active')).toHaveAttribute(
      'data-projection-ready',
      'true'
    );
    for (const procedure of [
      'GetViewer',
      'BatchGetUsers',
      'GetMotd',
      'GetRuntimeConfig',
      'ListNotificationOccurrences'
    ]) {
      expect(requests, `Unexpected ${procedure} after shield transition`).not.toContain(procedure);
    }
    expect(requests).toContain(active ? 'ActivatePrivilegedMode' : 'DeactivatePrivilegedMode');
    expect(requests).toContain('ListRooms');
    expect(requests).toContain('ListRoomGroups');
  }
});

for (const serverDriven of [false, true]) {
  test(`automatic expiry removes open-room authority (${serverDriven ? 'server close' : 'normal clock'})`, async ({
    page,
    roomPage
  }) => {
    let serverRequestedExpiry = false;
    if (serverDriven) {
      // Hold browser Date fixed so its local deadline cannot initiate recovery.
      // The real server clock must send the expiry close and drive the transition.
      await page.clock.setFixedTime(new Date());
      await page.routeWebSocket('**/api/realtime', (socket) => {
        const server = socket.connectToServer();
        server.onMessage((message) => {
          if (typeof message !== 'string') {
            const frame = RealtimeServerFrame.fromBinary(message).frame;
            if (
              frame.case === 'close' &&
              frame.value.code === RealtimeCloseCode.PRIVILEGED_MODE_EXPIRED
            ) {
              expect(frame.value.reconnect).toBe(true);
              serverRequestedExpiry = true;
            }
          }
          socket.send(message);
        });
      });
    }
    await page.goto(routes.root);
    await loginAsAdminAndUsePrimaryServer(page);
    const room = await restrictedRoom(page);
    // Shorten actual stored authority, then load a socket and UI that both accept
    // that deadline. Ten seconds leaves room for snapshot hydration on CI.
    const response = await page.request.post('/auth/test/privileged-mode-deadline', {
      data: { remainingMs: 10_000 }
    });
    expect(response.ok()).toBeTruthy();
    await page.goto(routes.room(room.id));
    await waitForRoomReady(page);
    await expect(page.getByRole('button', { name: 'Disable privileged mode' })).toBeVisible();
    await expect(page.getByText(room.body, { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Enable privileged mode' })).toBeEnabled();
    await expect(page.getByText(readDenied, { exact: true })).toBeVisible();
    await expect(page.getByText(room.body, { exact: true })).toHaveCount(0);
    await expectRestrictedAccessDenied(page, room.id);
    if (serverDriven) expect(serverRequestedExpiry).toBe(true);
    await activate(page);
    await expect(page.getByText(room.body, { exact: true })).toBeVisible();
    await roomPage.sendMessage('Posting restored after actual session expiry');
  });
}

test('a user without elevation entitlement has no shield and cannot activate through the API', async ({
  page
}) => {
  await page.goto(routes.root);
  await createAndLoginTestUser(page);
  await page.goto(routes.serverOverview);
  await expect(page.getByTestId('server-subscription-active')).toHaveAttribute(
    'data-projection-ready',
    'true'
  );
  await expect(page.getByTestId('privileged-mode-toggle')).toHaveCount(0);
  const response = await connectPostResponse(page, `${viewerService}/ActivatePrivilegedMode`);
  expect(response.status()).toBe(400);
  expect(await response.json()).toMatchObject({ code: 'failed_precondition' });
  const viewer = await connectPost<{ privilegedMode?: { available?: boolean; active?: boolean } }>(
    page,
    `${viewerService}/GetViewer`
  );
  expect(viewer.privilegedMode?.available ?? false).toBe(false);
  expect(viewer.privilegedMode?.active ?? false).toBe(false);
});

test('deactivation reaches another tab but preserves an independent session of the same owner', async ({
  page,
  context,
  browser,
  serverURL,
  browserErrors
}) => {
  // The production credential check interval is one minute. Exercise it without
  // a test-only fast path, and keep the timeout separate from ordinary UI waits.
  test.setTimeout(100_000);
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page);
  const room = await restrictedRoom(page);
  await page.goto(routes.room(room.id));
  await waitForRoomReady(page);
  const sibling = await context.newPage();
  const independentContext = await browser.newContext({ baseURL: serverURL });
  try {
    await sibling.goto(routes.room(room.id));
    await waitForRoomReady(sibling);
    await expect(sibling.getByText(room.body, { exact: true })).toBeVisible();
    const independent = await independentContext.newPage();
    independent.on('pageerror', (error) => browserErrors.push(error.message));
    await independent.goto(routes.root);
    await loginAsAdminAndUsePrimaryServer(independent);
    await independent.goto(routes.room(room.id));
    await waitForRoomReady(independent);
    await deactivate(page);
    await expectRestrictedAccessDenied(sibling, room.id);
    await expect(sibling.getByRole('button', { name: 'Enable privileged mode' })).toBeEnabled({
      timeout: 75_000
    });
    await expect(sibling.getByText(readDenied, { exact: true })).toBeVisible();
    await expect(sibling.getByText(room.body, { exact: true })).toHaveCount(0);
    await expect(
      independent.getByRole('button', { name: 'Disable privileged mode' })
    ).toBeEnabled();
    const body = 'Still authorized in the independent owner session';
    await postMessageViaConnect(independent, room.id, body);
    await expect(independent.getByText(body, { exact: true })).toBeVisible();
    // A subsequent event in a readable room proves the sibling recovered live
    // delivery while restricted content stays absent.
    await sibling.getByRole('link', { name: '# general', exact: true }).click();
    await waitForRoomReady(sibling, 'general');
    const generalId = new URL(sibling.url()).pathname.split('/')[3];
    await postMessageViaConnect(independent, generalId, 'Sibling recovery remains live');
    await expect(sibling.getByText('Sibling recovery remains live', { exact: true })).toBeVisible();
  } finally {
    await sibling.close();
    await independentContext.close();
  }
});
