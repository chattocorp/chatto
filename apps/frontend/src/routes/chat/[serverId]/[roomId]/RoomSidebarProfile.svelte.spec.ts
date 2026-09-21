import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import {
  __resetUserSummaryCachesForTests,
  getUserSummaryCache
} from '$lib/state/userSummaries.svelte';
import RoomSidebarProfile from './RoomSidebarProfile.svelte';

const mocks = vi.hoisted(() => ({
  queryState: {
    data: undefined as Record<string, unknown> | null | undefined,
    isPending: false
  },
  queryOptions: null as null | { queryFn: () => Promise<unknown> },
  batchGetUsers: vi.fn(),
  viewerSettings: {
    timezone: 'Europe/Berlin',
    timeFormat: 0
  }
}));

vi.mock('@tanstack/svelte-query', () => ({
  createQuery: (options: () => { queryFn: () => Promise<unknown> }) => {
    mocks.queryOptions = options();
    return {
      get data() {
        return mocks.queryState.data;
      },
      get isPending() {
        return mocks.queryState.isPending;
      }
    };
  }
}));

vi.mock('$lib/query/client', () => ({ queryClient: {} }));
vi.mock('$lib/api-client/users', () => ({ createUserAPI: vi.fn() }));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    connection: {
      queryScope: 'session-1',
      getAPI: () => ({ batchGetUsers: mocks.batchGetUsers })
    },
    store: { currentUser: { user: { settings: mocks.viewerSettings } } },
    isCurrent: () => true
  })
}));
vi.mock('$lib/state/userProfiles.svelte', () => ({
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
    __resetUserSummaryCachesForTests();
    mocks.queryState = { data: undefined, isPending: false };
    mocks.queryOptions = null;
    mocks.viewerSettings.timeFormat = TimeFormat.TIME_FORMAT_24_HOUR;
    mocks.batchGetUsers.mockResolvedValue([user]);
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2025-04-27T14:30:00Z'));
  });

  afterEach(() => {
    currentRender?.unmount();
    currentRender = null;
    vi.restoreAllMocks();
  });

  it('renders a cached profile while the fresh query is pending', () => {
    getUserSummaryCache('origin', 'session-1').prime([user]);
    mocks.queryState.isPending = true;

    const { container } = renderProfile();

    expect(container.textContent).toContain('Alice Example');
    expect(container.textContent).toContain('@alice');
    expect(container.textContent).toContain('I build chat software.');
    expect(container.textContent).toContain('16:30');
    expect(container.textContent).not.toContain('Loading');
  });

  it('collapses and expands the bio section', async () => {
    getUserSummaryCache('origin', 'session-1').prime([user]);
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
    getUserSummaryCache('origin', 'session-1').prime([{ ...user, bio: null }]);
    const { container } = renderProfile();
    expect(q(container, '[data-testid="profile-bio-heading"]')).toBeNull();
  });

  it('shows loading while an uncached profile is loading', () => {
    mocks.queryState.isPending = true;

    const { container } = renderProfile();

    expect(container.textContent).toContain('Loading');
    expect(q(container, '[aria-busy="true"]')).toBeTruthy();
  });

  it('uses the viewer preferred 12-hour time format', () => {
    getUserSummaryCache('origin', 'session-1').prime([user]);
    mocks.queryState.isPending = true;
    mocks.viewerSettings.timeFormat = TimeFormat.TIME_FORMAT_12_HOUR;

    const { container } = renderProfile();

    expect(container.textContent).toMatch(/04:30\s*pm/i);
  });

  it('shows the not-found state when the query returns no user', async () => {
    mocks.queryState = { data: null, isPending: false };
    mocks.batchGetUsers.mockResolvedValue([]);

    const { container } = renderProfile();

    await vi.waitFor(() => {
      expect(q(container, '[data-tone="danger"]') ?? container).toHaveTextContent('User not found');
    });
  });

  it('uses shared profile updates and deletion instead of retained query data', async () => {
    const profiles = getUserSummaryCache('origin', 'session-1');
    profiles.prime([user]);
    mocks.queryState.data = user;
    const { container } = renderProfile();
    profiles.prime([{ ...user, displayName: 'Updated profile' }]);
    await expect.element(container).toHaveTextContent('Updated profile');
    profiles.store.delete(user.id);
    await expect.element(container).toHaveTextContent('User not found');
    expect(container.textContent).not.toContain('Alice Example');
  });
});
