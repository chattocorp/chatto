import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';
import { adminGraphql, createBootstrapAdminRequest } from '../fixtures/adminRequest';
import { graphqlQuery } from '../fixtures/graphqlHelpers';
import { loginAsAdmin, logoutCurrentUser } from '../fixtures/testUser';
import { RoomPage } from './RoomPage';

/**
 * Page object for the main chat interface.
 * Handles sidebar navigation, server metadata, and room entry.
 */
export class ChatPage {
  constructor(readonly page: Page) {}

  /** The room list container in the sidebar */
  get roomList(): Locator {
    return this.page.locator('.room-list');
  }

  /**
   * Navigate to the chat page.
   * Note: users may be redirected to the server root, their last room, or
   * another chat-scoped page based on their local navigation state.
   */
  async goto(): Promise<void> {
    await this.page.goto('/chat');
    // Wait for any /chat path - redirects happen based on user state
    await this.page.waitForURL((url) => url.pathname.startsWith('/chat'));
  }

  /**
   * Return the legacy server-scope discriminator used by GraphQL operations
   * that still expose a `spaceId` input.
   */
  async getServerScopeId(): Promise<string> {
    return 'server';
  }

  /** Return the bootstrap server display name. */
  async getServerName(): Promise<string> {
    const data = await graphqlQuery<{
      server: { profile: { name: string } } | null;
    }>(this.page, `query { server { profile { name } } }`);
    if (!data.server) {
      throw new Error('Server query returned no data — bootstrap profile likely broken');
    }
    return data.server.profile.name;
  }

  /**
   * Enter a room by clicking it in the sidebar.
   * Returns a RoomPage for interacting with messages.
   * If already in the room, skips navigation to avoid disrupting WebSocket
   * subscriptions.
   * Always waits for room UI to be ready before returning.
   */
  async enterRoom(roomName: string): Promise<RoomPage> {
    const link = this.roomList.getByRole('link', { name: `# ${roomName}` });
    await expect(link).toBeVisible();

    // Check if already in this room (aria-current="page" indicates active link)
    const isActive = await link.getAttribute('aria-current');
    if (isActive !== 'page') {
      await link.click();
      await this.page.waitForURL(routes.patterns.anyRoom);
    }

    // Wait for room UI to be fully loaded (header and message input)
    await expect(this.getRoomHeader(roomName)).toBeVisible({ timeout: 5000 });
    await expect(this.page.getByTestId('message-input')).toBeVisible({ timeout: 5000 });

    return new RoomPage(this.page);
  }

  // --- Server Icon Indicators ---

  /**
   * Get the container div for a server icon by server name.
   * Scopes to the sidebar entry wrapping the button and any unread dots.
   */
  getServerIconContainer(serverName: string): Locator {
    return this.page
      .locator('.server-gutter')
      .locator('div', { has: this.page.getByRole('link', { name: serverName, exact: true }) });
  }

  /** Get the unread dot locator for a specific server. */
  getServerUnreadDot(serverName: string): Locator {
    return this.getServerIconContainer(serverName).getByTestId('server-unread-dot');
  }

  /** Click the unread dot on a specific server icon. */
  async clickServerUnreadDot(serverName: string): Promise<void> {
    await this.getServerUnreadDot(serverName).click();
  }

  /** Assert that a specific server icon shows an unread dot. */
  async expectServerHasUnread(serverName: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.getServerUnreadDot(serverName)).toBeVisible(options);
  }

  /** Assert that a specific server icon does NOT show an unread dot. */
  async expectServerHasNoUnread(serverName: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.getServerUnreadDot(serverName)).not.toBeVisible(options);
  }

  // --- Room Creation ---

  /** The room name input field in the admin room creation modal */
  get roomNameInput(): Locator {
    return this.page.getByLabel('Room Name');
  }

  /** The room description input field in the admin room creation modal */
  get roomDescriptionInput(): Locator {
    return this.page.getByLabel('Description (optional)');
  }

  /** The submit button in the room creation form */
  get roomFormSubmitButton(): Locator {
    return this.page.locator('form').getByRole('button', { name: 'Create Room' });
  }

  /** The room header (visible after navigating to a room) */
  getRoomHeader(roomName: string): Locator {
    return this.page.getByRole('heading', { name: `# ${roomName}` });
  }

  /**
   * Open the room creation modal on the admin rooms page.
   * Navigates to the admin rooms page and clicks "New Room".
   *
   * Issue #330 / ADR-027: with auto-join, the test user lands as a regular
   * member of the bootstrap server and the admin route 403s. Logout then
   * re-authenticate as e2eadmin (the bootstrap owner) before navigating so
   * the previous session's permissions don't leak into the page's reactive
   * state.
   */
  async openCreateRoomModal(): Promise<void> {
    await logoutCurrentUser(this.page);
    await loginAsAdmin(this.page);
    await this.page.goto(routes.serverAdminRooms);
    await expect(this.page).toHaveURL(/\/server-admin\/rooms/);
    await this.page.getByRole('button', { name: 'New Room' }).click();
    await expect(this.roomNameInput).toBeVisible();
  }

  /**
   * Create a new room via the GraphQL API, then navigate to it.
   * Much faster than UI-based creation and used for test setup.
   * Returns the room name for reference.
   */
  async createRoom(name?: string, description?: string): Promise<string> {
    const roomName = name ?? `test-room-${Date.now()}`;
    const adminContext = await createBootstrapAdminRequest(new URL(this.page.url()).origin);
    let roomId: string;
    try {
      const groupData = await adminGraphql<{ server: { roomGroups: { id: string }[] } }>(
        adminContext,
        `query { server { roomGroups { id } } }`
      );
      const groupId = groupData.server.roomGroups[0]?.id;
      if (!groupId) {
        throw new Error('No room group available for e2e room creation');
      }

      const createData = await adminGraphql<{ createRoom: { id: string; name: string } }>(
        adminContext,
        `mutation($input: CreateRoomInput!) { createRoom(input: $input) { id name } }`,
        { input: { name: roomName, description: description || undefined, groupId } }
      );
      roomId = createData.createRoom.id;
    } finally {
      await adminContext.dispose();
    }

    // Join as the currently tested user. Room creation itself uses the admin
    // context because ordinary E2E users no longer have room.create by default.
    await graphqlQuery<{ joinRoom: { id: string } }>(
      this.page,
      `mutation($input: JoinRoomInput!) { joinRoom(input: $input) { id } }`,
      { input: { roomId } }
    );

    // Navigate to the new room
    await this.page.goto(routes.room(roomId));

    // Wait for room UI to be fully loaded (header and message input)
    await expect(this.getRoomHeader(roomName)).toBeVisible({ timeout: 5000 });
    await expect(this.page.getByTestId('message-input')).toBeVisible({ timeout: 5000 });

    return roomName;
  }

  // --- Room Creation Assertions ---

  /**
   * Assert that the room creation submit button is disabled.
   */
  async expectRoomSubmitDisabled(): Promise<void> {
    await expect(this.roomFormSubmitButton).toBeDisabled();
  }

  /**
   * Assert that a validation error message is visible.
   */
  async expectValidationError(errorText: string): Promise<void> {
    await expect(this.page.getByText(errorText)).toBeVisible();
  }

  /**
   * Assert that the room header is visible (verifies navigation to room).
   */
  async expectRoomHeaderVisible(roomName: string): Promise<void> {
    await expect(this.getRoomHeader(roomName)).toBeVisible();
  }
}
