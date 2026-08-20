import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
  canCreateBots: false,
  query: {
    data: [] as unknown[],
    isPending: false,
    error: null as Error | null
  }
}));

vi.mock('@tanstack/svelte-query', () => ({
  createQuery: () => mocks.query
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
      projection: { viewer: {} }
    },
    connection: {
      queryScope: 'session-1',
      getAPI: vi.fn()
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/query/client', () => ({
  queryClient: { invalidateQueries: vi.fn() }
}));

import BotsPage from './+page.svelte';

function createButton(container: Element): HTMLButtonElement | undefined {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find((button) =>
    button.textContent?.includes('Create Bot')
  );
}

describe('Bot settings page', () => {
  beforeEach(() => {
    mocks.canCreateBots = false;
    mocks.query.data = [];
    mocks.query.isPending = false;
    mocks.query.error = null;
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
});
