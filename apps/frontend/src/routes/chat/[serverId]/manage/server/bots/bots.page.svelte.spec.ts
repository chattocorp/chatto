import { beforeEach, describe, expect, it, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import { getToasts } from '$lib/ui/toast';
import { Code, ConnectError } from '@connectrpc/connect';
import { mockService } from '$lib/test-utils';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { AdminPermissionService } from '@chatto/api-types/admin/v1/permissions_connect';
import {
  PermissionDecision,
  PermissionScopeKind,
  type SetUserPermissionRequest
} from '@chatto/api-types/admin/v1/permissions_pb';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

const navigation = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: navigation.goto
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

// Server handlers: the page runs the real bot and user API clients against them.
const bots = mockService(BotService);
const users = mockService(UserService);
const permissions = mockService(AdminPermissionService);
let server: TestServerScope;

import BotsPage from './+page.svelte';

const HELPER_PERMISSIONS = {
  'message.read': true,
  'message.post': true,
  'message.read-interactions': true,
  'message.post-in-interactions': true
};

/** Opens the dialog, fills it, selects the capabilities, and submits it. */
async function createHelperBot(
  container: Element,
  capabilities: RegExp[],
  beforeSubmit?: () => void
) {
  bots.createBot.mockReturnValue({
    bot: {
      user: { id: 'B1', login: 'helper', displayName: 'Helper', bot: { ownerUserId: 'viewer-1' } },
      ownerUserId: 'viewer-1'
    },
    apiKey: 'secret-key'
  });
  await userEvent.click(createButton(container)!);
  await userEvent.fill(document.querySelector<HTMLInputElement>('#bot-login')!, 'helper');
  await userEvent.fill(document.querySelector<HTMLInputElement>('#bot-display-name')!, 'Helper');
  for (const name of capabilities) {
    await userEvent.click(page.getByRole('checkbox', { name }));
  }
  beforeSubmit?.();
  await userEvent.click(page.getByRole('button', { name: 'Create bot', exact: true }).last());
}

function allowEveryGrant() {
  permissions.setUserPermission.mockImplementation((request: SetUserPermissionRequest) => ({
    decision: {
      permission: request.permission,
      scope: request.scope,
      decision: PermissionDecision.ALLOW
    }
  }));
}

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
      routes: (router) =>
        router
          .service(BotService, bots)
          .service(UserService, users)
          .service(AdminPermissionService, permissions)
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

  it('locks capabilities that the creator does not hold', async () => {
    server.permissions.canCreateBots = true;
    server.permissions.serverScope = { 'message.read': true, 'room.list': true };
    const { container } = render(BotsPage);

    await userEvent.click(createButton(container)!);

    await expect
      .element(page.getByRole('checkbox', { name: 'Read all messages' }))
      .not.toHaveAttribute('aria-disabled', 'true');
    const joinRooms = page.getByRole('checkbox', { name: 'Find and join rooms' });
    await expect.element(joinRooms).toHaveAttribute('aria-disabled', 'true');
    await expect.element(joinRooms).toHaveAccessibleDescription(/You do not have this permission/);
    // Playwright does not click an aria-disabled control, so click its label directly.
    (page.getByText('Find and join rooms').element() as HTMLElement).click();
    await expect.element(joinRooms).not.toBeChecked();
  });

  it('grants the selected capabilities after it creates the bot', async () => {
    server.permissions.canCreateBots = true;
    server.permissions.serverScope = HELPER_PERMISSIONS;
    allowEveryGrant();
    const { container } = render(BotsPage);

    await createHelperBot(container, [/Answer mentions and threads/, /Chat in direct messages/]);

    await expect.element(page.getByText('Save This API Key')).toBeInTheDocument();
    await vi.waitFor(() => expect(permissions.setUserPermission).toHaveBeenCalledTimes(4));
    expect(
      permissions.setUserPermission.mock.calls.map(([request]) => [
        request.userId,
        request.scope?.kind,
        request.permission,
        request.decision
      ])
    ).toEqual([
      ['B1', PermissionScopeKind.SERVER, 'message.read-interactions', PermissionDecision.ALLOW],
      ['B1', PermissionScopeKind.SERVER, 'message.post-in-interactions', PermissionDecision.ALLOW],
      ['B1', PermissionScopeKind.DM, 'message.read', PermissionDecision.ALLOW],
      ['B1', PermissionScopeKind.DM, 'message.post', PermissionDecision.ALLOW]
    ]);

    await userEvent.click(page.getByRole('button', { name: 'Got it' }));
    await vi.waitFor(() =>
      expect(navigation.goto).toHaveBeenCalledWith('/chat/-/manage/server/bots/B1')
    );
  });

  it('skips a capability that became unavailable while the dialog was open', async () => {
    server.permissions.canCreateBots = true;
    server.permissions.serverScope = HELPER_PERMISSIONS;
    allowEveryGrant();
    const { container } = render(BotsPage);

    await createHelperBot(container, [/Answer mentions and threads/, /Read all messages/], () => {
      server.permissions.serverScope = { ...HELPER_PERMISSIONS, 'message.read': false };
    });

    await vi.waitFor(() => expect(permissions.setUserPermission).toHaveBeenCalledTimes(2));
    expect(permissions.setUserPermission.mock.calls.map(([request]) => request.permission)).toEqual(
      ['message.read-interactions', 'message.post-in-interactions']
    );
  });

  it('warns and opens the Permissions tab when a grant fails', async () => {
    server.permissions.canCreateBots = true;
    server.permissions.serverScope = HELPER_PERMISSIONS;
    permissions.setUserPermission.mockImplementation(() => {
      throw new ConnectError('owner ceiling', Code.PermissionDenied);
    });
    const { container } = render(BotsPage);

    await createHelperBot(container, [/Chat in direct messages/]);

    await vi.waitFor(() =>
      expect(getToasts().map((item) => item.message)).toContain(
        'Some permissions could not be given to the bot. Check them on the Permissions tab.'
      )
    );
    await userEvent.click(page.getByRole('button', { name: 'Got it' }));
    await vi.waitFor(() =>
      expect(navigation.goto).toHaveBeenCalledWith('/chat/-/manage/server/bots/B1/permissions')
    );
  });

  it('opens the Permissions tab when no capability is selected', async () => {
    server.permissions.canCreateBots = true;
    const { container } = render(BotsPage);

    await createHelperBot(container, []);

    await userEvent.click(page.getByRole('button', { name: 'Got it' }));
    await vi.waitFor(() =>
      expect(navigation.goto).toHaveBeenCalledWith('/chat/-/manage/server/bots/B1/permissions')
    );
    expect(permissions.setUserPermission).not.toHaveBeenCalled();
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
