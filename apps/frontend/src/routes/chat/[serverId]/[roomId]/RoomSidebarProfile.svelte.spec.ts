import { RoomKind } from '@chatto/client/api/roomDirectory';
import type { UserProfileDetails, VerifiedIdentity } from '@chatto/client/api/users';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { queryClient, removeServerQueries } from '$lib/query/client';
import { q } from '$lib/test-utils';
import { getUserStore, resetUserStoresForTests } from '@chatto/client/server/users';
import { userProfileFixture } from '@chatto/client/testing/userProfile';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import RoomSidebarProfile from './RoomSidebarProfile.svelte';

const batchGetUsers = vi.fn();
const getVerifiedIdentities = vi.fn<() => Promise<VerifiedIdentity[]>>();
/** The shared profile-details read, built from the identity mock. */
const getProfileDetails = vi.fn(async (): Promise<UserProfileDetails> => ({
  roles: [],
  createdAt: null,
  verifiedIdentities: await getVerifiedIdentities()
}));
const searchMessages = vi.fn(async () => ({ results: [] as unknown[], nextCursor: null }));
const searchAvailability = { available: false };
let server: TestServerScope;
const gotoMock = vi.hoisted(() => vi.fn());
vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: gotoMock
}));
/** The shared profile store of the fixture's server session. */
const userStore = () => getUserStore('origin', server.scope.connection.queryScope);

// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('@chatto/client/api/users', () => ({ createUserAPI: vi.fn() }));
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBotOwnerUserId: (_userId: string, fallback: string | null) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveBio: (_userId: string, fallback: string | null) => fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveLogin: (_userId: string, fallback: string) => fallback,
  getLiveTimezone: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: () => null
}));
vi.mock('$lib/components/UserAvatar.svelte', async () => ({
  default: (await import('../../ChatRootTestStub.svelte')).default
}));
vi.mock('$lib/components/UserCustomStatusBadge.svelte', async () => ({
  default: (await import('../../ChatRootTestStub.svelte')).default
}));
vi.mock('$lib/components/bots/BotOwnerRow.svelte', async () => ({
  default: (await import('../../ChatRootTestStub.svelte')).default
}));
vi.mock('$lib/components/bots/BotPermissionSummary.svelte', async () => ({
  default: (await import('../../ChatRootTestStub.svelte')).default
}));

const bot = {
  id: 'bot-1',
  login: 'helper_bot',
  displayName: 'Helper Bot',
  deleted: false,
  isBot: true,
  bot: { ownerUserId: 'viewer-1' },
  avatarUrl: null,
  bio: null,
  timezone: null,
  presenceStatus: PresenceStatus.OFFLINE
};

const user = {
  id: 'user-1',
  login: 'alice',
  displayName: 'Alice Example',
  deleted: false,
  isBot: false,
  avatarUrl: null,
  bio: 'I build chat software.',
  timezone: 'Europe/Berlin',
  presenceStatus: PresenceStatus.OFFLINE
};

let currentRender: ReturnType<typeof render> | null = null;

function renderProfile(
  props: {
    userId?: string;
    onSendMessage?: (userId: string) => void;
    onMention?: (login: string) => void;
  } = {}
) {
  currentRender = render(RoomSidebarProfile, { props: { userId: 'user-1', ...props } });
  return currentRender;
}

/** Open the shared user menu from the profile card's menu button. */
async function openProfileMenu(container: HTMLElement): Promise<HTMLElement> {
  const button = q(container, '[data-testid="profile-user-menu-button"]');
  await expect.element(button).toBeInTheDocument();
  button!.click();
  const dialog = page.getByRole('dialog', { name: 'User profile' });
  await expect.element(dialog).toBeVisible();
  return dialog.element() as HTMLElement;
}

