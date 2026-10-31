// SPDX-License-Identifier: Apache-2.0

import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';

test('shares origin discovery on first visit and authenticated reload', async ({
  page,
  chatPage
}) => {
  let discoveryRequests = 0;
  const errors: string[] = [];
  page.on('request', (request) => {
    if (request.url().endsWith('/chatto.discovery.v1.ServerDiscoveryService/GetServer')) {
      discoveryRequests++;
    }
  });
  page.on('pageerror', (error) => errors.push(error.message));

  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
  expect(discoveryRequests).toBe(1);

  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  await waitForRoomReady(page);
  discoveryRequests = 0;
  await page.reload();
  await waitForRoomReady(page);
  expect(discoveryRequests).toBe(1);
  expect(errors).toEqual([]);
});
