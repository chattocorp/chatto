import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { render } from 'vitest-browser-svelte';
import { RoomKind } from '$lib/api-client/roomDirectory';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { settingsQueryKeys } from '$lib/query/settings';
import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  bot: {
    id: 'bot-user-id',
    login: 'helper_bot',
    displayName: 'Helper Bot',
    avatarUrl: null,
    bio: 'Initial bot bio',
    timezone: null,
    ownerUserId: 'owner-user-id',
    createdAt: null,
    apiKeyCreatedAt: new Date('2026-08-21T12:00:00Z'),
    lastLoginChange: null as Date | null,
    apiKeys: [
      {
        id: 'legacy',
        name: 'Default key',
        createdAt: new Date('2026-08-21T12:00:00Z'),
        lastUsedState: 'no_use_recorded' as const,
        lastUsedAt: null
      }
    ],
    incomingWebhooks: []
  }
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { botId: routeBotId };
    }
  }
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

const api = {
  getBot: vi.fn(),
  listOutboundWebhooks: vi.fn(),
  batchGetUsers: vi.fn(),
  listUsers: vi.fn(),
  createBotAPIKey: vi.fn(),
  revokeBotAPIKey: vi.fn(),
  reassignBotOwner: vi.fn(),
  createBotIncomingWebhook: vi.fn(),
  revokeBotIncomingWebhook: vi.fn(),
  updateUserProfile: vi.fn(),
  uploadAvatar: vi.fn(),
  deleteAvatar: vi.fn()
};
let server: TestServerScope;
let routeBotId = $state('bot-user-id');

vi.mock('$lib/components/rbac', async () => ({
  UserPermissionsMatrix: (await import('./BotUserPermissionsMatrixMock.svelte')).default
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError }
}));

import BotDetailPage from './+page.svelte';

