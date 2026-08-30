import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  canCreateBots: false,
  listBots: vi.fn(),
  batchGetUsers: vi.fn()
}));

vi.mock('$lib/api-client/viewer', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/viewer')>();
  return {
    ...actual,
    viewerResponseToState: () => ({
      viewerPermissions: { 'bot.create': mocks.canCreateBots }
    })
  };
});

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'server-1',
    store: {
      serverInfo: { supportsFeature: () => true },
      currentUser: { user: { settings: null } },
      projection: { viewer: {} }
    },
    connection: {
      queryScope: 'session-1',
      getAPI: () => ({
        listBots: mocks.listBots,
        batchGetUsers: mocks.batchGetUsers
      })
    },
    isCurrent: () => true
  })
}));

import BotsPage from './+page.svelte';

function createButton(container: Element): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find((button) =>
    button.textContent?.includes('Create bot')
  );
}

describe('Bot administration page', () => {
  beforeEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.canCreateBots = false;
    mocks.listBots.mockResolvedValue({ bots: [], totalCount: 0, hasMore: false });
    mocks.batchGetUsers.mockResolvedValue([]);
  });

  it('explains why creation is unavailable while preserving the bot-management page', () => {
    const { container } = render(BotsPage);

    expect(createButton(container)).toBeUndefined();
    expect(container.textContent).toContain(
      'You can manage existing bots, but you do not have permission to create one.'
    );
    expect(container.textContent).toContain('Bot Accounts');
  });

  it('offers creation when the viewer has bot.create', () => {
    mocks.canCreateBots = true;
    const { container } = render(BotsPage);

    expect(createButton(container)).toBeDefined();
    expect(container.textContent).not.toContain(
      'You can manage existing bots, but you do not have permission to create one.'
    );
  });

  it('does not ask for the initial API key name', () => {
    mocks.canCreateBots = true;
    const { container } = render(BotsPage);

    createButton(container)?.click();

    expect(container.querySelector('#bot-api-key-name')).toBeNull();
    expect(container.textContent).not.toContain('Key name');
  });

  it('renders bot and owner identities with avatars and display names', async () => {
    mocks.listBots.mockResolvedValue({
      bots: [
        {
          id: 'bot-user-id',
          login: 'helper_bot',
          displayName: 'Helper Bot',
          avatarUrl: null,
          bio: 'Build helper',
          timezone: null,
          ownerUserId: 'owner-user-id',
          createdAt: null,
          apiKeyCreatedAt: null,
          apiKeys: [],
          incomingWebhooks: []
        }
      ],
      totalCount: 1,
      hasMore: false
    });
    mocks.batchGetUsers.mockResolvedValue([
      {
        id: 'owner-user-id',
        login: 'alice',
        displayName: 'Alice Example',
        deleted: false,
        avatarUrl: null
      }
    ]);

    const { container } = render(BotsPage);
    await vi.waitFor(() => {
      expect(container.textContent).toContain('Alice Example');
    });

    expect(mocks.batchGetUsers).toHaveBeenCalledWith(['owner-user-id']);
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('Helper Bot');
    expect(container.textContent).not.toContain('owner-user-id');
    expect(container.querySelectorAll('[data-testid="user-identity"]')).toHaveLength(2);
    expect(container.querySelector('[data-testid="bot-badge"]')).not.toBeNull();
    expect(container.querySelector('a[href$="/manage/server/bots/bot-user-id"]')).not.toBeNull();
  });
});
