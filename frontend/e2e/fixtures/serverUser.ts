import { expect, type Browser, type BrowserContextOptions, type Page } from '@playwright/test';
import { ChatPage, RoomPage } from '../pages';
import { TIMEOUTS } from '../constants';
import { withBootstrapAdminRequest } from './adminRequest';
import {
  createAndLoginTestUser,
  loginTestUser,
  openServer,
  type CreateTestUserOptions,
  type TestUser
} from './testUser';

export interface ServerUserSession {
  page: Page;
  user: TestUser;
  chatPage: ChatPage;
  roomPage: RoomPage;
}

export interface ServerUserOptions extends BrowserContextOptions {
  userOptions?: CreateTestUserOptions;
}

export async function loginAndEnterRoom(
  page: Page,
  roomName = 'general',
  userOptions?: CreateTestUserOptions
): Promise<ServerUserSession> {
  const user = await createAndLoginTestUser(page, userOptions);
  const chatPage = new ChatPage(page);
  const roomPage = new RoomPage(page);

  await chatPage.goto();
  await chatPage.enterRoom(roomName);

  return { page, user, chatPage, roomPage };
}

export async function withServerUser<T>(
  browser: Browser,
  serverURL: string,
  run: (session: ServerUserSession) => Promise<T>,
  options: ServerUserOptions = {}
): Promise<T> {
  const { userOptions, ...contextOptions } = options;
  const context = await browser.newContext({ ...contextOptions, baseURL: serverURL });
  const page = await context.newPage();

  try {
    const user = await createAndLoginTestUser(page, userOptions);
    await openServer(page);

    return await run({
      page,
      user,
      chatPage: new ChatPage(page),
      roomPage: new RoomPage(page)
    });
  } finally {
    await context.close();
  }
}

export async function withLoggedInServerWindow<T>(
  browser: Browser,
  serverURL: string,
  user: TestUser,
  run: (session: ServerUserSession) => Promise<T>,
  contextOptions: BrowserContextOptions = {}
): Promise<T> {
  const context = await browser.newContext({ ...contextOptions, baseURL: serverURL });
  const page = await context.newPage();

  try {
    await loginTestUser(page, user);
    await openServer(page);

    return await run({
      page,
      user,
      chatPage: new ChatPage(page),
      roomPage: new RoomPage(page)
    });
  } finally {
    await context.close();
  }
}

export { withBootstrapAdminRequest } from './adminRequest';

export async function postMentionFromServerUser(
  browser: Browser,
  serverURL: string,
  mentionedLogin: string,
  message: string,
  roomName = 'general'
): Promise<void> {
  await withServerUser(browser, serverURL, async ({ chatPage, roomPage }) => {
    await chatPage.enterRoom(roomName);
    await roomPage.sendMessage(`@${mentionedLogin} ${message}`);
  });
}

export async function postThreadReplyFromServerUser(
  browser: Browser,
  serverURL: string,
  rootMessage: string,
  replyText: string
): Promise<void> {
  await withServerUser(browser, serverURL, async ({ chatPage, roomPage }) => {
    await chatPage.enterRoom('general');
    const message = roomPage.getMessage(rootMessage);
    await message.openThread();
    await roomPage.expectThreadPaneVisible();
    await roomPage.postThreadReply(replyText);
  });
}

export async function postRoomReplyFromServerUser(
  browser: Browser,
  serverURL: string,
  rootMessage: string,
  replyText: string
): Promise<void> {
  await withServerUser(browser, serverURL, async ({ page, chatPage, roomPage }) => {
    await chatPage.enterRoom('general');
    const targetMsg = roomPage.getMessage(rootMessage);
    await targetMsg.replyInRoom();
    await expect(page.getByTestId('reply-indicator')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await roomPage.sendMessage(replyText);
  });
}

export function serverNotificationBadge(page: Page) {
  return page
    .locator('.server-gutter [data-testid="server-icon"]')
    .first()
    .locator('..')
    .getByTestId('server-notification-badge');
}

export async function joinRoomFromOverview(page: Page, roomName: string): Promise<void> {
  await page.getByRole('link', { name: 'Overview' }).click();
  const roomItem = page.locator('li', { hasText: `# ${roomName}` });
  await roomItem.getByRole('button', { name: 'Join' }).click();
  // The Joined button swaps visible text to "Leave" on hover, and Playwright
  // leaves the cursor on the button it just clicked. Asserting on `title` is
  // stable across hover state.
  await expect(roomItem.locator('button[title^="Joined "]')).toBeVisible({
    timeout: TIMEOUTS.UI_STANDARD
  });
}
