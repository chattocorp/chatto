// SPDX-License-Identifier: Apache-2.0

import { RealtimeSubscribe } from '@chatto/api-types/realtime/v1/realtime_pb';
import type { WebSocketRoute } from '@playwright/test';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';

for (const recovery of ['resume', 'snapshot'] as const) {
  test(`posts from a loaded room before realtime ${recovery} completes`, async ({
    page,
    chatPage,
    roomPage
  }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));

    // Pass traffic through until the test holds a reconnect's catch-up frames.
    let holding = false;
    let subscribed = false;
    let released = false;
    let current: WebSocketRoute | null = null;
    const pending: (() => void)[] = [];
    await page.routeWebSocket('**/api/realtime', (socket) => {
      current = socket;
      const held = holding;
      const server = socket.connectToServer();
      socket.onMessage((message) => {
        if (typeof message === 'string') return server.send(message);
        const subscribe = RealtimeSubscribe.fromBinary(message);
        if (held) {
          // A reconnect resumes from the cursor that this page holds in memory.
          expect(subscribe.resumeCursor).toBeTruthy();
          if (recovery === 'snapshot') subscribe.resumeCursor = 'expired-early-command-test';
          subscribed = true;
        }
        server.send(Buffer.from(subscribe.toBinary()));
      });
      server.onMessage((message) => {
        if (!held || released) socket.send(message);
        else pending.push(() => socket.send(message));
      });
    });
    const release = () => {
      released = true;
      for (const send of pending.splice(0)) send();
    };

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReady(page);
    await roomPage.sendMessage('Loaded room for early commands');
    try {
      holding = true;
      await current!.close();
      await expect.poll(() => subscribed).toBe(true);
      await expect.poll(() => pending.length).toBeGreaterThan(0);
      const body = `Posted before ${recovery} catch-up`;
      await roomPage.sendMessage(body);
      await expect(page.getByText(body, { exact: true })).toHaveCount(1);
      release();
      await waitForRoomReady(page);
      await expect(page.getByText(body, { exact: true })).toHaveCount(1);
      holding = false;
      await page.reload();
      await waitForRoomReady(page);
      await expect(page.getByText(body, { exact: true })).toHaveCount(1);
      expect(errors).toEqual([]);
    } finally {
      release();
    }
  });
}
