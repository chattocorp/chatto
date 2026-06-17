import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

test('create room with valid name succeeds', async ({ page, chatPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Create room using API-based method
  const roomName = await chatPage.createRoom();

  // Room should appear in the header
  await chatPage.expectRoomHeaderVisible(roomName);
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

  // Submit
  await chatPage.roomFormSubmitButton.click();

  // Should show error message from backend
  await chatPage.expectValidationError('room name must be 30 characters or less');
});

test('create room with description exceeding max length shows error', async ({
  page,
  chatPage
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Open the room creation modal
  await chatPage.openCreateRoomModal();

  // Fill in valid room name
  await chatPage.roomNameInput.fill('valid-room-name');

  // Fill in description that exceeds 500 characters
  const longDescription = 'a'.repeat(501);
  await chatPage.roomDescriptionInput.fill(longDescription);

  // Submit
  await chatPage.roomFormSubmitButton.click();

  // Should show error message from backend
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

test('create room form resets after creating a room', async ({ page, chatPage }) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();

  // Open the room creation modal and create a room via UI
  await chatPage.openCreateRoomModal();
  await chatPage.roomNameInput.fill('first-room');
  await chatPage.roomFormSubmitButton.click();

  // Wait for the dialog to close (room created successfully)
  await expect(page.getByRole('dialog')).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

  // Open the modal again — the name field should be empty
  await chatPage.openCreateRoomModal();
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
