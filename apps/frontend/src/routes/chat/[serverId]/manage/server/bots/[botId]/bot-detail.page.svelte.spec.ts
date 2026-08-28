import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { settingsQueryKeys } from '$lib/query/settings';
import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

const mocks = vi.hoisted(() => ({
  getBot: vi.fn(),
  batchGetUsers: vi.fn(),
  listUsers: vi.fn(),
  rotateBotAPIKey: vi.fn(),
  reassignBotOwner: vi.fn(),
  createBotIncomingWebhook: vi.fn(),
  revokeBotIncomingWebhook: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  settings: null as { timezone: string; timeFormat: TimeFormat } | null,
  canManageBots: true,
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
    apiKeyRotatedAt: null,
    incomingWebhooks: []
  }
}));

vi.mock('$app/state', () => ({ page: { params: { botId: 'bot-user-id' } } }));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'server-1',
    store: {
      serverInfo: { supportsFeature: () => true },
      currentUser: { user: { settings: mocks.settings } },
      projection: {
        viewer: {
          user: { profile: { id: 'viewer', login: 'viewer', displayName: 'Viewer' } },
          viewerPermissions: {
            permissions: [{ permission: 'bot.manage', granted: mocks.canManageBots }]
          }
        }
      }
    },
    connection: {
      queryScope: 'session-1',
      getAPI: () => ({
        getBot: mocks.getBot,
        batchGetUsers: mocks.batchGetUsers,
        listUsers: mocks.listUsers,
        rotateBotAPIKey: mocks.rotateBotAPIKey,
        reassignBotOwner: mocks.reassignBotOwner,
        createBotIncomingWebhook: mocks.createBotIncomingWebhook,
        revokeBotIncomingWebhook: mocks.revokeBotIncomingWebhook
      })
    },
    isCurrent: () => true
  })
}));

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
    (candidate) => candidate.textContent?.trim() === text
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
    mocks.settings = null;
    mocks.canManageBots = true;
    mocks.getBot.mockResolvedValue(mocks.bot);
    mocks.batchGetUsers.mockResolvedValue([]);
    mocks.listUsers.mockResolvedValue({ members: [], totalCount: 0, hasMore: false });
    mocks.reassignBotOwner.mockImplementation((botId: string, ownerUserId: string) =>
      Promise.resolve({ ...mocks.bot, id: botId, ownerUserId })
    );
    mocks.rotateBotAPIKey.mockResolvedValue({ bot: mocks.bot, apiKey: 'rotated-secret' });
    mocks.createBotIncomingWebhook.mockResolvedValue({
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
    mocks.revokeBotIncomingWebhook.mockResolvedValue({ ...mocks.bot, incomingWebhooks: [] });
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

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
      expect(mocks.createBotIncomingWebhook).toHaveBeenCalledWith('bot-user-id', 'Production')
    );
    flushSync();

    expect(container.textContent).toContain('https://chat.example/webhooks/incoming/secret');
    expect(container.textContent).toContain('This URL is shown only once');
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
    mocks.getBot.mockResolvedValueOnce(hydrated).mockImplementation(() => new Promise(() => {}));
    mocks.createBotIncomingWebhook.mockResolvedValue({
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
    await vi.waitFor(() => expect(mocks.getBot).toHaveBeenCalledTimes(2));

    const cached = queryClient.getQueryData<typeof hydrated>(
      settingsQueryKeys.bot('server-1', { queryScope: 'session-1' }, 'bot-user-id')
    );
    expect(cached?.incomingWebhooks[0]).toMatchObject({
      id: 'existing-webhook',
      lastUsedState: 'recorded',
      lastUsedAt: recordedAt
    });
  });

  it('shows independent webhook lifecycle and last-use states', async () => {
    const recordedAt = new Date('2026-08-27T12:30:00Z');
    mocks.getBot.mockResolvedValue({
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
    mocks.batchGetUsers.mockResolvedValue([
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

    expect(mocks.batchGetUsers).toHaveBeenCalledWith(['owner-user-id']);
    expect(container.textContent).toContain('User ID');
    expect(container.textContent).toContain('bot-user-id');
    expect(container.querySelector('button[title="Copy to clipboard"]')).not.toBeNull();
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('Alice Owner');
    expect(container.textContent).not.toContain('owner-user-id');
    expect(container.querySelector('[data-testid="user-identity"]')).not.toBeNull();
  });

  it("formats API key timestamps with the viewer's timezone and time format", async () => {
    mocks.settings = {
      timezone: 'America/New_York',
      timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
    };
    const { container } = render(BotDetailPage);
    await settle();

    const expected = formatDateTime(
      mocks.bot.apiKeyCreatedAt,
      timeFormatSettingsFor(mocks.settings),
      'en-GB'
    );
    expect(container.textContent).toContain(expected);
  });

  it('shows owner reassignment only to bot managers', async () => {
    mocks.canManageBots = false;
    const { container } = render(BotDetailPage);
    await settle();

    expect(container.textContent).not.toContain('Reassign owner');
  });

  it('reassigns the bot to a selected human owner', async () => {
    mocks.listUsers.mockResolvedValue({
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
      expect(mocks.reassignBotOwner).toHaveBeenCalledWith('bot-user-id', 'recipient-user-id')
    );
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Bot owner reassigned');
  });
});
