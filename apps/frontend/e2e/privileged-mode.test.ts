import { expect } from '@playwright/test';
import { test } from './setup';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import { connectPost } from './fixtures/connectHelpers';
import * as routes from './routes';

test('owner explicitly activates and deactivates privileged mode', async ({ page }) => {
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page, { activatePrivilegedMode: false });
  await page.goto(routes.chat);

  const enable = page.getByRole('button', { name: 'Enable privileged mode' });
  await page.getByRole('link', { name: '# general', exact: true }).click();
  const roomHeading = page.getByRole('heading', { name: '# general' });
  await expect(roomHeading).toBeVisible();
  await roomHeading.evaluate((element) => {
    element.setAttribute('data-privileged-mode-mount', 'original');
  });
  await expect(enable).toBeVisible();
  await expect(page.getByRole('button', { name: 'New Group' })).not.toBeVisible();

  await enable.click();
  const dialog = page.getByRole('dialog', { name: 'Enable privileged mode' });
  await expect(dialog).toContainText(
    'For 15 minutes, this enables the additional permissions assigned to you on this server.'
  );
  await dialog.getByRole('button', { name: 'Enable privileged mode' }).click();

  const disable = page.getByRole('button', { name: 'Disable privileged mode' });
  await expect(disable).toBeVisible();
  await expect(page.getByRole('button', { name: 'New Group' })).toBeVisible();
  await expect(roomHeading).toHaveAttribute('data-privileged-mode-mount', 'original');

  await disable.click();
  await expect(enable).toBeVisible();
  await expect(page.getByRole('button', { name: 'New Group' })).not.toBeVisible();
  await expect(roomHeading).toHaveAttribute('data-privileged-mode-mount', 'original');

  await enable.click();
  await page
    .getByRole('dialog', { name: 'Enable privileged mode' })
    .getByRole('button', { name: 'Enable privileged mode' })
    .click();
  await expect(disable).toBeVisible();
  await expect(page.getByRole('button', { name: 'New Group' })).toBeVisible();

  for (const eventType of ['PrivilegedModeActivatedEvent', 'PrivilegedModeDeactivatedEvent']) {
    const response = await connectPost<{ entries?: Array<{ eventType?: string }> }>(
      page,
      'chatto.admin.v1.AdminEventLogService/ListEvents',
      { limit: 10, filter: { eventType } }
    );
    expect(response.entries).toEqual(
      expect.arrayContaining([expect.objectContaining({ eventType })])
    );
  }
});

test('Moderation follows effective permission on direct access and mode changes', async ({
  page
}) => {
  const pageErrors: string[] = [];
  const banRequests: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  page.on('request', (request) => {
    if (request.url().endsWith('/ListBans')) banRequests.push(request.method());
  });
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page, { activatePrivilegedMode: false });
  await page.goto(routes.serverAdmin('moderation'));

  const moderation = page.getByRole('link', { name: 'Moderation', exact: true });
  const denied = page.getByText('Access Denied', { exact: true });
  const emptyBans = page.getByText('No active room bans', { exact: true });
  const enable = page.getByRole('button', { name: 'Enable privileged mode' });
  const disable = page.getByRole('button', { name: 'Disable privileged mode' });
  await expect(denied).toBeVisible();
  await expect(moderation).not.toBeVisible();
  expect(banRequests).toHaveLength(0);

  await enable.click();
  await page
    .getByRole('dialog', { name: 'Enable privileged mode' })
    .getByRole('button', { name: 'Enable privileged mode' })
    .click();
  await expect(moderation).toBeVisible();
  await expect(emptyBans).toBeVisible();
  expect(banRequests.length).toBeGreaterThan(0);

  await disable.click();
  await expect(denied).toBeVisible();
  await expect(moderation).not.toBeVisible();
  await expect(emptyBans).not.toBeVisible();

  await enable.click();
  await page
    .getByRole('dialog', { name: 'Enable privileged mode' })
    .getByRole('button', { name: 'Enable privileged mode' })
    .click();
  await expect(moderation).toBeVisible();
  await expect(emptyBans).toBeVisible();
  expect(pageErrors).toEqual([]);
});
