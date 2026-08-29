import { expect, type Page } from '@playwright/test';
import {
  connectPost,
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect
} from './fixtures/connectHelpers';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import * as routes from './routes';
import { test } from './setup';

interface BotAPIResult {
  status: number;
  code?: string;
}

interface ListRoomsResponse {
  rooms?: Array<{ room?: { id?: string } }>;
}

interface ListBotsResponse {
  bots?: Array<{ user?: { id?: string; login?: string } }>;
}

interface ViewerResponse {
  user?: { profile?: { id?: string } };
}

interface StartDMResponse {
  room?: { id?: string };
}

interface CreatedUserResponse {
  id?: string;
}

const BOT_KEY_PATTERN = /^cht_BK_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;
const BOT_KEY_IN_TEXT_PATTERN = /cht_BK_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g;
const WEBHOOK_CREDENTIAL_PATTERN =
  /^cht_IW_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;
const WEBHOOK_CREDENTIAL_IN_TEXT_PATTERN =
  /cht_IW_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g;

// This test handles show-once bearer credentials in the DOM. Keep them out of
// Playwright artifacts even when the test fails or retries. Playwright 1.62's
// error-context snapshot is separate from the configured trace/screenshot
// controls, so disable it explicitly for this test worker as well.
process.env.PLAYWRIGHT_NO_COPY_PROMPT = '1';
test.use({ trace: 'off', screenshot: 'off', video: 'off' });

function redactBotKeys(value: string): string {
  return value
    .replace(BOT_KEY_IN_TEXT_PATTERN, '[REDACTED]')
    .replace(WEBHOOK_CREDENTIAL_IN_TEXT_PATTERN, '[REDACTED]');
}

async function captureShowOnceBotKey(page: Page): Promise<string> {
  const dialog = page.getByRole('dialog', { name: 'Save This API Key' });
  const keyElement = dialog.locator('code');
  await keyElement.waitFor({ state: 'visible' });
  const apiKey = await keyElement.evaluate((element) => {
    const value = element.textContent?.trim() ?? '';
    // Playwright 1.62 writes an accessibility snapshot on failure even when
    // trace and screenshots are off. Redact the live DOM before any later
    // assertion or action can fail and trigger that snapshot.
    element.textContent = '[REDACTED]';
    return value;
  });

  await dialog.getByRole('button', { name: 'Got it', exact: true }).click();
  await expect(dialog).toBeHidden();

  if (!apiKey || !BOT_KEY_PATTERN.test(apiKey)) {
    throw new Error('The show-once bot API key had an unexpected format');
  }
  return apiKey;
}

async function captureShowOnceWebhookURL(page: Page): Promise<string> {
  const dialog = page.getByRole('dialog', { name: 'Save This Webhook URL' });
  const urlElement = dialog.locator('code');
  await urlElement.waitFor({ state: 'visible' });
  const webhookURL = await urlElement.evaluate((element) => {
    const value = element.textContent?.trim() ?? '';
    element.textContent = '[REDACTED]';
    return value;
  });

  await dialog.getByRole('button', { name: 'Got it', exact: true }).click();
  await expect(dialog).toBeHidden();

  let credential = '';
  try {
    credential = new URL(webhookURL).pathname.split('/').at(-1) ?? '';
  } catch {
    throw new Error('The show-once incoming webhook had an invalid URL');
  }
  if (!WEBHOOK_CREDENTIAL_PATTERN.test(credential)) {
    throw new Error('The show-once incoming webhook had an unexpected credential format');
  }
  return webhookURL;
}

async function postIncomingWebhook(webhookURL: string, roomId: string, text: string) {
  const target = new URL(webhookURL);
  target.searchParams.set('room_id', roomId);
  const response = await fetch(target, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text })
  });
  return { status: response.status, body: await response.text() };
}

