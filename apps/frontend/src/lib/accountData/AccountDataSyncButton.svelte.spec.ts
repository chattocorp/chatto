import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
  configuration: { version: 1, authling: null } as {
    version: 1;
    authling: { issuer: string; clientId: string } | null;
  },
  initialize: vi.fn(async () => {})
}));

vi.mock('$lib/clientConfig', () => ({
  getClientConfiguration: vi.fn(async () => mocks.configuration)
}));

vi.mock('./sync.svelte', () => ({
  accountDataSync: {
    status: 'disconnected',
    providerLabel: null,
    initialize: mocks.initialize,
    connect: vi.fn(async () => {})
  }
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() }
}));

import AccountDataSyncButton from './AccountDataSyncButton.svelte';

beforeEach(() => {
  mocks.configuration = { version: 1, authling: null };
  mocks.initialize.mockClear();
});

describe('AccountDataSyncButton', () => {
  it('stays hidden when the frontend origin does not configure Authling', async () => {
    const { container } = render(AccountDataSyncButton);
    await vi.waitFor(() => expect(container.querySelector('button')).toBeNull());
    expect(mocks.initialize).not.toHaveBeenCalled();
  });

  it('loads synchronization when the frontend origin configures Authling', async () => {
    mocks.configuration = {
      version: 1,
      authling: {
        issuer: 'https://id.example',
        clientId: 'https://client.example/oauth/client-metadata.json'
      }
    };
    const { container } = render(AccountDataSyncButton);

    await vi.waitFor(() =>
      expect(container.querySelector('[data-state="disconnected"]')).not.toBeNull()
    );
    expect(container.querySelector('.uil--sync')).not.toBeNull();
    expect(mocks.initialize).toHaveBeenCalledOnce();
  });
});
