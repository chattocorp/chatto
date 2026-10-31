import { expect } from '@playwright/test';
import { test } from './setup';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import { createRoomViaConnect, getDefaultRoomGroupIdViaConnect } from './fixtures/connectHelpers';
import * as routes from './routes';

for (const kind of ['room', 'room-group'] as const) {
  test(`${kind} settings preserve drafts on cancellation and resume browser history`, async ({
    page
  }) => {
    await loginAsAdminAndUsePrimaryServer(page);
    const groupId = await getDefaultRoomGroupIdViaConnect(page);
    const settings =
      kind === 'room'
        ? `${routes.serverAdminRooms}/${await createRoomViaConnect(page, 'guard-test', groupId)}`
        : `${routes.chat}/manage/room-groups/${groupId}`;
    const permissions = `${settings}/permissions`;
    const name = page.locator(
      kind === 'room' ? '#room-settings-name' : '#room-group-settings-name'
    );
    const dialog = page.getByRole('dialog', { name: 'Discard changes' });

    // Direct entry and pristine navigation establish real SvelteKit history entries.
    await page.goto(settings);
    const originalName = await name.inputValue();
    await page.locator(`a[href="${permissions}"]`).click();
    await expect(page).toHaveURL(permissions);
    await page.locator(`a[href="${settings}"]`).click();
    await name.fill('Unsaved draft');

    await page.evaluate(() => history.back());
    await expect(dialog).toBeVisible();
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(page).toHaveURL(settings);
    await expect(name).toHaveValue('Unsaved draft');

    await page.evaluate(() => history.back());
    await dialog.getByRole('button', { name: 'Discard changes', exact: true }).click();
    await expect(page).toHaveURL(permissions);
    await page.goForward();
    await expect(page).toHaveURL(settings);
    await expect(name).toHaveValue(originalName);

    await name.fill('Another draft');
    await page.locator(`a[href="${permissions}"]`).click();
    await dialog.getByRole('button', { name: 'Discard changes', exact: true }).click();
    await expect(page).toHaveURL(permissions);
    await page.goBack();
    await expect(name).toHaveValue(originalName);

    await name.fill('Forward draft');
    await page.evaluate(() => history.forward());
    await dialog.getByRole('button', { name: 'Discard changes', exact: true }).click();
    await expect(page).toHaveURL(permissions);
    await page.goBack();
    await expect(name).toHaveValue(originalName);

    await name.fill('Reload draft');
    const unload = page.waitForEvent('dialog');
    const reload = page.reload();
    const warning = await unload;
    expect(warning.type()).toBe('beforeunload');
    await warning.accept();
    await reload;
    await expect(name).toHaveValue(originalName);
    await expect(dialog).not.toBeVisible();
  });
}
