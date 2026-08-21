import { expect, type Page } from '@playwright/test';
import { connectPost } from './fixtures/connectHelpers';
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

const BOT_KEY_PATTERN = /^cht_BK_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;
const BOT_KEY_IN_TEXT_PATTERN = /cht_BK_[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g;

// This test handles show-once bearer credentials in the DOM. Keep them out of
// Playwright artifacts even when the test fails or retries. Playwright 1.62's
// error-context snapshot is separate from the configured trace/screenshot
// controls, so disable it explicitly for this test worker as well.
process.env.PLAYWRIGHT_NO_COPY_PROMPT = '1';
test.use({ trace: 'off', screenshot: 'off', video: 'off' });

function redactBotKeys(value: string): string {
  return value.replace(BOT_KEY_IN_TEXT_PATTERN, '[REDACTED]');
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

async function getRoomAsBot(
  serverURL: string,
  apiKey: string,
  roomId: string
): Promise<BotAPIResult> {
  // Use Node's fetch rather than page.request so the admin's browser cookies
  // cannot supply ambient authority and Playwright never records the key.
  const response = await fetch(
    new URL('/api/connect/chatto.api.v1.RoomDirectoryService/GetRoom', serverURL),
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${apiKey}`,
        'Connect-Protocol-Version': '1',
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ roomId })
    }
  );

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

test.describe('Bot account lifecycle', () => {
  // setup.ts gives every test its own server and removes that server's data
  // directory during fixture teardown, including after an early failure.
  test('create, authorise, rotate, and delete through Server Admin', async ({
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

    await page.goto(routes.serverAdminBots);
    await expect(page.getByRole('heading', { name: 'Bots', exact: true })).toBeVisible();

    const suffix = Date.now().toString(36);
    const botLogin = `lifecycle_${suffix}_bot`;
    const botDisplayName = `Lifecycle Bot ${suffix}`;

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