describe('RoomSidebarProfile', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    searchAvailability.available = false;
    resetUserStoresForTests();
    server = createTestServerScope({
      serverId: 'origin',
      api: { batchGetUsers, getProfileDetails, searchMessages },
      viewer: {
        settings: { timezone: 'Europe/Berlin', timeFormat: TimeFormat.TIME_FORMAT_24_HOUR }
      },
      store: {
        get projection() {
          return { users: userStore() };
        },
        presence: { get: () => undefined },
        messageSearch: { availability: searchAvailability },
        mentionRoles: {
          roles: [
            { name: 'admin', displayName: 'Admin' },
            { name: 'moderator', displayName: 'Moderator' }
          ],
          load: async () => true
        }
      }
    });
    getVerifiedIdentities.mockResolvedValue([
      { method: 'atproto', methodLabel: 'AT Protocol', value: 'alice.example' }
    ]);
    batchGetUsers.mockResolvedValue([user]);
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2025-04-27T14:30:00Z'));
  });

  afterEach(() => {
    currentRender?.unmount();
    currentRender = null;
    vi.restoreAllMocks();
  });

  it('shows the member date and roles in catalogue order', async () => {
    getProfileDetails.mockResolvedValueOnce({
      roles: ['moderator', 'custom', 'admin'],
      createdAt: '2025-03-14T12:00:00Z',
      verifiedIdentities: []
    });
    userStore().set(user.id, userProfileFixture(user));
    const { container } = renderProfile();

    const roles = page.getByRole('list', { name: 'Roles' });
    await expect.element(roles).toBeVisible();
    expect(
      Array.from(roles.element().querySelectorAll('li')).map((role) => role.textContent?.trim())
    ).toEqual(['Admin', 'Moderator', 'custom']);
    // The date follows the browser's formatting locale.
    expect(q(container, '[data-testid="profile-member-since"]')?.textContent).toMatch(
      /Member since .*2025/
    );
    expect(q(container, '[data-testid="profile-local-time"]')?.textContent).toContain(
      'Same time as you'
    );
  });

  it('offers Mention only with a host composer and passes the login', async () => {
    userStore().set(user.id, userProfileFixture(user));
    const withoutHost = renderProfile();
    await expect.element(page.getByTestId('profile-quick-actions')).toBeVisible();
    expect(q(withoutHost.container, '[data-testid="profile-action-mention"]')).toBeNull();
    withoutHost.unmount();

    const onMention = vi.fn();
    renderProfile({ onMention });
    await page.getByTestId('profile-action-mention').click();
    expect(onMention).toHaveBeenCalledWith('alice');
  });

  it('lists the newest visible messages and opens one', async () => {
    searchAvailability.available = true;
    searchMessages.mockResolvedValueOnce({
      results: [
        {
          id: 'msg-1',
          roomId: 'room-1',
          roomName: 'general',
          roomKind: RoomKind.CHANNEL,
          actorId: user.id,
          actor: null,
          body: 'Shipping the **profile** redesign',
          createdAt: '2025-04-27T14:00:00Z',
          threadRootEventId: null,
          attachmentCount: 0,
          relevanceScore: 0
        }
      ],
      nextCursor: null
    });
    userStore().set(user.id, userProfileFixture(user));
    renderProfile();

    const row = page.getByRole('link', { name: /#general/ });
    await expect.element(row).toBeVisible();
    await expect.element(row).toHaveTextContent('Shipping the profile redesign');
    expect(searchMessages).toHaveBeenCalledWith(
      expect.objectContaining({ query: 'from:alice', authorId: 'user-1', pageSize: 3 })
    );
    await row.click();
    expect(gotoMock).toHaveBeenCalledWith(expect.stringContaining('room-1'));
  });

  it('omits recent messages while search is unavailable or empty', async () => {
    userStore().set(user.id, userProfileFixture(user));
    const unavailable = renderProfile();
    await expect.element(page.getByTestId('profile-user-card')).toBeVisible();
    expect(q(unavailable.container, '[data-testid="profile-recent-messages"]')).toBeNull();
    expect(searchMessages).not.toHaveBeenCalled();
    unavailable.unmount();

    searchAvailability.available = true;
    const empty = renderProfile();
    await expect.poll(() => searchMessages.mock.calls.length).toBe(1);
    await expect
      .poll(() => q(empty.container, '[data-testid="profile-recent-messages"]'))
      .toBeNull();
  });

  it('shows known and unfamiliar verified identities in the full profile and its popup', async () => {
    getVerifiedIdentities.mockResolvedValue([
      { method: 'atproto', methodLabel: 'AT Protocol', value: 'alice.example' },
      { method: 'custom-directory', methodLabel: 'Community directory', value: 'Member 123' }
    ]);
    userStore().set(user.id, userProfileFixture(user));
    const { container } = renderProfile();
    await expect.element(page.getByText('alice.example', { exact: true })).toBeVisible();
    await expect.element(page.getByTitle('Community directory · Member 123')).toBeVisible();
    // The full profile lists them in its details card.
    expect(
      q(container, '[data-testid="profile-details-section"] ul[aria-label="Verified identities"]')
    ).not.toBeNull();
    const menu = await openProfileMenu(container);
    await expect.poll(() => menu.textContent).toContain('alice.example');
    const section = menu.querySelector('ul[aria-label="Verified identities"]')!;
    expect(menu.textContent).not.toContain('Verified identities');
    expect(section.textContent).toContain('alice.example');
    expect(section.textContent).toContain('Member 123');
    expect(
      section.compareDocumentPosition(menu.querySelector('[data-testid="user-bio"]')!) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });

  it('omits the identities section in both views after an empty lookup', async () => {
    getVerifiedIdentities.mockResolvedValue([]);
    userStore().set(user.id, userProfileFixture(user));
    const { container } = renderProfile();
    await expect
      .poll(() =>
        queryClient.getQueryData([
          'server',
          'origin',
          'session',
          server.scope.connection.queryScope,
          'user',
          user.id,
          'profile-details'
        ])
      )
      .toEqual({ roles: [], createdAt: null, verifiedIdentities: [] });
    expect(container.textContent).not.toContain('Verified identities');
    const menu = await openProfileMenu(container);
    expect(menu.querySelector('ul[aria-label="Verified identities"]')).toBeNull();
  });

  it('renders a cached profile while the fresh query is pending', () => {
    userStore().set(user.id, userProfileFixture(user));
    batchGetUsers.mockReturnValue(new Promise(() => {}));

    const { container } = renderProfile();

    expect(container.textContent).toContain('Alice Example');
    expect(container.textContent).toContain('@alice');
    expect(container.textContent).toContain('I build chat software.');
    expect(container.textContent).toContain('16:30');
    expect(container.textContent).not.toContain('Loading');
  });

  it('shows the bio in its own section', async () => {
    userStore().set(user.id, userProfileFixture(user));
    const { container } = renderProfile();
    await expect.poll(() => q(container, '[data-testid="profile-bio"]')).not.toBeNull();
    const section = q(container, '[data-testid="profile-bio"]')!;
    expect(section.querySelector('h3')?.textContent).toBe('About');
    expect(section.querySelector('[data-testid="user-bio"]')?.textContent).toContain(
      'I build chat software.'
    );
  });

  it('omits the bio section when no bio is set', () => {
    userStore().set(user.id, userProfileFixture({ ...user, bio: null }));
    const { container } = renderProfile();
    expect(q(container, '[data-testid="profile-bio"]')).toBeNull();
  });

  it('shows loading while an uncached profile is loading', () => {
    batchGetUsers.mockReturnValue(new Promise(() => {}));

    const { container } = renderProfile();

    expect(q(container, '[data-loading-dots][aria-busy="true"]')).toBeTruthy();
    expect(container.textContent).not.toContain('Loading');
  });

  it('uses the viewer preferred 12-hour time format', () => {
    userStore().set(user.id, userProfileFixture(user));
    server.currentUser.user!.settings!.timeFormat = TimeFormat.TIME_FORMAT_12_HOUR;

    const { container } = renderProfile();

    expect(container.textContent).toMatch(/04:30\s*pm/i);
  });

  it('purges the profile read with the server session cache', async () => {
    renderProfile();
    const profileReads = () =>
      queryClient
        .getQueryCache()
        .findAll()
        .filter((query) => query.queryKey.includes(user.id));
    await vi.waitFor(() => expect(profileReads()[0]?.state.status).toBe('success'));

    removeServerQueries('origin');

    expect(profileReads()).toHaveLength(0);
  });

  it('shows the not-found state when the query returns no user', async () => {
    batchGetUsers.mockResolvedValue([]);

    const { container } = renderProfile();

    await vi.waitFor(() => {
      expect(q(container, '[data-tone="danger"]') ?? container).toHaveTextContent('User not found');
    });
  });

  it('uses shared profile updates and deletion instead of retained query data', async () => {
    const profiles = userStore();
    profiles.set(user.id, userProfileFixture(user));
    const { container } = renderProfile();
    profiles.set(user.id, userProfileFixture({ ...user, displayName: 'Updated profile' }));
    await expect.element(container).toHaveTextContent('Updated profile');
    profiles.delete(user.id);
    await expect.element(container).toHaveTextContent('User not found');
    expect(container.textContent).not.toContain('Alice Example');
  });

  it('renders the header as the shared user card', async () => {
    userStore().set(user.id, userProfileFixture(user));
    const { container } = renderProfile();

    const card = q(container, '[data-testid="profile-user-card"]');
    await expect.element(card).toHaveTextContent('Alice Example');
    await expect.element(card).toHaveTextContent('@alice');
    await expect
      .element(page.getByRole('heading', { level: 2, name: 'Alice Example' }))
      .toBeInTheDocument();
    await expect
      .element(q(container, '[data-testid="profile-user-menu-button"]'))
      .toHaveAccessibleName('Open profile card of Alice Example');
  });

  it('opens the shared user menu on right-click of the card', async () => {
    userStore().set(user.id, userProfileFixture(user));
    const { container } = renderProfile();

    const card = q(container, '[data-testid="profile-user-card"]');
    await expect.element(card).toBeInTheDocument();
    card!.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));

    await expect.element(page.getByRole('dialog', { name: 'User profile' })).toBeVisible();
  });

  it('opens the shared user menu without a view-profile action', async () => {
    userStore().set(user.id, userProfileFixture(user));
    const onSendMessage = vi.fn();
    const { container } = renderProfile({ onSendMessage });

    const dialog = await openProfileMenu(container);

    expect(dialog.textContent).not.toContain('View profile');
    expect(dialog.textContent).not.toContain('Manage bot');
    const sendMessage = Array.from(dialog.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open conversation'
    );
    sendMessage!.click();
    expect(onSendMessage).toHaveBeenCalledWith('user-1');
  });

  it('lets the bot owner open bot management from the profile card menu', async () => {
    userStore().set(bot.id, userProfileFixture(bot));
    const { container } = renderProfile({ userId: bot.id });

    const dialog = await openProfileMenu(container);

    const link = dialog.querySelector('[data-testid="manage-bot"]');
    expect(link?.getAttribute('href')).toMatch(/\/manage\/server\/bots\/bot-1$/);
  });

  it('hides bot management from viewers who do not own or manage the bot', async () => {
    const othersBot = { ...bot, bot: { ownerUserId: 'someone-else' } };
    userStore().set(bot.id, userProfileFixture(othersBot));
    const { container } = renderProfile({ userId: bot.id });

    const dialog = await openProfileMenu(container);

    expect(dialog.querySelector('[data-testid="manage-bot"]')).toBeNull();
  });
});
