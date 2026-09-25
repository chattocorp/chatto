// SPDX-License-Identifier: Apache-2.0

import { RealtimeSubscribe } from '@chatto/api-types/realtime/v1/realtime_pb';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';

for (const recovery of ['resume', 'snapshot'] as const) {
  test(`posts from a saved room before realtime ${recovery} completes`, async ({
    page,
    chatPage,
    roomPage
  }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page);
    const seed = await roomPage.sendMessage('Saved room for early commands');
    const seedId = await seed.getEventId();
    await expect
      .poll(() =>
        page.evaluate(async (id) => {
          const db = await new Promise<IDBDatabase>((resolve, reject) => {
            const request = indexedDB.open('chatto-saved-views', 2);
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
          });
          try {
            return await new Promise<boolean>((resolve, reject) => {
              const request = db.transaction('resources').objectStore('resources').getAll();
              request.onsuccess = () =>
                resolve(
                  request.result.some((row) =>
                    row.data.events?.some((event: { id: string }) => event.id === id)
                  )
                );
              request.onerror = () => reject(request.error);
            });
          } finally {
            db.close();
          }
        }, seedId)
      )
      .toBe(true);

    let subscribed = false;
    let released = false;
    const pending: (() => void)[] = [];
    await page.routeWebSocket('**/api/realtime', (socket) => {
      const server = socket.connectToServer();
      socket.onMessage((message) => {
        if (typeof message === 'string') return server.send(message);
        const subscribe = RealtimeSubscribe.fromBinary(message);
        expect(subscribe.resumeCursor).toBeTruthy();
        if (recovery === 'snapshot') subscribe.resumeCursor = 'expired-early-command-test';
        server.send(Buffer.from(subscribe.toBinary()));
        subscribed = true;
      });
      server.onMessage((message) => {
        if (released) socket.send(message);
        else pending.push(() => socket.send(message));
      });
    });
    const release = () => {
      released = true;
      for (const send of pending.splice(0)) send();
    };
    try {
      await page.reload();
      // Subscription starts only after the saved viewer has been verified.
      await expect.poll(() => subscribed).toBe(true);
      await expect.poll(() => pending.length).toBeGreaterThan(0);
      const body = `Posted before ${recovery} catch-up`;
      await roomPage.sendMessage(body);
      await expect(page.getByText(body, { exact: true })).toHaveCount(1);
      release();
      await waitForRoomReady(page);
      await expect(page.getByText(body, { exact: true })).toHaveCount(1);
      await page.reload();
      await waitForRoomReady(page);
      await expect(page.getByText(body, { exact: true })).toHaveCount(1);
      expect(errors).toEqual([]);
    } finally {
      release();
    }
  });
}
