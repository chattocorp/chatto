import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { q } from '$lib/test-utils';
import {
  __resetUserSummaryCachesForTests,
  getUserSummaryCache
} from '$lib/state/userSummaries.svelte';

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

vi.mock('$app/state', () => ({
  page: { params: { serverId: 'origin', userId: 'user-1' } }
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
  getLiveBio: (_userId: string, fallback: string | null) => fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveLogin: (_userId: string, fallback: string) => fallback,
  getLiveTimezone: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: () => null
}));

vi.mock('$lib/components/UserAvatar.svelte', async () => ({
  default: (await import('../../../ChatRootTestStub.svelte')).default
}));

vi.mock('$lib/components/UserCustomStatusBadge.svelte', async () => ({
  default: (await import('../../../ChatRootTestStub.svelte')).default
}));

import UserProfilePage from './+page.svelte';

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

function renderPage() {
  currentRender = render(UserProfilePage);
  return currentRender;
}

describe('User profile page', () => {
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
    getUserSummaryCache('origin').prime([user]);
    mocks.queryState.isPending = true;

    const { container } = renderPage();

    expect(container.textContent).toContain('Alice Example');
    expect(container.textContent).toContain('@alice');
    expect(container.textContent).toContain('I build chat software.');
    expect(container.textContent).toContain('16:30');
    expect(container.textContent).not.toContain('Loading');
  });

  it('uses the viewer preferred 12-hour time format', () => {
    getUserSummaryCache('origin').prime([user]);
    mocks.queryState.isPending = true;
    mocks.viewerSettings.timeFormat = TimeFormat.TIME_FORMAT_12_HOUR;

    const { container } = renderPage();

    expect(container.textContent).toMatch(/04:30\s*pm/i);
  });

  it('shows the not-found state when the query returns no user', async () => {
    mocks.queryState = { data: null, isPending: false };
    mocks.batchGetUsers.mockResolvedValue([]);

    const { container } = renderPage();

    await vi.waitFor(() => {
      expect(q(container, '[data-tone="danger"]') ?? container).toHaveTextContent('User not found');
    });
  });

  it('primes the summary cache only from a successful query', async () => {
    const prime = vi.spyOn(getUserSummaryCache('origin'), 'prime');
    renderPage();

    expect(prime).not.toHaveBeenCalled();
    await mocks.queryOptions?.queryFn();

    expect(mocks.batchGetUsers).toHaveBeenCalledWith(['user-1']);
    expect(prime).toHaveBeenCalledExactlyOnceWith([user]);
  });
});
