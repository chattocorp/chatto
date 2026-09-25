// SPDX-License-Identifier: Apache-2.0

import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import type { Page } from '@playwright/test';

/** Record geometry as well as text: an unchanged final DOM can still have shifted. */
async function layout(page: Page, observe = false) {
  return page.evaluate((observe) => {
    const selectors = [
      '[data-testid="server-sidebar"]',
      '[data-testid="room-group-section"]',
      '[data-testid="room-member-list"]',
      '[data-testid="room-sidebar-desktop-pane"]',
      '[data-testid="message-input"]',
      '[data-event-id]'
    ];
    const measure = () =>
      selectors.flatMap((selector) =>
        [...document.querySelectorAll(selector)].map((node) => {
          const rect = node.getBoundingClientRect();
          return {
            selector,
            text: node.textContent,
            x: rect.x,
            y: rect.y,
            width: rect.width,
            height: rect.height
          };
        })
      );
    const initial = measure();
    if (observe) {
      const probe = { changes: [] as ReturnType<typeof measure>[], frame: 0 };
      const baseline = JSON.stringify(initial);
      const sample = () => {
        const current = measure();
        if (JSON.stringify(current) !== baseline && probe.changes.length < 10)
          probe.changes.push(current);
        probe.frame = requestAnimationFrame(sample);
      };
      Object.assign(window, { cachedLayoutProbe: probe });
      probe.frame = requestAnimationFrame(sample);
    }
    return initial;
  }, observe);
}

test('unchanged cached room keeps message metadata and sidebar geometry through reconnect', async ({
  page,
  chatPage,
  roomPage
}, testInfo) => {
  test.setTimeout(90_000);
  await page.setViewportSize({ width: 1440, height: 1000 });
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  await waitForRoomReady(page);
  // Keep server data unchanged: automatic presence otherwise moves the viewer
  // offline on disconnect and online again after reconnect.
  await page.getByTestId('current-user-presence-menu').click();
  await page.getByRole('menuitemradio', { name: 'Look offline', exact: true }).click();
  await expect(roomPage.offlineSectionHeader).toHaveText('Offline (2)');
  await roomPage.offlineSectionHeader.click();
  await expect(page.getByTestId('room-member-card')).toHaveCount(2);
  const root = await roomPage.sendMessage('Cached layout with thread and reaction');
  const eventId = await root.getEventId();
  await root.openThread();
  await roomPage.postThreadReply('A saved thread reply');
  await roomPage.closeThread();
  await root.react('👍');
  await root.expectReaction('👍', 1);
  await root.expectFollowingThread();
  await expect(page.getByTestId('room-member-list')).toBeVisible();

  // Wait for the real persistence owner, without injecting a special cache fixture.
  await expect
    .poll(
      () =>
        page.evaluate(async (id) => {
          return new Promise<boolean>((resolve) => {
            const request = indexedDB.open('chatto-saved-views', 1);
            request.onsuccess = () => {
              const db = request.result;
              const read = db.transaction('views').objectStore('views').getAll();
              read.onsuccess = () => {
                resolve(
                  read.result.some(
                    (view) =>
                      view.version === 2 &&
                      view.rooms.some(
                        (room: {
                          members?: unknown;
                          events: {
                            id: string;
                            event: { replyCount?: number; reactions?: unknown[] };
                          }[];
                        }) =>
                          room.members &&
                          room.events.some(
                            (event) =>
                              event.id === id &&
                              event.event.replyCount === 1 &&
                              event.event.reactions?.length === 1
                          )
                      )
                  )
                );
                db.close();
              };
            };
          });
        }, eventId),
      { timeout: 20_000 }
    )
    .toBe(true);

  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/chatto.api.v1.ViewerService/GetViewer', async (route) => {
    await gate;
    await route.continue();
  });
  try {
    await page.reload();
    await root.expectReaction('👍', 1);
    await root.expectFollowingThread();
    await expect(page.getByTestId('room-member-list')).toBeVisible();
    await expect(page.getByTestId('room-member-card')).toHaveCount(2);
    await expect(page.getByTestId('room-post-denied')).toHaveCount(0);
    await page.evaluate(() => document.fonts.ready);
    // Finish the normal route entrance animation before measuring the data handoff.
    await page.evaluate(async () => {
      await Promise.all(
        document
          .getAnimations()
          .filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity)
          .map((animation) => animation.finished.catch(() => {}))
      );
    });
    const cached = await layout(page, true);
    await page.screenshot({ path: testInfo.outputPath('cached-room.png') });
    release();
    await waitForRoomReady(page);
    await expect.poll(() => layout(page)).toEqual(cached);
    const changes = await page.evaluate(() => {
      const probe = (
        window as unknown as { cachedLayoutProbe: { frame: number; changes: unknown[] } }
      ).cachedLayoutProbe;
      cancelAnimationFrame(probe.frame);
      return probe.changes;
    });
    expect(changes, 'Every rendered frame must keep the unchanged layout').toEqual([]);
    await page.screenshot({ path: testInfo.outputPath('reconciled-room.png') });
    expect(errors).toEqual([]);
  } finally {
    release();
    await page.unrouteAll({ behavior: 'wait' });
  }
});
