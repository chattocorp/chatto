import { beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

// Server handlers: the page runs the real bot and user API clients against them.
const bots = mockService(BotService);
const users = mockService(UserService);
let server: TestServerScope;

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
    server = createTestServerScope({
      routes: (router) => router.service(BotService, bots).service(UserService, users)
    });
    bots.listBots.mockReturnValue({ bots: [], page: { totalCount: 0n, hasMore: false } });
    users.batchGetUsers.mockReturnValue({ users: [] });
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
    server.permissions.canCreateBots = true;
    const { container } = render(BotsPage);

    expect(createButton(container)).toBeDefined();
    expect(container.textContent).not.toContain(
      'You can manage existing bots, but you do not have permission to create one.'
    );
  });

  it('does not ask for the initial API key name', () => {
    server.permissions.canCreateBots = true;
    const { container } = render(BotsPage);

    createButton(container)?.click();

    expect(container.querySelector('#bot-api-key-name')).toBeNull();
    expect(container.textContent).not.toContain('Key name');
  });

  it('accepts a bot username without a suffix', async () => {
    server.permissions.canCreateBots = true;
    const { container } = render(BotsPage);

    await userEvent.click(createButton(container)!);
    await vi.waitFor(() => expect(document.querySelector('#bot-login')).not.toBeNull());
    const login = document.querySelector<HTMLInputElement>('#bot-login')!;
    const displayName = document.querySelector<HTMLInputElement>('#bot-display-name')!;
    const dialog = login.closest('dialog')!;
    await userEvent.fill(login, 'helper');
    await userEvent.fill(displayName, 'Helper');

    expect(dialog.textContent).not.toContain('must end in _bot');
    expect(login.getAttribute('aria-invalid')).not.toBe('true');
    const submit = Array.from(dialog.querySelectorAll<HTMLButtonElement>('button')).find(
      (button) => button.textContent?.trim() === 'Create bot'
    );
    expect(submit?.disabled).toBe(false);
  });

  it('renders bot and owner identities with avatars and display names', async () => {
    bots.listBots.mockReturnValue({
      bots: [
        {
          user: {
            id: 'bot-user-id',
            login: 'helper_bot',
            displayName: 'Helper Bot',
            bio: 'Build helper',
            bot: { ownerUserId: 'owner-user-id' }
          },
          ownerUserId: 'owner-user-id'
        }
      ],
      page: { totalCount: 1n, hasMore: false }
    });
    users.batchGetUsers.mockReturnValue({
      users: [{ user: { id: 'owner-user-id', login: 'alice', displayName: 'Alice Example' } }]
    });

    const { container } = render(BotsPage);
    await vi.waitFor(() => {
      expect(container.textContent).toContain('Alice Example');
    });

    expect(users.batchGetUsers.mock.calls[0]?.[0]).toMatchObject({ userIds: ['owner-user-id'] });
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('Helper Bot');
    expect(container.textContent).not.toContain('owner-user-id');
    expect(container.querySelectorAll('[data-testid="user-identity"]')).toHaveLength(2);
    expect(container.querySelector('[data-testid="bot-badge"]')).not.toBeNull();
    expect(container.querySelector('a[href$="/manage/server/bots/bot-user-id"]')).not.toBeNull();
  });
});
