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
      const recipientRoom = recipientPage
        .getByTestId('server-sidebar')
        .locator(`a[href="${routes.room(roomId)}"]`);
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
      const overviewInvitation = recipientPage
        .getByRole('listitem')
        .filter({ hasText: 'Private invitation project' });
      await expect(overviewInvitation.getByText('Invited', { exact: true })).toBeVisible();
      await recipientPage.screenshot({
        animations: 'disabled',
        path: test.info().outputPath('overview-room-invitation.png')
      });
      await overviewInvitation.getByRole('link').click();
      const welcome = recipientPage.getByTestId('room-invitation-preview');
      await expect(welcome).toBeVisible();
      await expect(welcome).toContainText(sender.displayName);
      const invitationNotifications = async () => {
        const response = await connectPost<{
          occurrences: {
            id: string;
            unread: boolean;
            signal?: { roomInvitationReceived?: { room?: { id: string } } };
          }[];
        }>(recipientPage, 'chatto.api.v1.NotificationService/ListNotificationOccurrences', {});
        return (response.occurrences ?? []).filter(
          (item) => item.signal?.roomInvitationReceived?.room?.id === roomId
        );
      };
      // A plain room URL (including a push destination) does not answer or read the invite.
      await expect
        .poll(async () => (await invitationNotifications()).filter((item) => item.unread).length)
        .toBe(1);
      await expect(recipientRoom.getByTestId('room-notification-badge')).toBeVisible();
      if (answer === 'accept') {
        await recipientPage.goto(routes.settingsNotifications);
        const invitationPolicy = recipientPage.locator(
          'td[data-notification-field="roomInvitations"][data-notification-scope="server"] button'
        );
        await expect(invitationPolicy).toHaveAttribute('aria-label', /Push notification/);
        await expect(
          recipientPage.locator(
            `td[data-notification-field="roomInvitations"][data-notification-scope="room:${roomId}"] button`
          )
        ).toBeEnabled();
        await recipientPage.screenshot({
          animations: 'disabled',
          path: test.info().outputPath('room-invitation-preferences.png')
        });
      }
      await recipientPage.goto(routes.notifications);
      const notification = recipientPage
        .getByTestId('notification-group')
        .filter({ hasText: 'invited you to a room' });
      await expect(notification).toContainText(sender.displayName);
      await recipientPage.screenshot({
        animations: 'disabled',
        path: test.info().outputPath('room-invitation-notification.png')
      });
      await notification.locator(':scope > button').click();
      await expect(recipientPage).toHaveURL(new RegExp(`${routes.room(roomId)}$`));
      await expect(welcome).toBeVisible();
      await expect
        .poll(async () => {
          const items = await invitationNotifications();
          return items.length === 1 && !items[0]?.unread;
        })
        .toBe(true);
      await expect(recipientRoom.getByTestId('room-notification-badge')).not.toBeVisible();
      await welcome.getByTestId('user-identity').click();
      await expect(recipientPage.getByRole('dialog')).toContainText(sender.displayName);
      await recipientPage.getByRole('button', { name: 'View profile', exact: true }).click();
      const profile = recipientPage.getByRole('dialog', { name: 'View profile', exact: true });
      await expect(profile.getByTestId('profile-user-card')).toContainText(sender.displayName);
      await recipientPage.screenshot({
        animations: 'disabled',
        path: test.info().outputPath('inviter-profile.png')
      });
      await recipientPage.keyboard.press('Escape');
      await expect(profile).not.toBeVisible();
      await welcome.getByTestId('user-identity').click();
      await recipientPage.getByRole('button', { name: 'Open conversation', exact: true }).click();
      await expect(recipientPage).toHaveURL(new RegExp(`/user/${sender.id}$`));
      await expect(new RoomPage(recipientPage).messageInput).toBeVisible();
      await recipientPage.goto(routes.room(roomId));
      await expect(welcome).toBeVisible();
      await expect(welcome).toContainText('Expires on');
      await recipientPage.reload();
      await expect(welcome).toBeVisible();
      await recipientPage.setViewportSize({ width: 390, height: 844 });
      await expect(welcome.getByRole('button', { name: 'Accept invitation' })).toBeInViewport();
      await expect(welcome.getByRole('button', { name: 'Decline', exact: true })).toBeInViewport();
      await recipientPage.screenshot({
        animations: 'disabled',
        path: test.info().outputPath('invitation-welcome.png')
      });
      await welcome
        .getByRole('button', {
          name: answer === 'accept' ? 'Accept invitation' : 'Decline',
          exact: true
        })
        .click();
      if (answer === 'decline') {
        const confirmation = recipientPage.getByRole('dialog');
        await expect(confirmation).toContainText(
          'You will need a new invitation to join this room.'
        );
        await recipientPage.screenshot({
          animations: 'disabled',
          path: test.info().outputPath('decline-invitation-confirmation.png')
        });
        await confirmation.getByRole('button', { name: 'Decline', exact: true }).click();
      }
      await expect(welcome).not.toBeVisible();
      await expect.poll(async () => (await invitationNotifications()).length).toBe(0);
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

        await recipientPage.setViewportSize({ width: 1280, height: 900 });
        await recipientPage.goto(routes.serverOverview);
        await expect(
          recipientPage.getByRole('heading', { name: 'Overview', exact: true })
        ).toBeVisible();

        await recipientRoom.click({ button: 'right' });
        await recipientPage.getByRole('menuitem', { name: 'Leave room', exact: true }).click();
        const leaveConfirmation = recipientPage.getByRole('dialog');
        await expect(leaveConfirmation).toContainText(
          'You will need a new invitation to join this room.'
        );
        await leaveConfirmation.getByRole('button', { name: 'Cancel', exact: true }).click();

        // Cache the disabled result while the recipient is still a room member.
        await senderRoom.click({ button: 'right' });
        await senderPage.getByRole('menuitem', { name: 'Invite member', exact: true }).click();
        await dialog.getByRole('combobox', { name: 'Members' }).fill(recipient.login);
        await expect(dialog.getByRole('option')).toBeDisabled();
        await expect(dialog.getByRole('option')).toContainText('Already in this room');
        await senderPage.screenshot({
          animations: 'disabled',
          path: test.info().outputPath('invite-existing-member.png')
        });
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
        // Opening the inviter's conversation establishes the last-room destination.
        await expect(recipientPage).toHaveURL(new RegExp(`/user/${sender.id}$`));
        await expect(new RoomPage(recipientPage).messageInput).toBeVisible();
        await expect(recipientRoom).not.toBeAttached();
      }
      expect(errors).toEqual([]);
    } finally {
      await recipientContext.close();
    }
  });
}
