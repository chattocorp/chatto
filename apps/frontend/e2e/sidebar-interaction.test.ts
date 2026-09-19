import type { Locator } from '@playwright/test';
import { test, expect } from './setup';
import { withBootstrapAdminRequest } from './fixtures/adminRequest';
import {
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect,
  joinRoomViaConnect
} from './fixtures/connectHelpers';
import { createAndLoginTestUser, loginAsAdmin } from './fixtures/testUser';
import * as routes from './routes';

test.use({ screenshot: 'only-on-failure', trace: 'retain-on-failure' });

for (const touch of [false, true]) {
  test.describe(touch ? 'Sidebar touch input' : 'Sidebar mouse input', () => {
    test.use({
      hasTouch: touch,
      isMobile: touch,
      viewport: touch ? { width: 375, height: 812 } : { width: 1280, height: 800 }
    });

    for (const admin of [false, true]) {
      test(`${admin ? 'administrator' : 'member'} can select rooms on the first attempt`, async ({
        page,
        serverURL,
        chatPage
      }, testInfo) => {
        const errors: string[] = [];
        const touchWarnings = new Set<string>();
        page.on('pageerror', (error) => errors.push(error.stack ?? error.message));
        page.on('console', (message) => {
          if (message.type() === 'error') errors.push(message.text());
          if (/touchstart|non-passive/i.test(message.text())) touchWarnings.add(message.text());
        });
        // Chrome reports scroll-blocking listener violations through Log, not always console.
        const diagnostics = await page.context().newCDPSession(page);
        await diagnostics.send('Log.enable');
        await diagnostics.send('Log.startViolationsReport', {
          config: [{ name: 'discouragedAPIUse', threshold: -1 }]
        });
        diagnostics.on('Log.entryAdded', ({ entry }) => {
          if (/touchstart|non-passive/i.test(entry.text)) touchWarnings.add(entry.text);
        });

        try {
          if (admin) await loginAsAdmin(page);
          else await createAndLoginTestUser(page);

          const rooms = await withBootstrapAdminRequest(serverURL, async (request) => {
            const groupId = await getDefaultRoomGroupIdViaConnect(request);
            const result = [];
            for (const name of ['click-alpha', 'click-bravo']) {
              result.push({ name, id: await createRoomViaConnect(request, name, groupId) });
            }
            return result;
          });
          for (const room of rooms) await joinRoomViaConnect(page, room.id);

          const activate = (target: Locator) => (touch ? target.tap() : target.click());
          const expectRoom = async (room: (typeof rooms)[number]) => {
            await expect(page).toHaveURL(new URL(routes.room(room.id), serverURL).href);
            await expect(chatPage.getRoomHeader(room.name)).toBeVisible();
            await expect(page.getByTestId('message-input')).toBeVisible();
            await expect(page.getByTestId('message-input')).toHaveAttribute(
              'contenteditable',
              'true'
            );
          };

          await page.goto(routes.room(rooms[0].id));
          await expectRoom(rooms[0]);

          for (let selection = 0; selection < 5; selection += 1) {
            const current = rooms[selection % 2];
            const destination = rooms[(selection + 1) % 2];
            await test.step(`Select ${destination.name}, attempt ${selection + 1}`, async () => {
              if (touch) {
                await expect(chatPage.roomList).not.toBeVisible();
                await activate(page.locator('button[title="Toggle sidebar"]'));
              }
              const link = chatPage.getRoomLink(destination.name);
              await expect(link).toBeVisible();
              const handle = link.getByTestId('room-drag-handle');
              if (admin) {
                if (!touch) await link.hover();
                await expect(handle).toHaveCSS('opacity', '1');
                // A click/tap presses and releases without a drag. It must not navigate.
                await activate(handle);
                await expectRoom(current);
                await expect(page.locator('#dnd-action-dragged-el')).toHaveCount(0);
              } else {
                await expect(handle).toHaveCount(0);
              }

              // Target the name explicitly: the leading icon can be a drag handle.
              // Never retry this action; only the resulting state assertions may poll.
              await activate(link.getByText(destination.name, { exact: true }));
              await expectRoom(destination);
              await expect(link).toHaveAttribute('aria-current', 'page');
              await expect(page.locator('#dnd-action-dragged-el')).toHaveCount(0);
              if (touch) await expect(chatPage.roomList).not.toBeVisible();
              expect(errors, 'Browser errors during sidebar navigation').toEqual([]);
            });
          }
        } finally {
          await testInfo.attach('sidebar-browser-diagnostics', {
            body: JSON.stringify({ errors, touchWarnings: [...touchWarnings] }, null, 2),
            contentType: 'application/json'
          });
          await diagnostics.detach();
          expect.soft(errors, 'Browser errors during sidebar navigation').toEqual([]);
        }
      });
    }
  });
}
