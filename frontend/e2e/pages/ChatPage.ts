import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';
import { graphqlQuery } from '../fixtures/graphqlHelpers';
import { loginAsAdmin } from '../fixtures/testUser';
import { RoomPage } from './RoomPage';

/**
 * Page object for the main chat interface.
 * Handles sidebar navigation, space creation, and room entry.
 */
export class ChatPage {
  constructor(readonly page: Page) {}

  /** The explore spaces link in the sidebar */
  get exploreSpacesLink(): Locator {
    return this.page.getByRole('link', { name: 'Explore Spaces' });
  }

  /** The room list container in the sidebar */
  get roomList(): Locator {
    return this.page.locator('.room-list');
  }

  /**
   * Navigate to the chat page.
   * Note: Users may be redirected based on their state:
   * - New users (no spaces): redirected to /chat/spaces
   * - Users with last space: redirected to /chat/-/[spaceId]/[roomId]
   */
  async goto(): Promise<void> {
    await this.page.goto('/chat');
    // Wait for any /chat path - redirects happen based on user state
    await this.page.waitForURL((url) => url.pathname.startsWith('/chat'));

  }

	/**
	 * Return the kind-discriminator constant used as a spaceID by core methods.
	 * Post-ADR-030 every channel-scoped call uses this single deployment-wide
	 * value (`core.LegacyServerSpaceID = "server"` on the backend).
	 */
	async getSpaceId(): Promise<string> {
    return 'server';
  }

  /**
   * Wait until the user is in the deployment's bootstrap server and return
   * the server name. Post-ADR-030 there is no per-deployment Space record;
   * signup auto-joins the default rooms and this just confirms the server
   * config is reachable. `name` and `description` args are ignored,
   * retained only so existing call sites compile.
   */
  async createSpace(_name?: string, _description?: string): Promise<string> {
    const data = await graphqlQuery<{
      server: { config: { serverName: string } } | null;
    }>(
      this.page,
      `query { server { config { serverName } } }`
    );
    if (!data.server) {
      throw new Error('Server query returned no data — bootstrap config likely broken');
    }
    return data.server.config.serverName;
  }

  /**
   * Enter a room by clicking it in the sidebar.
   * Returns a RoomPage for interacting with messages.
   * If already in the room (e.g., after createSpace redirect), skips navigation
   * to avoid disrupting WebSocket subscriptions.
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

  // --- Space Icon Indicators ---

  /**
   * Get the container div for a space icon by space name.
   * Scopes to the specific space in the sidebar (the parent div wrapping the button and any dots).
   */
  getSpaceIconContainer(spaceName: string): Locator {
    return this.page
      .locator('.server-gutter')
      .locator('div', { has: this.page.getByRole('link', { name: spaceName, exact: true }) });
  }

  /** Get the unread dot locator for a specific space */
  getSpaceUnreadDot(spaceName: string): Locator {
    return this.getSpaceIconContainer(spaceName).getByTestId('space-unread-dot');
  }

  /** Click the unread dot on a specific space icon */
  async clickSpaceUnreadDot(spaceName: string): Promise<void> {
    await this.getSpaceUnreadDot(spaceName).click();
  }

  /** Assert that a specific space icon shows an unread dot */
  async expectSpaceHasUnread(spaceName: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.getSpaceUnreadDot(spaceName)).toBeVisible(options);
  }

  /** Assert that a specific space icon does NOT show an unread dot */
  async expectSpaceHasNoUnread(spaceName: string, options?: { timeout?: number }): Promise<void> {
    await expect(this.getSpaceUnreadDot(spaceName)).not.toBeVisible(options);
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
   * member of the bootstrap space and the admin route 403s. Logout then
   * re-authenticate as e2eadmin (the bootstrap owner) before navigating so
   * the previous session's permissions don't leak into the page's reactive
   * state.
   */
  async openCreateRoomModal(): Promise<void> {
    await this.page.request.post('/auth/logout');
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
    const groupData = await graphqlQuery<{ server: { roomGroups: { id: string }[] } }>(
      this.page,
      `query { server { roomGroups { id } } }`
    );
    const groupId = groupData.server.roomGroups[0]?.id;
    if (!groupId) {
      throw new Error('No room group available for e2e room creation');
    }

    // Create and join room via API
    const result = await this.page.evaluate(
      async ({ roomName, description, groupId }) => {
        const createRes = await fetch('/api/graphql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
          credentials: 'include',
          body: JSON.stringify({
            query: `mutation($input: CreateRoomInput!) { createRoom(input: $input) { id name } }`,
            variables: { input: { name: roomName, description: description || undefined, groupId } }
          })
        });
        const createData = await createRes.json();
        if (createData.errors) throw new Error(JSON.stringify(createData.errors));
        const roomId = createData.data.createRoom.id;

        // Join the room
        const joinRes = await fetch('/api/graphql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
          credentials: 'include',
          body: JSON.stringify({
            query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) { id } }`,
            variables: { input: { roomId } }
          })
        });
        const joinData = await joinRes.json();
        if (joinData.errors) throw new Error(JSON.stringify(joinData.errors));

        return { roomId };
      },
      { roomName, description, groupId }
    );

    // Navigate to the new room
    await this.page.goto(routes.room(result.roomId));

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

  /**
   * Navigate to the Explore Spaces page.
   */
  async goToExploreSpaces(): Promise<void> {
    // Post-#330 PR(a) the Browse Spaces UI is gone. Kept as a no-op so
    // existing tests compile; ExplorePage.joinSpace navigates to the chat
    // root directly.
  }

  // --- Assertions ---

  /**
   * Assert that the explore spaces button is visible.
   */
  async expectExploreSpacesVisible(): Promise<void> {
    await expect(this.page.locator('[title="Explore Spaces"]')).toBeVisible();
  }
}
