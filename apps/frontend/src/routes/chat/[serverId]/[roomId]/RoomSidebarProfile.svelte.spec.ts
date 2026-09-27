import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { queryClient, removeServerQueries } from '$lib/query/client';
import { q } from '$lib/test-utils';
import { getUserStore, resetUserStoresForTests } from '$lib/state/server/users.svelte';
import { userProfileFixture } from '$lib/test-utils/userProfile';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import RoomSidebarProfile from './RoomSidebarProfile.svelte';

const batchGetUsers = vi.fn();
let server: TestServerScope;
/** The shared profile store of the fixture's server session. */
const userStore = () => getUserStore('origin', server.scope.connection.queryScope);

vi.mock('$lib/api-client/users', () => ({ createUserAPI: vi.fn() }));
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

function renderProfile() {
  currentRender = render(RoomSidebarProfile, { props: { userId: 'user-1' } });
  return currentRender;
}

describe('RoomSidebarProfile', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetUserStoresForTests();
    server = createTestServerScope({
      serverId: 'origin',
      api: { batchGetUsers },
      viewer: {
        settings: { timezone: 'Europe/Berlin', timeFormat: TimeFormat.TIME_FORMAT_24_HOUR }
      },
      store: {
        get projection() {
          return { users: userStore() };
        }
      }
    });
    batchGetUsers.mockResolvedValue([user]);
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2025-04-27T14:30:00Z'));
  });

  afterEach(() => {
    currentRender?.unmount();
    currentRender = null;
    vi.restoreAllMocks();
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

  it('collapses and expands the bio section', async () => {
    userStore().set(user.id, userProfileFixture(user));
    const { container } = renderProfile();
    await expect.poll(() => q(container, '[data-testid="profile-bio-heading"]')).not.toBeNull();
    const heading = q(container, '[data-testid="profile-bio-heading"]');

    await expect.element(heading).toHaveAttribute('aria-expanded', 'true');
    await expect.poll(() => q(container, '[data-testid="user-bio"]')).not.toBeNull();
    heading?.click();
    await expect.element(heading).toHaveAttribute('aria-expanded', 'false');
    expect(q(container, '[data-testid="user-bio"]')?.closest('[inert]')).not.toBeNull();

    heading?.click();
    await expect.element(heading).toHaveAttribute('aria-expanded', 'true');
    await expect.poll(() => q(container, '[data-testid="user-bio"]')).not.toBeNull();
  });

  it('omits the bio section when no bio is set', () => {
    userStore().set(user.id, userProfileFixture({ ...user, bio: null }));
    const { container } = renderProfile();
    expect(q(container, '[data-testid="profile-bio-heading"]')).toBeNull();
  });

  it('shows loading while an uncached profile is loading', () => {
    batchGetUsers.mockReturnValue(new Promise(() => {}));

    const { container } = renderProfile();

    expect(q(container, '[data-loading-fog][aria-busy="true"]')).toBeTruthy();
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
});
