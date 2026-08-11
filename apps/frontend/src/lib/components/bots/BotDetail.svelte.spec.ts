import '../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { BotAccount } from '$lib/api-client/bots';
import BotDetail from './BotDetail.svelte';

const mocks = vi.hoisted(() => ({
  getBot: vi.fn(),
  listApplicationCapabilities: vi.fn()
}));

vi.mock('$app/navigation', () => ({
  afterNavigate: vi.fn(),
  beforeNavigate: vi.fn(),
  goto: vi.fn(),
  invalidate: vi.fn(),
  invalidateAll: vi.fn(),
  onNavigate: vi.fn(),
  preloadCode: vi.fn(),
  preloadData: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn()
}));
vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: (path: string) => path
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    connection: { connectBaseUrl: '/api/connect', bearerToken: 'token' },
    store: { currentUser: { user: { id: 'owner-1' } } }
  })
}));

vi.mock('$lib/api-client/bots', () => ({
  createBotAPI: () => ({
    getBot: mocks.getBot,
    listApplicationCapabilities: mocks.listApplicationCapabilities
  })
}));

function bot(): BotAccount {
  return {
    id: 'bot-1',
    login: 'helper_bot',
    displayName: 'Helper Bot',
    avatarUrl: null,
    ownerId: 'owner-1',
    description: 'Helps people',
    capabilities: [],
    createdAt: '2026-07-22T12:00:00.000Z',
    apiKeyCreatedAt: null
  };
}

function button(container: HTMLElement, label: string): HTMLButtonElement {
  const found = [...container.querySelectorAll('button')].find((item) =>
    item.textContent?.includes(label)
  );
  if (!found) throw new Error(`button not found: ${label}`);
  return found;
}

describe('BotDetail', () => {
  beforeEach(() => {
    mocks.getBot.mockReset();
    mocks.listApplicationCapabilities.mockReset();
    mocks.listApplicationCapabilities.mockResolvedValue([]);
  });

  it('offers to generate a key to the owner from the admin view after revocation', async () => {
    mocks.getBot.mockResolvedValue(bot());
    const { container } = render(BotDetail, {
      props: { botId: 'bot-1', scope: 'admin' }
    });

    await vi.waitFor(() => expect(container.textContent).toContain('Generate API key'));
    expect(container.textContent).not.toContain('Reset API key');

    button(container, 'Generate API key').click();

    await vi.waitFor(() => expect(container.textContent).toContain('Generate API key?'));
    expect(container.textContent).not.toContain('Reset API key?');
  });
});