async function callAsBot(
  serverURL: string,
  apiKey: string,
  procedure: string,
  body: Record<string, unknown>
): Promise<BotAPIResult> {
  // Use Node's fetch rather than page.request so the admin's browser cookies
  // cannot supply ambient authority and Playwright never records the key.
  const response = await fetch(new URL(`/api/connect/${procedure}`, serverURL), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Connect-Protocol-Version': '1',
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(body)
  });

  let code: string | undefined;
  try {
    const payload: unknown = await response.json();
    if (payload && typeof payload === 'object' && 'code' in payload) {
      const candidate = (payload as { code?: unknown }).code;
      if (typeof candidate === 'string') code = candidate;
    }
  } catch {
    // Successful Connect responses and the status are sufficient for this
    // lifecycle assertion even if an error proxy returns a non-JSON body.
  }

  return { status: response.status, ...(code ? { code } : {}) };
}

async function getRoomAsBot(
  serverURL: string,
  apiKey: string,
  roomId: string
): Promise<BotAPIResult> {
  return callAsBot(serverURL, apiKey, 'chatto.api.v1.RoomDirectoryService/GetRoom', { roomId });
}

async function createHumanOwner(
  page: Page,
  suffix: string
): Promise<{ login: string; displayName: string }> {
  const login = `botowner${suffix}`;
  const displayName = `Bot Owner ${suffix}`;
  const response = await page.request.post('/auth/test/create-user', {
    headers: { 'Content-Type': 'application/json' },
    data: { login, displayName, password: 'testpassword123' }
  });
  expect(response.ok()).toBeTruthy();
  const created = (await response.json()) as CreatedUserResponse;
  if (!created.id) throw new Error('The bot owner fixture did not return a user ID');
  return { login, displayName };
}

