import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { ClientConfiguration } from '$lib/clientConfig';

const mocks = {
  configuration: { version: 1, authling: null } as {
    version: 1;
    authling: { issuer: string; clientId: string } | null;
  },
  getConfiguration: vi.fn<() => Promise<ClientConfiguration>>(async () => ({
    version: 1,
    authling: null
  })),
  sync: {
    status: 'disconnected' as const,
    providerLabel: null as string | null,
    accountId: null as string | null,
    initialize: vi.fn(async () => {}),
    connect: vi.fn(async () => {})
  },
  loadSyncModule: vi.fn()
};

vi.mock('$lib/ui/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() }
}));

import AccountDataSyncButton from './AccountDataSyncButton.svelte';

beforeEach(() => {
  mocks.configuration = { version: 1, authling: null };
  mocks.getConfiguration.mockImplementation(async () => mocks.configuration);
  mocks.sync.initialize.mockClear();
  mocks.loadSyncModule.mockImplementation(async () => ({ accountDataSync: mocks.sync }));
});

describe('AccountDataSyncButton', () => {
  it('stays hidden when the frontend origin does not configure Authling', async () => {
    const { container } = render(AccountDataSyncButton, {
      props: {
        getConfiguration: mocks.getConfiguration,
        loadSyncModule: mocks.loadSyncModule
      }
    });
    await vi.waitFor(() => expect(container.querySelector('button')).toBeNull());
    expect(mocks.sync.initialize).not.toHaveBeenCalled();
  });

  it('loads synchronization when the frontend origin configures Authling', async () => {
    mocks.configuration = {
      version: 1,
      authling: {
        issuer: 'https://id.example',
        clientId: 'https://client.example/oauth/client-metadata.json'
      }
    };
    const { container } = render(AccountDataSyncButton, {
      props: {
        getConfiguration: mocks.getConfiguration,
        loadSyncModule: mocks.loadSyncModule
      }
    });

    await vi.waitFor(() =>
      expect(container.querySelector('[data-state="disconnected"]')).not.toBeNull()
    );
    expect(container.querySelector('[class~="icon-[uil--sync]"]')).not.toBeNull();
    expect(mocks.sync.initialize).toHaveBeenCalledOnce();
  });
});
