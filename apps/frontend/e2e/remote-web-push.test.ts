import { createHash } from 'node:crypto';
import type { CDPSession, Page } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  connectRemoteInstance,
  createUserOnRemote,
  startSecondServer,
  stopSecondServer
} from './fixtures/multiServer';
import type { ServerInfo } from './fixtures/server';
import { ChatPage } from './pages';
import { expect, test } from './setup';

type ServiceWorkerRegistrationUpdated = {
  registrations: Array<{
    registrationId: string;
    scopeURL: string;
    isDeleted: boolean;
  }>;
};

type NotificationSnapshot = {
  title: string;
  body: string;
  tag: string;
  data: unknown;
};

type NotificationState = {
  permission: NotificationPermission;
  registrationScopes: string[];
  notifications: NotificationSnapshot[];
};

test.describe('remote Web Push delivery', () => {
  test.describe.configure({ timeout: 60_000 });

  let remoteServer: ServerInfo | undefined;

  test.beforeEach(async ({}, testInfo) => {
    remoteServer = await startSecondServer(testInfo);
  });

  test.afterEach(async ({}, testInfo) => {
    if (remoteServer) {
      await stopSecondServer(remoteServer, testInfo);
      remoteServer = undefined;
    }
  });

  test('a scoped worker receives a remote notification while the app is closed', async ({
    browserName,
    playwright,
    serverURL
  }) => {
    test.skip(browserName !== 'chromium', 'Push injection requires the Chromium DevTools protocol');

    // Playwright's default Chromium headless shell hard-denies notification
    // permission. Full Chromium's new headless mode supports the production
    // permission and service-worker notification paths used by this test.
    const pushBrowser = await playwright.chromium.launch({ channel: 'chromium' });
    try {
      const context = await pushBrowser.newContext({
        baseURL: serverURL,
        permissions: ['notifications']
      });
      const page = await context.newPage();
      const chatPage = new ChatPage(page);

      await createAndLoginTestUser(page);
      await chatPage.goto();

      const remoteBaseURL = remoteServer!.baseURL.replace('localhost', '127.0.0.1');
      const remoteOrigin = new URL(remoteBaseURL).origin;
      const remoteUser = await createUserOnRemote(
        remoteBaseURL,
        'remote-push-recipient',
        'password123'
      );
      await connectRemoteInstance(
        page,
        { ...remoteServer!, baseURL: remoteBaseURL },
        remoteUser.userId
      );
      await chatPage.enterRoom('general');
      const browserErrors = collectBrowserErrors(page);

      const clientOrigin = new URL(page.url()).origin;
      const notificationURL = page.url();
      expect(notificationURL).toContain(`/chat/${new URL(remoteBaseURL).hostname}/`);

      expect(await page.evaluate(() => Notification.permission)).toBe('granted');

      const scopePath = `/__chatto/push/${createHash('sha256').update(remoteOrigin).digest('hex')}/`;
      const scopeURL = new URL(scopePath, clientOrigin).toString();
      // CDP's ServiceWorker domain must be attached to a page target. Keep an
      // inert same-context target alive after closing the actual Chatto page.
      const protocolPage = await context.newPage();
      const devtools = await context.newCDPSession(protocolPage);
      const workerErrors: string[] = [];
      devtools.on('ServiceWorker.workerErrorReported', ({ errorMessage }) => {
        workerErrors.push(errorMessage.errorMessage);
      });
      await devtools.send('ServiceWorker.enable');

      const registrationIdPromise = waitForRegistrationId(devtools, scopeURL);
      const registeredScope = await registerScopedWorker(page, scopePath);
      expect(registeredScope).toBe(scopeURL);
      const registrationId = await registrationIdPromise;

      const payload = {
        web_push: 8030,
        mutable: true,
        title: 'Remote mention',
        body: 'A message from server B',
        tag: 'remote-push-e2e',
        notificationId: 'remote-notification-e2e',
        url: notificationURL,
        notification: {
          title: 'Remote mention',
          body: 'A message from server B',
          tag: 'remote-push-e2e',
          navigate: notificationURL,
          data: {
            notificationId: 'remote-notification-e2e',
            url: notificationURL
          }
        }
      };

      await page.close();
      expect(context.pages().map((candidate) => candidate.url())).toEqual(['about:blank']);

      await devtools.send('ServiceWorker.deliverPushMessage', {
        origin: clientOrigin,
        registrationId,
        data: JSON.stringify(payload)
      });

      const inspectionPage = protocolPage;
      const inspectionBrowserErrors = collectBrowserErrors(inspectionPage);
      await inspectionPage.goto('/');

      await expect
        .poll(() => notificationState(inspectionPage, scopeURL))
        .toEqual({
          permission: 'granted',
          registrationScopes: expect.arrayContaining([scopeURL]),
          notifications: [
            {
              title: payload.title,
              body: payload.body,
              tag: payload.tag,
              data: {
                notificationId: payload.notificationId,
                url: notificationURL
              }
            }
          ]
        });

      expect(browserErrors).toEqual([]);
      expect(inspectionBrowserErrors).toEqual([]);
      expect(workerErrors).toEqual([]);
      await devtools.detach();
    } finally {
      await pushBrowser.close();
    }
  });
});

function collectBrowserErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => {
    if (message.type() !== 'error') return;
    const location = message.location();
    errors.push(location.url ? `${message.text()} (${location.url})` : message.text());
  });
  return errors;
}

function waitForRegistrationId(devtools: CDPSession, expectedScopeURL: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      cleanup();
      reject(new Error(`Chromium did not report service worker registration ${expectedScopeURL}`));
    }, 15_000);

    const onRegistrationUpdated = (event: ServiceWorkerRegistrationUpdated) => {
      const registration = event.registrations.find(
        (candidate) => candidate.scopeURL === expectedScopeURL && !candidate.isDeleted
      );
      if (!registration) return;

      cleanup();
      resolve(registration.registrationId);
    };

    const cleanup = () => {
      clearTimeout(timeout);
      devtools.off('ServiceWorker.workerRegistrationUpdated', onRegistrationUpdated);
    };

    devtools.on('ServiceWorker.workerRegistrationUpdated', onRegistrationUpdated);
  });
}

async function registerScopedWorker(page: Page, scopePath: string): Promise<string> {
  return page.evaluate(async (scope) => {
    const registration = await navigator.serviceWorker.register('/service-worker.js', {
      scope,
      type: 'module'
    });
    if (registration.active) return registration.scope;

    const worker = registration.installing ?? registration.waiting;
    if (!worker) throw new Error('Scoped service worker did not install');

    await new Promise<void>((resolve, reject) => {
      const timeout = window.setTimeout(() => {
        cleanup();
        reject(new Error(`Scoped service worker did not activate; final state: ${worker.state}`));
      }, 15_000);
      const onStateChange = () => {
        if (worker.state === 'activated') {
          cleanup();
          resolve();
        } else if (worker.state === 'redundant') {
          cleanup();
          reject(new Error('Scoped service worker became redundant'));
        }
      };
      const cleanup = () => {
        window.clearTimeout(timeout);
        worker.removeEventListener('statechange', onStateChange);
      };

      worker.addEventListener('statechange', onStateChange);
      onStateChange();
    });

    return registration.scope;
  }, scopePath);
}

async function notificationState(page: Page, scopeURL: string): Promise<NotificationState> {
  return page.evaluate(async (expectedScopeURL) => {
    const registrations = await navigator.serviceWorker.getRegistrations();
    const registration = registrations.find((candidate) => candidate.scope === expectedScopeURL);
    const notifications = registration ? await registration.getNotifications() : [];

    return {
      permission: Notification.permission,
      registrationScopes: registrations.map((candidate) => candidate.scope),
      notifications: notifications.map((notification) => ({
        title: notification.title,
        body: notification.body,
        tag: notification.tag,
        data: notification.data
      }))
    };
  }, scopeURL);
}
