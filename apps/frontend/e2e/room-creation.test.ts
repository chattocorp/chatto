import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test('creates and renames a flexible Unicode room name', async ({
  page,
  chatPage,
  serverAdminRoomsPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  const roomName = 'Team chat 💬';
  await chatPage.openCreateRoomModal();
  await chatPage.roomNameInput.fill(roomName);
  await chatPage.roomFormSubmitButton.click();
  await expect(page.getByRole('dialog')).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  await expect(serverAdminRoomsPage.roomSettingsNameInput).toHaveValue(roomName, {
    timeout: TIMEOUTS.REALTIME_EVENT
  });

  const renamedRoom = 'Team chat 💬 / updates!';
  await serverAdminRoomsPage.updateCurrentRoom(renamedRoom);
  await serverAdminRoomsPage.returnToOverview();
  await serverAdminRoomsPage.expectRoomVisible(renamedRoom, TIMEOUTS.REALTIME_EVENT);
});

test('room header shows channel description on desktop', async ({ page, chatPage }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await createAndLoginTestUser(page);
  await chatPage.goto();

  const description = `Header description ${Date.now()}`;
  const describedRoom = await chatPage.createRoom(undefined, description);

  await chatPage.expectRoomHeaderVisible(describedRoom);
  await expect(page.getByText(description, { exact: true })).toBeVisible();

  const plainRoom = await chatPage.createRoom();

  await chatPage.expectRoomHeaderVisible(plainRoom);
  await expect(page.getByText(description, { exact: true })).not.toBeVisible();
});

test('create room with empty name has disabled submit button', async ({ page, chatPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Open the room creation modal
  await chatPage.openCreateRoomModal();

  // Submit button should be disabled when name is empty
  await chatPage.expectRoomSubmitDisabled();
});

test('create room with name exceeding max length shows error', async ({ page, chatPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Open the room creation modal
  await chatPage.openCreateRoomModal();

  // Fill in room name that exceeds 30 characters
  const longName = 'a'.repeat(31);
  await chatPage.roomNameInput.fill(longName);

  // The client mirrors the server's limit and keeps the invalid form local.
  await chatPage.expectValidationError('Room name cannot exceed 30 characters');
  await chatPage.expectRoomSubmitDisabled();
});

test('room settings reject a description exceeding the maximum length', async ({
  page,
  chatPage,
  serverAdminRoomsPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  await chatPage.openCreateRoomModal();
  await chatPage.roomNameInput.fill('valid-room-name');
  await chatPage.roomFormSubmitButton.click();
  await expect(serverAdminRoomsPage.roomSettingsNameInput).toHaveValue('valid-room-name', {
    timeout: TIMEOUTS.REALTIME_EVENT
  });

  const longDescription = 'a'.repeat(501);
  await serverAdminRoomsPage.roomSettingsDescriptionInput.fill(longDescription);
  await page.getByRole('button', { name: 'Save Changes' }).click();

  await chatPage.expectValidationError('room description must be 500 characters or less');
});

test('can leave room immediately after creating it', async ({ page, chatPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Create a new room
  const roomName = await chatPage.createRoom();

  // Verify we're in the room
  await chatPage.expectRoomHeaderVisible(roomName);

  // Get the current URL to verify we leave
  const roomUrl = page.url();

  // Click the leave room button - should show confirmation modal
  await page.getByTitle('Leave room').click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByText(`Are you sure you want to leave`)).toBeVisible();

  // Confirm leave
  await page.getByRole('dialog').getByRole('button', { name: 'Leave Room' }).click();

  // Should navigate away from the room to a different URL
  // Use a function predicate to wait for URL to change from the current room
  await page.waitForURL((url) => url.href !== roomUrl, { timeout: TIMEOUTS.REALTIME_EVENT });

  // The room should no longer appear in the sidebar (may need time to update)
  await expect(chatPage.roomList.getByRole('link', { name: `# ${roomName}` })).not.toBeVisible({
    timeout: TIMEOUTS.UI_STANDARD
  });
});

test('can cancel leaving a room', async ({ page, chatPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Create a new room
  const roomName = await chatPage.createRoom();

  // Verify we're in the room
  await chatPage.expectRoomHeaderVisible(roomName);

  // Get the current URL
  const roomUrl = page.url();

  // Click the leave room button - should show confirmation modal
  await page.getByTitle('Leave room').click();
  await expect(page.getByRole('dialog')).toBeVisible();

  // Cancel leave
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();

  // Modal should close
  await expect(page.getByRole('dialog')).not.toBeVisible();

  // Should still be in the same room
  expect(page.url()).toBe(roomUrl);
  await chatPage.expectRoomHeaderVisible(roomName);
});

test('create room form resets after creating a room', async ({
  page,
  chatPage,
  serverAdminRoomsPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Open the room creation modal and create a room via UI
  await chatPage.openCreateRoomModal();
  await chatPage.roomNameInput.fill('first-room');
  await chatPage.roomFormSubmitButton.click();

  // Creation closes the dialog and opens the new room's settings.
  await expect(page.getByRole('dialog')).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  await expect(serverAdminRoomsPage.roomSettingsNameInput).toHaveValue('first-room', {
    timeout: TIMEOUTS.REALTIME_EVENT
  });

  // Return to the overview and open a fresh dialog.
  await serverAdminRoomsPage.returnToOverview();
  await serverAdminRoomsPage.newRoomButton('Lobby').click();
  await expect(chatPage.roomNameInput).toHaveValue('');
});

test('freshly created room shows join event and Today header', async ({
  page,
  chatPage,
  roomPage
}) => {
  const testUser = await createAndLoginTestUser(page);
  await chatPage.goto();

  // Create a new room (automatically navigates to it)
  await chatPage.createRoom();

  // Should see the "Today" day separator
  await roomPage.expectDaySeparator('Today');

  // Should see the join event with the user's display name
  await expect(page.getByText(`${testUser.displayName} joined the room`)).toBeVisible();
});