function setInput(input: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

function buttonByText(root: ParentNode, text: string): HTMLButtonElement {
  const button = [...root.querySelectorAll('button')].find(
    (candidate) => (candidate.getAttribute('aria-label') || candidate.textContent?.trim()) === text
  );
  if (!(button instanceof HTMLButtonElement)) throw new Error(`Button not found: ${text}`);
  return button;
}

async function settle(): Promise<void> {
  await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
  flushSync();
}

describe('Bot detail page', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    routeBotId = 'bot-user-id';
    server = createTestServerScope({
      api,
      permissions: { canManageBots: true },
      store: {
        navigation: {
          rooms: [
            { id: 'R-alerts', name: 'alerts', type: RoomKind.CHANNEL },
            { id: 'R-general', name: 'general', type: RoomKind.CHANNEL },
            { id: 'R-dm', name: 'Private conversation', type: RoomKind.DM }
          ]
        }
      }
    });
    mocks.bot.lastLoginChange = null;
    api.updateUserProfile.mockImplementation(
      (userId: string, input: { login?: string; displayName?: string; bio?: string }) =>
        Promise.resolve({
          id: userId,
          login: input.login ?? mocks.bot.login,
          displayName: input.displayName ?? mocks.bot.displayName,
          bio: input.bio ?? mocks.bot.bio,
          deleted: false,
          avatarUrl: null
        })
    );
    api.listOutboundWebhooks.mockResolvedValue([]);
    api.getBot.mockResolvedValue(mocks.bot);
    api.batchGetUsers.mockResolvedValue([]);
    api.listUsers.mockResolvedValue({ members: [], totalCount: 0, hasMore: false });
    api.reassignBotOwner.mockImplementation((botId: string, ownerUserId: string) =>
      Promise.resolve({ ...mocks.bot, id: botId, ownerUserId })
    );
    api.createBotAPIKey.mockResolvedValue({
      bot: {
        ...mocks.bot,
        apiKeys: [
          ...mocks.bot.apiKeys,
          {
            id: 'K-production',
            name: 'Production',
            createdAt: new Date(),
            lastUsedState: 'no_use_recorded' as const,
            lastUsedAt: null
          }
        ]
      },
      apiKey: 'created-secret'
    });
    api.revokeBotAPIKey.mockResolvedValue({ ...mocks.bot, apiKeys: [] });
    api.createBotIncomingWebhook.mockResolvedValue({
      bot: {
        ...mocks.bot,
        incomingWebhooks: [
          {
            id: 'webhook-id',
            name: 'Production',
            createdAt: new Date(),
            lastUsedState: 'no_use_recorded' as const,
            lastUsedAt: null
          }
        ]
      },
      webhookUrl: 'https://chat.example/webhooks/incoming/secret'
    });
    api.revokeBotIncomingWebhook.mockResolvedValue({ ...mocks.bot, incomingWebhooks: [] });
    api.uploadAvatar.mockResolvedValue({ id: mocks.bot.id, avatarUrl: '/bot-avatar.webp' });
    api.deleteAvatar.mockResolvedValue({ id: mocks.bot.id, avatarUrl: null });
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it.each([true, false])(
    'gates outbound webhook settings on server support (%s)',
    async (supported) => {
      server.features = { botOutboundWebhooks: supported };
      const { container } = render(BotDetailPage);
      await settle();
      expect(container.querySelector('[data-testid="bot-outbound-webhooks"]') !== null).toBe(
        supported
      );
      expect(api.listOutboundWebhooks).toHaveBeenCalledTimes(supported ? 1 : 0);
    }
  );

  it('creates a named incoming webhook and shows its URL once', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    buttonByText(container, 'Create Webhook').click();
    flushSync();
    setInput(container.querySelector('#create-bot-webhook-name') as HTMLInputElement, 'Production');
    const createButtons = [...container.querySelectorAll('button')].filter(
      (button) => button.textContent?.trim() === 'Create Webhook'
    );
    createButtons.at(-1)?.click();
    await vi.waitFor(() =>
      expect(api.createBotIncomingWebhook).toHaveBeenCalledWith('bot-user-id', 'Production')
    );
    await vi.waitFor(() =>
      expect(container.textContent).toContain('https://chat.example/webhooks/incoming/secret')
    );
    expect(container.textContent).toContain('This URL is shown only once');
  });

  it('includes the selected room in the show-once URL and resets it for the next webhook', async () => {
    api.createBotIncomingWebhook.mockResolvedValue({
      webhookUrl: 'https://chat.example/webhooks/incoming/secret?existing=keep'
    });
    const { container } = render(BotDetailPage);
    await settle();

    buttonByText(container, 'Create Webhook').click();
    flushSync();
    const select = container.querySelector('#create-bot-webhook-room') as HTMLSelectElement;
    expect([...select.options].map((option) => option.value)).toEqual([
      '',
      'R-alerts',
      'R-general'
    ]);
    select.value = 'R-alerts';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    flushSync();
    setInput(container.querySelector('#create-bot-webhook-name') as HTMLInputElement, 'Grafana');
    buttonByText(container.querySelector('dialog[open]')!, 'Create Webhook').click();

    await vi.waitFor(() =>
      expect(container.querySelector('dialog[open] code')?.textContent).toBe(
        'https://chat.example/webhooks/incoming/secret?existing=keep&room_id=R-alerts'
      )
    );
    expect(api.createBotIncomingWebhook).toHaveBeenCalledWith('bot-user-id', 'Grafana');
    buttonByText(container, 'Got it').click();
    flushSync();
    expect(container.textContent).not.toContain('/incoming/secret');
    buttonByText(container, 'Create Webhook').click();
    flushSync();
    expect((container.querySelector('#create-bot-webhook-room') as HTMLSelectElement).value).toBe(
      ''
    );
  });

  it('saves only the changed bot profile fields and caches the result', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    setInput(
      container.querySelector('[data-testid="bot-profile-display-name"]') as HTMLInputElement,
      'Renamed Bot'
    );
    buttonByText(container, 'Save changes').click();

    await vi.waitFor(() =>
      expect(api.updateUserProfile).toHaveBeenCalledWith('bot-user-id', {
        displayName: 'Renamed Bot'
      })
    );
    await vi.waitFor(() => {
      const cached = queryClient.getQueryData<{ displayName: string; bio: string | null }>(
        settingsQueryKeys.bot('server-1', server.scope.connection, 'bot-user-id')
      );
      expect(cached?.displayName).toBe('Renamed Bot');
      expect(cached?.bio).toBe('Initial bot bio');
    });
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Bot profile updated');
  });

  it('keeps the bot profile draft when a save fails', async () => {
    api.updateUserProfile.mockRejectedValueOnce(new Error('Username is already taken'));
    const { container } = render(BotDetailPage);
    await settle();

    const login = container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement;
    setInput(login, 'taken_login');
    buttonByText(container, 'Save changes').click();
    await vi.waitFor(() => buttonByText(document, 'Change username').click());

    await vi.waitFor(() => expect(container.textContent).toContain('Username is already taken'));
    expect(login.value).toBe('taken_login');
  });

  it('does not send back an untouched field that changed during the edit', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    setInput(
      container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement,
      'renamed_bot'
    );
    // A realtime refresh delivers another manager's display-name change.
    queryClient.setQueryData(
      settingsQueryKeys.bot('server-1', server.scope.connection, 'bot-user-id'),
      { ...mocks.bot, displayName: 'Renamed Elsewhere' }
    );
    flushSync();
    await vi.waitFor(() => expect(container.textContent).toContain('Renamed Elsewhere'));
    buttonByText(container, 'Save changes').click();
    await vi.waitFor(() => buttonByText(document, 'Change username').click());

    await vi.waitFor(() =>
      expect(api.updateUserProfile).toHaveBeenCalledWith('bot-user-id', {
        login: 'renamed_bot'
      })
    );
  });

  it('shows a localized message when the profile changed concurrently', async () => {
    api.updateUserProfile.mockRejectedValueOnce(
      new ConnectError('optimistic concurrency sequence mismatch', Code.Aborted)
    );
    const { container } = render(BotDetailPage);
    await settle();

    setInput(
      container.querySelector('[data-testid="bot-profile-display-name"]') as HTMLInputElement,
      'Conflicting Name'
    );
    buttonByText(container, 'Save changes').click();

    await vi.waitFor(() =>
      expect(container.textContent).toContain('This profile changed while you were editing it.')
    );
    expect(container.textContent).not.toContain('optimistic concurrency');
  });

  it('confirms a username change and then locks the username during the cooldown', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    const login = container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement;
    setInput(login, 'fresh_name');
    buttonByText(container, 'Save changes').click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain('Change the username of this bot to @fresh_name?')
    );
    expect(api.updateUserProfile).not.toHaveBeenCalled();

    buttonByText(document, 'Change username').click();
    await vi.waitFor(() =>
      expect(api.updateUserProfile).toHaveBeenCalledWith('bot-user-id', { login: 'fresh_name' })
    );
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="bot-profile-login-cooldown"]')).not.toBeNull()
    );
    expect(login.disabled).toBe(true);
  });

  it('saves a case-only rename without confirmation or a cooldown lock', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    const login = container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement;
    setInput(login, 'Helper_Bot');
    buttonByText(container, 'Save changes').click();

    await vi.waitFor(() =>
      expect(api.updateUserProfile).toHaveBeenCalledWith('bot-user-id', { login: 'Helper_Bot' })
    );
    expect(document.body.textContent).not.toContain('Change the username of this bot');
    await vi.waitFor(() => expect(mocks.toastSuccess).toHaveBeenCalled());
    expect(login.disabled).toBe(false);
    expect(container.querySelector('[data-testid="bot-profile-login-cooldown"]')).toBeNull();
  });

  it('keeps the started cooldown in the bot cache after a rename', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    setInput(
      container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement,
      'fresh_name'
    );
    buttonByText(container, 'Save changes').click();
    await vi.waitFor(() => buttonByText(document, 'Change username').click());

    await vi.waitFor(() => {
      const cached = queryClient.getQueryData<{ lastLoginChange: Date | null }>(
        settingsQueryKeys.bot('server-1', server.scope.connection, 'bot-user-id')
      );
      expect(cached?.lastLoginChange).toBeInstanceOf(Date);
    });
  });

  it('explains why a username edit cannot be saved when a cooldown starts during the edit', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    setInput(
      container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement,
      'late_rename'
    );
    // Another manager's rename arrives while this form is open.
    queryClient.setQueryData(
      settingsQueryKeys.bot('server-1', server.scope.connection, 'bot-user-id'),
      { ...mocks.bot, lastLoginChange: new Date() }
    );
    flushSync();
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="bot-profile-login-cooldown"]')).not.toBeNull()
    );
    buttonByText(container, 'Save changes').click();

    await vi.waitFor(() =>
      expect(container.querySelector('.form-error')?.textContent).toContain(
        'The username of this bot can change again in'
      )
    );
    expect(api.updateUserProfile).not.toHaveBeenCalled();
  });

  it('uses bot-specific help text for the bio', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    expect(container.textContent).toContain('shown on the profile of the bot.');
    expect(container.textContent).not.toContain('shown on your profile');
  });

  it('locks the username while the bot cooldown is active', async () => {
    mocks.bot.lastLoginChange = new Date();
    const { container } = render(BotDetailPage);
    await settle();

    const login = container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement;
    expect(login.disabled).toBe(true);
    expect(
      container.querySelector('[data-testid="bot-profile-login-cooldown"]')?.textContent
    ).toContain('The username of this bot can change again in');
  });

  it('lets an account manager rename a bot during its cooldown without confirmation', async () => {
    server.permissions.canManageBots = false;
    server.permissions.canAdminManageAccounts = true;
    mocks.bot.lastLoginChange = new Date();
    const { container } = render(BotDetailPage);
    await settle();

    const login = container.querySelector('[data-testid="bot-profile-login"]') as HTMLInputElement;
    expect(login.disabled).toBe(false);
    setInput(login, 'admin_renamed');
    buttonByText(container, 'Save changes').click();

    await vi.waitFor(() =>
      expect(api.updateUserProfile).toHaveBeenCalledWith('bot-user-id', {
        login: 'admin_renamed'
      })
    );
    expect(document.body.textContent).not.toContain('Change the username of this bot');
  });

  it('hides bot profile editing on servers without managed profile updates', async () => {
    server.features = { managedUserProfiles: false };
    const { container } = render(BotDetailPage);
    await settle();

    expect(container.querySelector('[data-testid="bot-profile-login"]')).toBeNull();
  });

  it('hides bot profile editing from viewers who cannot manage the bot', async () => {
    server.permissions.canManageBots = false;
    const { container } = render(BotDetailPage);
    await settle();

    expect(container.querySelector('[data-testid="bot-profile-login"]')).toBeNull();
  });

  it('uploads the selected bot avatar through the user API', async () => {
    const { container } = render(BotDetailPage);
    await settle();
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'bot.png', {
      type: 'image/png'
    });
    const input = container.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { configurable: true, value: [file] });
    input.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => expect(api.uploadAvatar).toHaveBeenCalledWith('bot-user-id', file));
    await vi.waitFor(() => {
      const cached = queryClient.getQueryData<{ avatarUrl: string | null }>(
        settingsQueryKeys.bot('server-1', server.scope.connection, 'bot-user-id')
      );
      expect(cached?.avatarUrl).toBe('/bot-avatar.webp');
    });
  });

  it('creates and revokes API keys independently', async () => {
    const { container } = render(BotDetailPage);
    await settle();

    buttonByText(container, 'Create API key').click();
    flushSync();
    setInput(container.querySelector('#create-bot-api-key-name') as HTMLInputElement, 'Production');
    const createButtons = [...container.querySelectorAll('button')].filter(
      (button) => button.textContent?.trim() === 'Create API key'
    );
    createButtons.at(-1)?.click();
    await vi.waitFor(() =>
      expect(api.createBotAPIKey).toHaveBeenCalledWith('bot-user-id', 'Production')
    );
    await vi.waitFor(() => expect(container.textContent).toContain('created-secret'));

    buttonByText(container, 'Revoke key').click();
    flushSync();
    const revokeButtons = [...document.querySelectorAll('button')].filter(
      (button) => button.textContent?.trim() === 'Revoke key'
    );
    revokeButtons.at(-1)?.click();
    await vi.waitFor(() =>
      expect(api.revokeBotAPIKey).toHaveBeenCalledWith('bot-user-id', 'legacy')
    );
  });

  it('closes a pending credential revocation when the route reuses the page for another bot', async () => {
    api.getBot.mockImplementation((botId: string) => Promise.resolve({ ...mocks.bot, id: botId }));
    const { container } = render(BotDetailPage);
    await settle();

    buttonByText(container, 'Revoke key').click();
    flushSync();
    expect(container.querySelector('dialog[open]')).not.toBeNull();

    queryClient.setQueryData(settingsQueryKeys.bot('server-1', server.scope.connection, 'bot-b'), {
      ...mocks.bot,
      id: 'bot-b'
    });
    routeBotId = 'bot-b';
    await vi.waitFor(() => expect(container.textContent).toContain('bot-b'));
    await settle();

    expect(container.querySelector('dialog[open]')).toBeNull();
    expect(api.revokeBotAPIKey).not.toHaveBeenCalled();
  });

  it('keeps hydrated webhook telemetry while it refetches after credential issuance', async () => {
    const recordedAt = new Date('2026-08-27T12:30:00Z');
    const hydrated = {
      ...mocks.bot,
      incomingWebhooks: [
        {
          id: 'existing-webhook',
          name: 'Monitoring',
          createdAt: new Date('2026-08-27T11:00:00Z'),
          lastUsedState: 'recorded' as const,
          lastUsedAt: recordedAt
        }
      ]
    };
    api.getBot.mockResolvedValueOnce(hydrated).mockImplementation(() => new Promise(() => {}));
    api.createBotIncomingWebhook.mockResolvedValue({
      bot: {
        ...hydrated,
        incomingWebhooks: [
          { ...hydrated.incomingWebhooks[0], lastUsedState: 'unavailable', lastUsedAt: null },
          {
            id: 'new-webhook',
            name: 'Production',
            createdAt: new Date(),
            lastUsedState: 'no_use_recorded',
            lastUsedAt: null
          }
        ]
      },
      webhookUrl: 'https://chat.example/webhooks/incoming/secret'
    });
    const { container } = render(BotDetailPage);
    await settle();

    buttonByText(container, 'Create Webhook').click();
    flushSync();
    setInput(container.querySelector('#create-bot-webhook-name') as HTMLInputElement, 'Production');
    const createButtons = [...container.querySelectorAll('button')].filter(
      (button) => button.textContent?.trim() === 'Create Webhook'
    );
    createButtons.at(-1)?.click();
    await vi.waitFor(() => expect(api.getBot).toHaveBeenCalledTimes(2));

    const cached = queryClient.getQueryData<typeof hydrated>(
      settingsQueryKeys.bot('server-1', server.scope.connection, 'bot-user-id')
    );
    expect(cached?.incomingWebhooks[0]).toMatchObject({
      id: 'existing-webhook',
      lastUsedState: 'recorded',
      lastUsedAt: recordedAt
    });
  });

  it('shows independent webhook lifecycle and last-use states', async () => {
    const recordedAt = new Date('2026-08-27T12:30:00Z');
    api.getBot.mockResolvedValue({
      ...mocks.bot,
      incomingWebhooks: [
        {
          id: 'first',
          name: 'Production',
          createdAt: new Date('2026-08-27T10:00:00Z'),
          lastUsedState: 'no_use_recorded',
          lastUsedAt: null
        },
        {
          id: 'second',
          name: 'Monitoring',
          createdAt: new Date('2026-08-27T11:00:00Z'),
          lastUsedState: 'recorded',
          lastUsedAt: recordedAt
        },
        {
          id: 'third',
          name: 'Unavailable',
          createdAt: new Date('2026-08-27T12:00:00Z'),
          lastUsedState: 'unavailable',
          lastUsedAt: null
        }
      ]
    });
    const { container } = render(BotDetailPage);
    await settle();

    expect(container.textContent).toContain('Production');
    expect(container.textContent).toContain('Monitoring');
    expect(container.textContent).toContain('No use recorded');
    expect(container.textContent).toContain('Temporarily unavailable');
    expect(container.textContent).toContain(
      formatDateTime(recordedAt, timeFormatSettingsFor(null), 'en-GB')
    );
  });

  it('shows the bot user ID and hydrates its owner as a reusable user identity', async () => {
    api.batchGetUsers.mockResolvedValue([
      {
        id: 'owner-user-id',
        login: 'alice',
        displayName: 'Alice Owner',
        avatarUrl: null,
        deleted: false,
        isBot: false
      }
    ]);
    const { container } = render(BotDetailPage);
    await vi.waitFor(() => {
      expect(container.textContent).toContain('Alice Owner');
    });

    expect(api.batchGetUsers).toHaveBeenCalledWith(['owner-user-id']);
    expect(container.textContent).toContain('User ID');
    expect(container.textContent).toContain('bot-user-id');
    expect(container.querySelector('button[title="Copy to clipboard"]')).not.toBeNull();
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('Alice Owner');
    expect(container.textContent).not.toContain('owner-user-id');
    expect(container.querySelector('[data-testid="user-identity"]')).not.toBeNull();
  });

  it("formats API key timestamps with the viewer's timezone and time format", async () => {
    server.features = { botMultipleApiKeys: false };
    const settings = { timezone: 'America/New_York', timeFormat: TimeFormat.TIME_FORMAT_24_HOUR };
    server.currentUser.user!.settings = settings;
    const { container } = render(BotDetailPage);
    await settle();

    const expected = formatDateTime(
      mocks.bot.apiKeyCreatedAt,
      timeFormatSettingsFor(settings),
      'en-GB'
    );
    expect(container.textContent).toContain(expected);
    expect(container.textContent).not.toContain('Create API key');
    expect(container.querySelector('button[aria-label="Revoke key"]')).toBeNull();
    expect(container.textContent).not.toContain('Replace all keys');
  });

  it('shows owner reassignment only to bot managers', async () => {
    server.permissions.canManageBots = false;
    const { container } = render(BotDetailPage);
    await settle();

    expect(container.textContent).not.toContain('Reassign owner');
  });

  it('shows only identity management to an account manager who does not manage bots', async () => {
    server.permissions.canManageBots = false;
    server.permissions.canAdminManageAccounts = true;
    const { container } = render(BotDetailPage);
    await settle();

    expect(container.textContent).toContain('Upload avatar');
    expect(container.querySelector('[data-testid="bot-profile-login"]')).not.toBeNull();
    expect(container.textContent).not.toContain('Create API key');
    expect(container.textContent).not.toContain('Create incoming webhook');
    expect(container.textContent).not.toContain('Reassign owner');
  });

  it('reassigns the bot to a selected human owner', async () => {
    api.listUsers.mockResolvedValue({
      members: [
        {
          id: 'recipient-user-id',
          login: 'recipient',
          displayName: 'Recipient User',
          deleted: false,
          isBot: false,
          avatarUrl: null,
          presenceStatus: 'OFFLINE',
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      totalCount: 1,
      hasMore: false
    });
    const rendered = render(BotDetailPage);
    await settle();

    buttonByText(rendered.container, 'Reassign owner').click();
    flushSync();
    setInput(document.querySelector('#reassign-bot-owner') as HTMLInputElement, 'recipient');
    await new Promise((resolve) => setTimeout(resolve, 250));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Recipient User'));
    const recipientOption = document.querySelector('button[role="option"]');
    if (!(recipientOption instanceof HTMLButtonElement)) throw new Error('Recipient not found');
    recipientOption.click();
    flushSync();
    const submit = [...document.querySelectorAll('button')]
      .filter((button) => button.textContent?.trim() === 'Reassign owner')
      .at(-1);
    if (!(submit instanceof HTMLButtonElement)) throw new Error('Reassign submit not found');
    submit.click();

    await vi.waitFor(() =>
      expect(api.reassignBotOwner).toHaveBeenCalledWith('bot-user-id', 'recipient-user-id')
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Bot owner reassigned');
  });
});
