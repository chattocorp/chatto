import { expect } from '@playwright/test';
import { withBootstrapAdminRequest } from './fixtures/adminRequest';
import { test } from './setup';
import {
  grantPermission,
  loginAsAdmin,
  loginAsAdminAndUsePrimaryServer
} from './fixtures/testUser';
import {
  connectPost,
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect
} from './fixtures/connectHelpers';
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
  await expect(dialog).toContainText('Choose the roles to activate for 15 minutes.');
  await expect(dialog.getByRole('button', { name: 'Enable privileged mode' })).toBeDisabled();
  await dialog.getByText('owner', { exact: true }).click();
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
  await page.getByRole('dialog').getByText('owner', { exact: true }).click();
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
  const suspensionRequests: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  page.on('request', (request) => {
    if (request.url().endsWith('/ListSuspensions')) suspensionRequests.push(request.method());
  });
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page, { activatePrivilegedMode: false });
  await page.goto(routes.serverAdmin('moderation'));

  const moderation = page.getByRole('link', { name: 'Moderation', exact: true });
  const denied = page.getByText('Access Denied', { exact: true });
  const emptySuspensions = page.getByText('No active room suspensions', { exact: true });
  const enable = page.getByRole('button', { name: 'Enable privileged mode' });
  const disable = page.getByRole('button', { name: 'Disable privileged mode' });
  await expect(denied).toBeVisible();
  await expect(moderation).not.toBeVisible();
  expect(suspensionRequests).toHaveLength(0);

  await enable.click();
  await page.getByRole('dialog').getByText('owner', { exact: true }).click();
  await page
    .getByRole('dialog', { name: 'Enable privileged mode' })
    .getByRole('button', { name: 'Enable privileged mode' })
    .click();
  await expect(moderation).toBeVisible();
  await expect(emptySuspensions).toBeVisible();
  expect(suspensionRequests.length).toBeGreaterThan(0);

  await disable.click();
  await expect(denied).toBeVisible();
  await expect(moderation).not.toBeVisible();
  await expect(emptySuspensions).not.toBeVisible();

  await enable.click();
  await page.getByRole('dialog').getByText('owner', { exact: true }).click();
  await page
    .getByRole('dialog', { name: 'Enable privileged mode' })
    .getByRole('button', { name: 'Enable privileged mode' })
    .click();
  await expect(moderation).toBeVisible();
  await expect(emptySuspensions).toBeVisible();
  expect(pageErrors).toEqual([]);
});

test('owner sees restricted rooms only while privileged mode is active', async ({ page }) => {
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page);
  const roomName = `vault${Date.now()}`;
  const roomId = await createRoomViaConnect(
    page,
    roomName,
    await getDefaultRoomGroupIdViaConnect(page)
  );
  await connectPost(page, 'chatto.api.v1.RoomService/LeaveRoom', { roomId });
  for (const permission of ['room.list', 'room.join', 'message.read']) {
    await connectPost(page, 'chatto.admin.v1.AdminPermissionService/SetRolePermission', {
      roleName: 'everyone',
      permission,
      decision: 'PERMISSION_DECISION_DENY',
      scope: { kind: 'PERMISSION_SCOPE_KIND_ROOM', id: roomId }
    });
  }

  await page.goto(routes.serverOverview);
  const restrictedRoom = page.getByText(roomName, { exact: true });
  const enable = page.getByRole('button', { name: 'Enable privileged mode' });
  const disable = page.getByRole('button', { name: 'Disable privileged mode' });
  await expect(disable).toBeVisible();
  await expect(restrictedRoom).toBeVisible();

  await disable.click();
  await expect(enable).toBeVisible();
  await expect(restrictedRoom).not.toBeVisible();

  await enable.click();
  await page.getByRole('dialog').getByText('owner', { exact: true }).click();
  await page
    .getByRole('dialog', { name: 'Enable privileged mode' })
    .getByRole('button', { name: 'Enable privileged mode' })
    .click();
  await expect(disable).toBeVisible();
  await expect(restrictedRoom).toBeVisible();
});

test('a protected role can be selected without the owner override', async ({
  page,
  serverRolesPage,
  serverURL
}) => {
  await page.goto(routes.root);
  const owner = await loginAsAdmin(page);
  await serverRolesPage.gotoCreateRole('server');
  await serverRolesPage.fillRoleForm({ name: 'room-helper', displayName: 'Room helper' });
  await page.getByText('Require Shield Mode', { exact: true }).click();
  await serverRolesPage.submitButton.click();
  await expect(page).toHaveURL(new RegExp(routes.serverAdminRole('room-helper') + '$'));
  await expect(page.getByRole('checkbox', { name: 'Require Shield Mode' })).toBeChecked();
  await grantPermission(page, 'room-helper', 'room.remove-member');
  await connectPost(page, 'chatto.admin.v1.AdminUserService/AssignRole', {
    userId: owner.id,
    roleName: 'room-helper'
  });
  await page.goto(routes.serverAdmin('moderation'));
  await page.getByRole('button', { name: 'Disable privileged mode' }).click();
  await expect(page.getByText('Access Denied', { exact: true })).toBeVisible();
  // A role manager changes protection in a separate session. The mounted page
  // must gain and lose access without navigating or activating the shield.
  await withBootstrapAdminRequest(serverURL, async (admin) => {
    for (const requiresShieldMode of [false, true]) {
      await connectPost(admin, 'chatto.admin.v1.AdminRoleService/UpdateRole', {
        name: 'room-helper',
        requiresShieldMode,
        updateMask: 'requiresShieldMode'
      });
      await expect(page.getByText('No active room suspensions', { exact: true })).toBeVisible({
        visible: !requiresShieldMode
      });
    }
  });
  await page.getByRole('button', { name: 'Enable privileged mode' }).click();
  const dialog = page.getByRole('dialog', { name: 'Enable privileged mode' });
  await expect(dialog.getByRole('checkbox', { name: 'owner' })).not.toBeChecked();
  await dialog.getByText('room-helper', { exact: true }).click();
  await dialog.getByRole('button', { name: 'Enable privileged mode' }).click();
  await expect(page.getByText('No active room suspensions', { exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Roles', exact: true })).not.toBeVisible();
});
