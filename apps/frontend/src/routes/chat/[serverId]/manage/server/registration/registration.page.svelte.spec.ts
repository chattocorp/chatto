import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { ConnectError, Code } from '@connectrpc/connect';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { AccountCreationPolicy } from '@chatto/api-types/api/v1/server_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import { queryClient } from '$lib/query/client';
import RegistrationPage from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Registration' }));

const settings = mockService(AdminServerService);
const refreshProfile = vi.fn().mockResolvedValue(undefined);
beforeAll(() => loadLocaleMessages('en-GB'));
beforeEach(() => {
  vi.clearAllMocks();
  queryClient.clear();
  settings.getServerConfig.mockReturnValue({
    config: { registration: { admission: AccountCreationPolicy.OPEN } }
  });
  settings.updateServerConfig.mockReturnValue({
    config: { registration: { admission: AccountCreationPolicy.INVITE_ONLY } }
  });
  createTestServerScope({
    permissions: { canManageServer: true, canManageInvites: true },
    serverInfo: { refreshProfile, directRegistrationEnabled: true, authProviders: [] },
    routes: (router) => router.service(AdminServerService, settings)
  });
});
afterEach(() => queryClient.clear());

describe('Registration settings', () => {
  it('saves admission changes without embedding invitation controls', async () => {
    const view = render(RegistrationPage);
    await expect.element(view.getByRole('radio', { name: /^Open Registration/ })).toBeChecked();
    await expect.element(view.getByRole('link', { name: 'Invites' })).not.toBeInTheDocument();
    await expect.element(view.getByRole('table')).not.toBeInTheDocument();
    await userEvent.click(view.getByRole('radio', { name: /^Invite Code Required/ }));
    await expect.element(view.getByRole('radio', { name: /^Invite Code Required/ })).toBeChecked();
    await expect.element(view.getByRole('link', { name: 'Invites' })).not.toBeInTheDocument();
    expect(settings.updateServerConfig.mock.calls[0][0]).toMatchObject({
      registration: { admission: AccountCreationPolicy.INVITE_ONLY },
      updateMask: { paths: ['registration.admission'] }
    });
    expect(refreshProfile).toHaveBeenCalledOnce();
    settings.updateServerConfig.mockReturnValue({
      config: { registration: { admission: AccountCreationPolicy.OPEN } }
    });
    await userEvent.click(view.getByRole('radio', { name: /^Open Registration/ }));
    await expect.element(view.getByRole('radio', { name: /^Open Registration/ })).toBeChecked();
    await expect.element(view.getByRole('link', { name: 'Invites' })).not.toBeInTheDocument();
  });

  it('keeps the previous mode when saving fails', async () => {
    settings.updateServerConfig.mockRejectedValue(
      new ConnectError('Unavailable', Code.Unavailable)
    );
    const view = render(RegistrationPage);
    await userEvent.click(view.getByRole('radio', { name: /^Invite Code Required/ }));
    await vi.waitFor(() => expect(settings.updateServerConfig).toHaveBeenCalledOnce());
    await expect.element(view.getByRole('radio', { name: /^Open Registration/ })).toBeChecked();
    await expect.element(view.getByRole('link', { name: 'Invites' })).not.toBeInTheDocument();
  });
});
