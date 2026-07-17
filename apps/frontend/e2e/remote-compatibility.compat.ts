import { test, expect } from './setup';
import { TIMEOUTS } from './constants';
import { getRoomOnRemote, postMessageOnRemote } from './fixtures/multiServer';
import { createProductionUser, startProductionServer } from './fixtures/productionServer';
import { stopServer, type ServerInfo } from './fixtures/server';

const currentBinary = process.env.CHATTO_COMPAT_CURRENT_BINARY;
const remoteBinary = process.env.CHATTO_COMPAT_REMOTE_BINARY;
const unsupportedBinary = process.env.CHATTO_COMPAT_UNSUPPORTED_BINARY;

const originUser = {
  login: 'compat-origin',
  displayName: 'Compatibility Origin',
  password: 'compat-origin-password123'
};
const remoteUser = {
  login: 'compat-remote',
  displayName: 'Compatibility Remote',
  password: 'compat-remote-password123'
};

test.use({
  serverOptions: {
    executablePath: currentBinary,
    instanceId: 'compat-current',
    operatorApi: true
  }
});

test.describe('supported release baseline', () => {
  test.skip(
    !currentBinary || !remoteBinary,
    'Set current and compatible remote production executables'
  );

  test('current frontend can connect to and use the latest compatible stable server', async ({
    page,
    authPage,
    chatPage,
    roomPage,
    server
  }, testInfo) => {
    let remoteServer: ServerInfo | undefined;

    try {
      await createProductionUser(server, originUser);
      remoteServer = await startProductionServer(
        testInfo,
        remoteBinary!,
        'compat-remote-stable',
        5,
        '127.0.0.1'
      );
      await createProductionUser(remoteServer, remoteUser);

      // Sign in to the current production build and use its embedded frontend.
      await authPage.gotoLogin();
      await authPage.fillLoginForm(originUser.login, originUser.password);
      await authPage.signInButton.click();
      await page.waitForURL(
        (url) => url.hostname === 'localhost' && url.pathname.startsWith('/chat')
      );

      // Discover the released server and complete its real OAuth + PKCE flow.
      const remoteHost = new URL(remoteServer.baseURL).host;
      await page.getByTitle('Add Server').click();
      await page.getByLabel('Server URL').fill(remoteHost);
      await page.getByRole('button', { name: 'Connect' }).click();
      await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      const popupPromise = page.waitForEvent('popup');
      await page.getByRole('button', { name: 'Sign in', exact: true }).click();
      const remoteLoginPage = await popupPromise;

      await expect(remoteLoginPage).toHaveURL(/127\.0\.0\.1.*\/login\?redirect=/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await remoteLoginPage.locator('input[autocomplete="username"]').fill(remoteUser.login);
      await remoteLoginPage
        .locator('input[autocomplete="current-password"]')
        .fill(remoteUser.password);
      await remoteLoginPage.getByRole('button', { name: /Sign In/i }).click();
      await expect(remoteLoginPage).toHaveURL(/127\.0\.0\.1.*\/oauth\/consent/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      const popupClosed = remoteLoginPage.waitForEvent('close');
      await remoteLoginPage.getByRole('button', { name: 'Allow Access' }).click();
      await popupClosed;

      await expect(page).toHaveURL(/localhost.*\/chat\/127\.0\.0\.1(\/|$)/, {
        timeout: TIMEOUTS.COMPLEX_OPERATION
      });
      await expect(
        page.locator(`[data-testid="server-icon"][href*="127.0.0.1"]`).first()
      ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Exercise the current room directory and join flow against the released server.
      const generalRoom = page.getByRole('listitem').filter({ hasText: '# general' });
      await generalRoom.getByRole('button', { name: 'Join' }).click();
      await expect(chatPage.roomList.getByRole('link', { name: '# general' })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Exercise authenticated timeline/message APIs through the current UI.
      await chatPage.enterRoom('general');
      const sentMessage = `Compatibility UI message ${Date.now()}`;
      await roomPage.sendMessage(sentMessage);

      const remoteRegistration = await page.evaluate(() => {
        const registrations = JSON.parse(
          localStorage.getItem('chatto:instances') || '[]'
        ) as Array<{
          url?: string;
          token?: string;
        }>;
        return registrations.find((registration) => registration.url?.includes('127.0.0.1'));
      });
      expect(remoteRegistration?.token).toBeTruthy();

      const roomId = await getRoomOnRemote(
        remoteServer.baseURL,
        remoteRegistration!.token!,
        'general'
      );
      const receivedMessage = `Compatibility realtime message ${Date.now()}`;
      await postMessageOnRemote(
        remoteServer.baseURL,
        remoteRegistration!.token!,
        roomId,
        receivedMessage
      );
      await roomPage.expectMessageVisible(receivedMessage);

      // A cold load must retain the remote registration and read persisted history.
      await page.reload();
      await expect(page).toHaveURL(/\/chat\/127\.0\.0\.1\//);
      await roomPage.expectMessageVisible(sentMessage);
      await roomPage.expectMessageVisible(receivedMessage);
      await expect(page.getByTitle('Sign out')).toBeVisible();
    } finally {
      if (remoteServer) {
        await stopServer(remoteServer);
      }
    }
  });
});

test.describe('unsupported release boundary', () => {
  test.skip(
    !currentBinary || !unsupportedBinary,
    'Set current and unsupported remote production executables'
  );

  test('current frontend rejects a 0.4 server without opening realtime', async ({
    page,
    authPage,
    server
  }, testInfo) => {
    let unsupportedServer: ServerInfo | undefined;

    try {
      await createProductionUser(server, originUser);
      unsupportedServer = await startProductionServer(
        testInfo,
        unsupportedBinary!,
        'compat-remote-unsupported',
        6,
        '127.0.0.1'
      );
      await createProductionUser(unsupportedServer, remoteUser);

      await authPage.gotoLogin();
      await authPage.fillLoginForm(originUser.login, originUser.password);
      await authPage.signInButton.click();
      await page.waitForURL(
        (url) => url.hostname === 'localhost' && url.pathname.startsWith('/chat')
      );

      const remoteHost = new URL(unsupportedServer.baseURL).host;
      let remoteRealtimeConnections = 0;
      page.on('websocket', (socket) => {
        const url = new URL(socket.url());
        if (url.host === remoteHost && url.pathname === '/api/realtime') {
          remoteRealtimeConnections++;
        }
      });

      await page.getByTitle('Add Server').click();
      await page.getByLabel('Server URL').fill(remoteHost);
      await page.getByRole('button', { name: 'Connect' }).click();
      await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      const popupPromise = page.waitForEvent('popup');
      await page.getByRole('button', { name: 'Sign in', exact: true }).click();
      const remoteLoginPage = await popupPromise;

      await expect(remoteLoginPage).toHaveURL(/127\.0\.0\.1.*\/login\?redirect=/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await remoteLoginPage.locator('input[autocomplete="username"]').fill(remoteUser.login);
      await remoteLoginPage
        .locator('input[autocomplete="current-password"]')
        .fill(remoteUser.password);
      await remoteLoginPage.getByRole('button', { name: /Sign In/i }).click();
      await expect(remoteLoginPage).toHaveURL(/127\.0\.0\.1.*\/oauth\/consent/, {
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      const popupClosed = remoteLoginPage.waitForEvent('close');
      await remoteLoginPage.getByRole('button', { name: 'Allow Access' }).click();
      await popupClosed;

      const remoteIcon = page.locator(`[data-testid="server-icon"][href*="127.0.0.1"]`).first();
      await expect(remoteIcon).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
      await expect(
        remoteIcon.locator('xpath=..').getByTestId('server-compatibility-warning')
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(remoteIcon).toHaveAttribute(
        'title',
        /This server must be upgraded to Chatto 0\.5 or newer before this app can connect\./
      );
      expect(remoteRealtimeConnections).toBe(0);
    } finally {
      if (unsupportedServer) {
        await stopServer(unsupportedServer);
      }
    }
  });
});
