import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withBootstrapAdminRequest } from './fixtures/adminRequest';
import { connectPost, createRoomViaConnect, joinRoomViaConnect } from './fixtures/connectHelpers';
import { RoomPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

/** Exercise invitation UI against independent browser sessions and durable server state. */
for (const answer of ['accept', 'decline'] as const) {
  test(`invites a member to a hidden room, survives reload, and ${answer}s on mobile`, async ({
    page: senderPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60_000);
    const errors: string[] = [];
    const sender = await createAndLoginTestUser(senderPage);
    const recipientContext = await browser.newContext({ baseURL: serverURL });
    try {
      const recipientPage = await recipientContext.newPage();
      for (const page of [senderPage, recipientPage]) {
        page.on('pageerror', (error) => errors.push(error.stack ?? error.message));
        page.on('console', (message) => {
          if (message.type() === 'error') errors.push(message.text());
        });
      }
      const recipient = await createAndLoginTestUser(recipientPage);
      const additionalContext = await browser.newContext({ baseURL: serverURL });
      const additional = await createAndLoginTestUser(await additionalContext.newPage(), {
        loginPrefix: 'additional'
      });
      await additionalContext.close();
      const roomId = await withBootstrapAdminRequest(serverURL, async (admin) => {
        const { group } = await connectPost<{ group: { id: string } }>(
          admin,
          'chatto.admin.v1.AdminRoomLayoutService/CreateRoomGroup',
          { name: 'Invitation project' }
        );
        const id = await createRoomViaConnect(admin, 'Private invitation project', group.id);
        for (const [userId, permission, decision] of [
          [sender.id, 'room.invite', 'PERMISSION_DECISION_ALLOW'],
          [sender.id, 'room.remove-member', 'PERMISSION_DECISION_ALLOW'],
          [recipient.id, 'room.list', 'PERMISSION_DECISION_DENY'],
          [recipient.id, 'room.join', 'PERMISSION_DECISION_DENY']
        ]) {
          await connectPost(admin, 'chatto.admin.v1.AdminPermissionService/SetUserPermission', {
            userId,
            permission,
            decision,
            scope: { kind: 'PERMISSION_SCOPE_KIND_ROOM', id }
          });
        }
        return id;
      });
      await joinRoomViaConnect(senderPage, roomId);
      await senderPage.goto(routes.room(roomId));
      await recipientPage.goto(routes.serverOverview);
      await expect(
        recipientPage.getByRole('heading', { name: 'Overview', exact: true })
      ).toBeVisible();
      const recipientRoom = recipientPage.locator(`a[href="${routes.room(roomId)}"]`);
      await expect(recipientRoom).not.toBeAttached();
      await expect(
        recipientPage.getByRole('button', { name: 'Invitation project', exact: true })
      ).not.toBeVisible();

      const senderRoom = senderPage.locator(`a[href="${routes.room(roomId)}"]`);
      await senderRoom.click({ button: 'right' });
      await senderPage.getByRole('menuitem', { name: 'Invite member', exact: true }).click();
      const dialog = senderPage.getByRole('dialog', {
        name: 'Invite members to #Private invitation project'
      });
      await dialog.getByRole('combobox', { name: 'Members' }).fill(recipient.login);
      await dialog.getByRole('option').click();
      await expect(
        dialog.getByRole('button', { name: `Remove ${recipient.displayName}`, exact: true })
      ).toBeVisible();
      await dialog.getByRole('combobox', { name: 'Members' }).fill(additional.login);
      await dialog.getByRole('option').click();
      await expect(dialog.getByTestId('combobox-selection')).toHaveCount(2);
      if (answer === 'accept') {
        await senderPage.screenshot({
          animations: 'disabled',
          path: test.info().outputPath('invite-members-desktop.png')
        });
        await senderPage.emulateMedia({ colorScheme: 'dark' });
        await expect(senderPage.locator('html')).toHaveAttribute('data-theme', 'dark');
        await senderPage.screenshot({
          animations: 'disabled',
          path: test.info().outputPath('invite-members-dark.png')
        });
        await senderPage.setViewportSize({ width: 390, height: 844 });
        await expect(
          dialog.getByRole('button', { name: 'Send invitations', exact: true })
        ).toBeInViewport();
        await expect(dialog.getByRole('combobox', { name: 'Members' })).toBeInViewport();
        await senderPage.screenshot({
          animations: 'disabled',
          path: test.info().outputPath('invite-members-mobile.png')
        });
        await senderPage.setViewportSize({ width: 1280, height: 900 });
      }
      await dialog.getByRole('button', { name: 'Send invitations', exact: true }).click();
      await expect(dialog).not.toBeVisible();
      const sent = await connectPost<{ invitations: { recipientId: string }[] }>(
        senderPage,
        'chatto.api.v1.RoomInvitationService/ListRoomInvitations',
        { sent: true, pendingOnly: true }
      );
      expect(sent.invitations.map((invitation) => invitation.recipientId)).toEqual(
        expect.arrayContaining([recipient.id, additional.id])
      );

      await expect(recipientRoom).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(recipientRoom.getByRole('img', { name: 'You have been invited' })).toBeVisible();
      await expect(
        recipientPage.getByRole('button', { name: 'Invitation project', exact: true })
      ).toBeVisible();
      await recipientRoom.click();
      const welcome = recipientPage.getByTestId('room-invitation-preview');
      await expect(welcome).toBeVisible();
      await expect(welcome).toContainText(sender.displayName);
      await expect(welcome).toContainText('Expires on');
      await recipientPage.reload();
      await expect(welcome).toBeVisible();
      await recipientPage.setViewportSize({ width: 390, height: 844 });
      await expect(welcome.getByRole('button', { name: 'Accept invitation' })).toBeInViewport();
      await expect(welcome.getByRole('button', { name: 'Decline', exact: true })).toBeInViewport();
      await welcome
        .getByRole('button', {
          name: answer === 'accept' ? 'Accept invitation' : 'Decline',
          exact: true
        })
        .click();
      await expect(welcome).not.toBeVisible();
      if (answer === 'accept') {
        await expect
          .poll(async () => {
            const result = await connectPost<{ room: { viewerState: { isMember: boolean } } }>(
              recipientPage,
              'chatto.api.v1.RoomDirectoryService/GetRoom',
              { roomId }
            );
            return result.room.viewerState.isMember;
          })
          .toBe(true);
        await recipientPage.reload();
        await expect(new RoomPage(recipientPage).messageInput).toBeVisible();
        await expect(recipientPage.getByTestId('room-invitation-preview')).not.toBeVisible();

        await recipientPage.goto(routes.serverOverview);
        await expect(
          recipientPage.getByRole('heading', { name: 'Overview', exact: true })
        ).toBeVisible();

        // Cache an empty search while the recipient is still a room member.
        await senderRoom.click({ button: 'right' });
        await senderPage.getByRole('menuitem', { name: 'Invite member', exact: true }).click();
        await dialog.getByRole('combobox', { name: 'Members' }).fill(recipient.login);
        await expect(dialog).toContainText('No users found');
        await dialog.getByRole('combobox', { name: 'Members' }).press('Escape');
        await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
        await expect(dialog).not.toBeVisible();

        await senderPage.getByTestId('privileged-mode-toggle').click();
        await senderPage
          .getByRole('dialog', { name: 'Enable privileged mode' })
          .getByRole('button', { name: 'Enable privileged mode' })
          .click();
        await expect(
          senderPage.getByRole('button', { name: 'Disable privileged mode' })
        ).toBeEnabled();
        await connectPost(senderPage, 'chatto.api.v1.RoomService/RemoveUser', {
          roomId,
          userId: recipient.id,
          reason: 'Invitation regression test'
        });
        await senderPage.getByRole('button', { name: 'Disable privileged mode' }).click();
        await expect(
          senderPage.getByRole('button', { name: 'Enable privileged mode' })
        ).toBeEnabled();

        // The same query must find the removed member without reloading the sender.
        await senderRoom.click({ button: 'right' });
        await senderPage.getByRole('menuitem', { name: 'Invite member', exact: true }).click();
        await dialog.getByRole('combobox', { name: 'Members' }).fill(recipient.login);
        await expect(dialog.getByRole('option')).toBeVisible();
        await dialog.getByRole('option').click();
        await dialog.getByRole('button', { name: 'Send invitations', exact: true }).click();
        await expect(dialog).not.toBeVisible();
      } else {
        await expect(recipientRoom).not.toBeAttached();
        await recipientPage.setViewportSize({ width: 1280, height: 900 });
        await expect(
          recipientPage.getByRole('button', { name: 'Invitation project', exact: true })
        ).not.toBeVisible();
        await recipientPage.reload();
        await expect(
          recipientPage.getByRole('heading', { name: 'Overview', exact: true })
        ).toBeVisible();
        await expect(recipientRoom).not.toBeAttached();
      }
      expect(errors).toEqual([]);
    } finally {
      await recipientContext.close();
    }
  });
}
