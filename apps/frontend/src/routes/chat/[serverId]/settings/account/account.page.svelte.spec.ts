import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import AccountPage from './+page.svelte';

const mocks = vi.hoisted(() => ({
  listExternalIdentities: vi.fn(),
  currentUser: {
    user: {
      id: 'U123abcetc.',
      login: 'alice',
      displayName: 'Alice',
      hasPassword: true,
      viewerCanDeleteAccount: false
    },
    loading: false,
    load: vi.fn()
  }
}));

const connection = {
  queryScope: 'account-settings-test',
  getAPI: () => ({
    list: mocks.listExternalIdentities
  })
};

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: { currentUser: mocks.currentUser },
    connection,
    isCurrent: () => true
  })
}));

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('Account settings page', () => {
  beforeEach(() => {
    mocks.listExternalIdentities.mockReset();
    mocks.listExternalIdentities.mockResolvedValue({ providers: [], linkedIdentities: [] });
  });

  it('shows the current user ID in account information', async () => {
    const { container, getByText } = render(AccountPage);
    await settle();

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(3);
    await expect.element(getByText('User ID')).toBeVisible();
    await expect.element(getByText('U123abcetc.')).toBeVisible();
  });
});