test.describe('Bot account lifecycle', () => {
  // setup.ts gives every test its own server and removes that server's data
  // directory during fixture teardown, including after an early failure.
  test('create, authorise, manage credentials, and delete through Server Admin', async ({
    page,
    serverURL
  }) => {
    const browserErrors: string[] = [];
    page.on('pageerror', (error) => browserErrors.push(redactBotKeys(error.message)));
    page.on('console', (message) => {
      if (message.type() === 'error') browserErrors.push(redactBotKeys(message.text()));
    });

    await loginAsAdminAndUsePrimaryServer(page);
    const directory = await connectPost<ListRoomsResponse>(
      page,
      'chatto.api.v1.RoomDirectoryService/ListRooms'
    );
    const roomId = directory.rooms?.[0]?.room?.id;
    if (!roomId) throw new Error('The bootstrap server did not expose a room for the bot test');
    const roomGroupId = await getDefaultRoomGroupIdViaConnect(page);
    const webhookRoomId = await createRoomViaConnect(
      page,
      `bot-lifecycle-${Date.now()}`,
      roomGroupId
    );

    await page.goto(routes.serverAdminBots);
    await expect(page.getByRole('heading', { name: 'Bots', exact: true })).toBeVisible();

    const suffix = Date.now().toString(36);
    const botLogin = `lifecycle_${suffix}_bot`;
    const botDisplayName = `Lifecycle Bot ${suffix}`;
    const newOwner = await createHumanOwner(page, suffix);

    await page.getByRole('button', { name: 'Create Bot', exact: true }).click();
    const createDialog = page.getByRole('dialog', { name: 'Create Bot Account' });
    await createDialog.getByRole('textbox', { name: 'Username' }).fill(botLogin);
    await createDialog.getByRole('textbox', { name: 'Display Name' }).fill(botDisplayName);
    await createDialog.getByRole('button', { name: 'Create Bot', exact: true }).click();

    const originalKey = await captureShowOnceBotKey(page);
    await page.waitForURL(routes.patterns.anyAdminBot);
    await expect(
      page.getByRole('heading', { name: botDisplayName, exact: true, level: 1 })
    ).toBeVisible();

    const listedBots = await connectPost<ListBotsResponse>(
      page,
      'chatto.api.v1.BotService/ListBots'
    );
    const botId = listedBots.bots?.find((bot) => bot.user?.login === botLogin)?.user?.id;
    if (!botId) throw new Error('The new bot was not present in the bot directory');
    await connectPost(page, 'chatto.api.v1.RoomService/AddMember', {
      roomId: webhookRoomId,
      userId: botId
    });
    const viewer = await connectPost<ViewerResponse>(page, 'chatto.api.v1.ViewerService/GetViewer');
    const adminId = viewer.user?.profile?.id;
    if (!adminId) throw new Error('The viewer response did not contain the admin user ID');

    await expect(getRoomAsBot(serverURL, originalKey, roomId)).resolves.toEqual({
      status: 403,
      code: 'permission_denied'
    });

    const permissionFilter = page.getByTestId('permission-filter');
    await expect(permissionFilter).toBeVisible();
    await permissionFilter.fill('room.list');
    const disabledRoomList = page.getByRole('button', {
      name: 'room.list is Disabled for bot at Server',
      exact: true
    });
    await expect(disabledRoomList).toBeEnabled();
    await disabledRoomList.click();
    await expect(
      page.getByRole('button', {
        name: 'room.list is Enabled for bot at Server',
        exact: true
      })
    ).toBeVisible();

    await expect(getRoomAsBot(serverURL, originalKey, roomId)).resolves.toEqual({ status: 200 });

    await permissionFilter.fill('message.post');
    const disabledMessagePost = page.getByRole('button', {
      name: 'message.post is Disabled for bot at Server',
      exact: true
    });
    await expect(disabledMessagePost).toBeEnabled();
    await disabledMessagePost.click();
    await expect(
      page.getByRole('button', {
        name: 'message.post is Enabled for bot at Server',
        exact: true
      })
    ).toBeVisible();

    await page.getByRole('button', { name: 'Create Webhook', exact: true }).click();
    const createWebhookDialog = page.getByRole('dialog', { name: 'Create Webhook' });
    await createWebhookDialog.getByRole('textbox', { name: 'Name' }).fill('Production');
    await createWebhookDialog
      .getByRole('button', { name: 'Create Webhook', exact: true })
      .click();
    const originalWebhookURL = await captureShowOnceWebhookURL(page);

    await page.getByRole('button', { name: 'Create Webhook', exact: true }).click();
    await createWebhookDialog.getByRole('textbox', { name: 'Name' }).fill('Backup');
    await createWebhookDialog
      .getByRole('button', { name: 'Create Webhook', exact: true })
      .click();
    const backupWebhookURL = await captureShowOnceWebhookURL(page);
    await expect(
      postIncomingWebhook(originalWebhookURL, webhookRoomId, 'First incoming webhook message')
    ).resolves.toEqual({ status: 200, body: 'ok' });
    await page.reload();
    const webhookList = page.getByTestId('bot-incoming-webhooks');
    const productionWebhook = webhookList.locator('.selectable-list-item').filter({
      hasText: 'Production'
    });
    const backupWebhook = webhookList.locator('.selectable-list-item').filter({ hasText: 'Backup' });
    await expect(productionWebhook).toContainText('Last used');
    await expect(productionWebhook).not.toContainText('No use recorded');
    await expect(backupWebhook).toContainText('No use recorded');

    await expect(
      webhookList.getByRole('button', { name: 'Rotate Webhook', exact: true })
    ).toHaveCount(0);
    await page.getByRole('button', { name: 'Create Webhook', exact: true }).click();
    await createWebhookDialog.getByRole('textbox', { name: 'Name' }).fill('Replacement');
    await createWebhookDialog
      .getByRole('button', { name: 'Create Webhook', exact: true })
      .click();
    const replacementWebhookURL = await captureShowOnceWebhookURL(page);
    await expect(
      postIncomingWebhook(
        replacementWebhookURL,
        webhookRoomId,
        'Replacement incoming webhook message'
      )
    ).resolves.toEqual({ status: 200, body: 'ok' });
    await expect(
      postIncomingWebhook(backupWebhookURL, webhookRoomId, 'Independent backup webhook message')
    ).resolves.toEqual({ status: 200, body: 'ok' });

    await productionWebhook.getByRole('button', { name: 'Revoke Webhook', exact: true }).click();
    const revokeWebhookDialog = page.getByRole('dialog', { name: 'Revoke Webhook' });
    await revokeWebhookDialog.getByRole('button', { name: 'Revoke Webhook', exact: true }).click();
    await expect(revokeWebhookDialog).toBeHidden();
    await expect(
      postIncomingWebhook(originalWebhookURL, webhookRoomId, 'Rejected revoked webhook message')
    ).resolves.toEqual({ status: 401, body: 'invalid_token' });
    await expect(
      postIncomingWebhook(replacementWebhookURL, webhookRoomId, 'Replacement after revocation')
    ).resolves.toEqual({ status: 200, body: 'ok' });
    await expect(
      postIncomingWebhook(backupWebhookURL, webhookRoomId, 'Backup webhook after revocation')
    ).resolves.toEqual({ status: 200, body: 'ok' });

    const startedDM = await connectPost<StartDMResponse>(
      page,
      'chatto.api.v1.RoomService/StartDM',
      { participantIds: [botId] }
    );
    const dmRoomId = startedDM.room?.id;
    if (!dmRoomId) throw new Error('The human-started bot DM did not return a room ID');

    // A direct message.post grant lets the bot interact in an existing DM,
    // but it cannot invoke StartDM even to retrieve that same DM or itself.
    await expect(
      callAsBot(serverURL, originalKey, 'chatto.api.v1.RoomService/StartDM', {
        participantIds: [adminId]
      })
    ).resolves.toEqual({ status: 403, code: 'permission_denied' });
    await expect(
      callAsBot(serverURL, originalKey, 'chatto.api.v1.RoomService/StartDM', {
        participantIds: []
      })
    ).resolves.toEqual({ status: 403, code: 'permission_denied' });
    await expect(
      callAsBot(serverURL, originalKey, 'chatto.api.v1.MessageService/CreateMessage', {
        roomId: dmRoomId,
        body: 'Bot reply in a human-started DM'
      })
    ).resolves.toEqual({ status: 200 });

    await page.getByRole('button', { name: 'Reassign owner', exact: true }).click();
    const reassignDialog = page.getByRole('dialog', { name: 'Reassign owner' });
    await reassignDialog.getByRole('combobox', { name: 'Owner' }).fill(newOwner.login);
    await page.getByRole('option').filter({ hasText: newOwner.login }).click();
    await reassignDialog.getByRole('button', { name: 'Reassign owner', exact: true }).click();
    await expect(page.getByText('Bot owner reassigned', { exact: true })).toBeVisible();
    await expect(page.getByText(newOwner.displayName, { exact: true })).toBeVisible();

    // Reassignment changes administrative responsibility without rotating or
    // interrupting the integration credential.
    await expect(getRoomAsBot(serverURL, originalKey, roomId)).resolves.toEqual({ status: 200 });

    await page.getByRole('button', { name: 'Rotate Key', exact: true }).click();
    const rotateDialog = page.getByRole('dialog', { name: 'Rotate API Key' });
    await rotateDialog.getByRole('button', { name: 'Rotate Key', exact: true }).click();
    const rotatedKey = await captureShowOnceBotKey(page);
    if (rotatedKey === originalKey) {
      throw new Error('Rotating the bot API key did not issue a new credential');
    }

    await expect(getRoomAsBot(serverURL, originalKey, roomId)).resolves.toEqual({
      status: 401,
      code: 'unauthenticated'
    });
    await expect(getRoomAsBot(serverURL, rotatedKey, roomId)).resolves.toEqual({ status: 200 });

    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    const deleteDialog = page.getByRole('dialog', { name: 'Delete Bot' });
    await deleteDialog.getByRole('button', { name: 'Delete', exact: true }).click();
    await page.waitForURL(routes.serverAdminBots);
    await expect(page.getByText('Bot deleted', { exact: true })).toBeVisible();

    await expect(getRoomAsBot(serverURL, rotatedKey, roomId)).resolves.toEqual({
      status: 401,
      code: 'unauthenticated'
    });
    expect(browserErrors, 'browser console and page errors').toEqual([]);
  });
});
