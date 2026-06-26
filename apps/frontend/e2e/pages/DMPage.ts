import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';
import { RoomPage } from './RoomPage';

/**
 * Page object for Direct Messages interactions.
 *
 * Per #330 phase 3, DMs are rooms on the Server: they appear in the
 * primary-server sidebar alongside channels and use the channel URL shape
 * (/chat/{instanceSegment}/{roomId}). This helper still reaches the DM
 * room directly via the API for setup convenience, and offers
 * sidebar-scoped assertions for tests that care about list rendering.
 */
export class DMPage {
  constructor(readonly page: Page) {}

  // --- Navigation ---

  /**
   * Navigate to the chat root so that the primary-server sidebar (which
   * contains DMs) is visible.
   */
  async goto(): Promise<void> {
    await this.page.goto(routes.chat);
    await this.page.waitForURL(routes.chat);
  }

  // --- API Actions ---

  /**
   * Start a DM conversation with a user via the ConnectRPC API and navigate
   * to the resulting room (using the channel URL shape).
   */
  async startConversation(username: string): Promise<RoomPage> {
    // Look up user by login
    const userResult = await this.page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `query FindUserByLogin($login: String!) { userByLogin(login: $login) { id } }`,
        variables: { login: username }
      }
    });
    const userData = await userResult.json();
    const userId = userData.data?.userByLogin?.id;
    if (!userId) {
      throw new Error(`User not found: ${username}`);
    }

    // Start DM
    const dmResult = await this.page.request.post(
      '/api/connect/chatto.api.v1.RoomService/StartDM',
      {
        headers: { 'Content-Type': 'application/json', 'Connect-Protocol-Version': '1' },
        data: { participantIds: [userId] }
      }
    );
    if (!dmResult.ok()) {
      throw new Error(`Failed to start DM with ${username}: ${await dmResult.text()}`);
    }
    const dmData = await dmResult.json();
    const conversationId = dmData.room?.id;
    if (!conversationId) {
      throw new Error(`Failed to start DM with ${username}`);
    }

    // Navigate to the conversation (channel URL shape)
    await this.page.goto(routes.room(conversationId));
    await this.page.waitForURL(routes.patterns.anyRoom);

    const roomPage = new RoomPage(this.page);
    await expect(roomPage.messageInput).toBeVisible({ timeout: 5000 });
    return roomPage;
  }

  // --- Conversation List (in primary-server sidebar) ---

  /**
   * Get a DM sidebar item by the other user's display name.
   * Scoped to the sidebar nav to avoid matching the room header.
   * Uses filter with exact text matching to avoid partial matches with similar names.
   */
  getConversation(displayName: string): Locator {
    return this.page
      .locator('nav a.sidebar-item')
      .filter({ has: this.page.getByText(displayName, { exact: true }) });
  }

  /**
   * Click on a DM in the sidebar to open it.
   * Returns a RoomPage for interacting with messages.
   */
  async openConversation(displayName: string): Promise<RoomPage> {
    await this.getConversation(displayName).click();
    await this.page.waitForURL(routes.patterns.anyRoom);
    const roomPage = new RoomPage(this.page);
    await expect(roomPage.messageInput).toBeVisible({ timeout: 5000 });
    return roomPage;
  }

  // --- Assertions ---

  async expectConversationVisible(displayName: string): Promise<void> {
    await expect(this.getConversation(displayName)).toBeVisible();
  }

  async expectConversationNotVisible(displayName: string): Promise<void> {
    await expect(this.getConversation(displayName)).not.toBeVisible();
  }

  async expectConversationHeader(displayName: string): Promise<void> {
    await expect(this.page.getByRole('heading', { name: displayName })).toBeVisible();
  }

  async expectConversationUnread(displayName: string): Promise<void> {
    const conv = this.getConversation(displayName);
    await expect(conv.locator('[data-testid="dm-unread-dot"]')).toBeVisible();
  }

  async expectConversationRead(displayName: string): Promise<void> {
    const conv = this.getConversation(displayName);
    await expect(conv.locator('[data-testid="dm-unread-dot"]')).not.toBeVisible();
  }
}
