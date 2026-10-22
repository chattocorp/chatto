import { expect } from '@playwright/test';
import { test } from './setup';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import {
  connectPost,
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect
} from './fixtures/connectHelpers';
import * as routes from './routes';

/** Exercise inherited decisions, settings navigation, and a projected permission correction. */
test('configuration warnings link to permissions and clear after a read grant', async ({
  page
}) => {
  await loginAsAdminAndUsePrimaryServer(page);
  const roomName = 'Feedback inbox';
  const roomId = await createRoomViaConnect(
    page,
    roomName,
    await getDefaultRoomGroupIdViaConnect(page)
  );
  await connectPost(page, 'chatto.admin.v1.AdminRoleService/CreateRole', {
    name: 'feedback',
    displayName: 'Feedback role'
  });
  await connectPost(page, 'chatto.admin.v1.AdminPermissionService/SetRolePermission', {
    roleName: 'feedback',
    permission: 'message.post',
    scope: { kind: 'PERMISSION_SCOPE_KIND_SERVER' },
    decision: 'PERMISSION_DECISION_ALLOW'
  });
  await connectPost(page, 'chatto.admin.v1.AdminPermissionService/SetRolePermission', {
    roleName: 'feedback',
    permission: 'message.read',
    scope: { kind: 'PERMISSION_SCOPE_KIND_ROOM', id: roomId },
    decision: 'PERMISSION_DECISION_DENY'
  });

  await page.goto(routes.serverAdmin('permissions'));
  await page.getByRole('link', { name: 'Configuration warnings', exact: true }).click();
  await expect(page.getByText(roomName, { exact: true })).toBeVisible();
  await expect(page.getByText(/Other roles and user-specific permissions/)).toBeVisible();
  await page
    .getByText(roomName, { exact: true })
    .locator('..')
    .locator('..')
    .getByRole('link', { name: 'Review role permissions' })
    .click();
  await expect(page).toHaveURL(/\/manage\/server\/permissions\/feedback$/);
  await expect(page.getByTestId('subject-permissions-matrix')).toBeVisible();

  await page.getByRole('link', { name: 'Configuration warnings', exact: true }).click();
  await expect(page.getByText(roomName, { exact: true })).toBeVisible();
  await connectPost(page, 'chatto.admin.v1.AdminPermissionService/SetRolePermission', {
    roleName: 'feedback',
    permission: 'message.read',
    scope: { kind: 'PERMISSION_SCOPE_KIND_ROOM', id: roomId },
    decision: 'PERMISSION_DECISION_ALLOW'
  });
  await expect(page.getByText(roomName, { exact: true })).not.toBeVisible();
});
