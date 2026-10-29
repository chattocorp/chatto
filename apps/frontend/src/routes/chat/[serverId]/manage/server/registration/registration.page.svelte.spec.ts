import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { userEvent } from 'vitest/browser';
import { ConnectError, Code } from '@connectrpc/connect';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { AccountCreationPolicy } from '@chatto/api-types/api/v1/server_pb';
import { AdminInvitationCodeService } from '@chatto/api-types/admin/v1/invitations_connect';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import { queryClient } from '$lib/query/client';
import RegistrationPage from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Registration' }));

const settings = mockService(AdminServerService);
const invitations = mockService(AdminInvitationCodeService);
const signupMethods = {
  directRegistrationEnabled: true,
  authProviders: [
    { id: 'company', label: 'Company SSO', autoProvision: true },
    { id: 'existing', label: 'Existing accounts only', autoProvision: false }
  ]
};
let server: TestServerScope;
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
  invitations.listInvitationCodes.mockReturnValue({
    invitationCodes: [],
    page: { totalCount: 0n, hasMore: false }
  });
  server = createTestServerScope({
    permissions: { canManageServer: true, canManageInvites: true },
    serverInfo: { ...signupMethods },
    routes: (router) => {
      router.service(AdminServerService, settings);
      router.service(AdminInvitationCodeService, invitations);
    }
  });
});
afterEach(() => queryClient.clear());

describe('Registration admin page', () => {
  it('shows signup methods and saves only the selected admission mode', async () => {
    const page = render(RegistrationPage);
    await expect.element(page.getByRole('radio', { name: /^Invite Code Required/ })).toBeVisible();
    await expect.element(page.getByRole('radio', { name: /^Open Registration/ })).toBeChecked();
    await expect.element(page.getByText('Email and password', { exact: true })).toBeVisible();
    await expect.element(page.getByText('Company SSO', { exact: true })).toBeVisible();
    await expect
      .element(page.getByText('Existing accounts only', { exact: true }))
      .not.toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Create code', exact: true }))
      .toBeVisible();
    await userEvent.click(page.getByRole('radio', { name: /^Invite Code Required/ }));
    await vi.waitFor(() => expect(settings.updateServerConfig).toHaveBeenCalledOnce());
    expect(settings.updateServerConfig.mock.calls[0][0]).toMatchObject({
      registration: { admission: AccountCreationPolicy.INVITE_ONLY },
      updateMask: { paths: ['registration.admission'] }
    });
    await expect.element(page.getByRole('radio', { name: /^Invite Code Required/ })).toBeChecked();
    await expect.element(page.getByRole('radio', { name: /^Open Registration/ })).not.toBeChecked();

    settings.updateServerConfig.mockReturnValue({
      config: { registration: { admission: AccountCreationPolicy.OPEN } }
    });
    await userEvent.click(page.getByRole('radio', { name: /^Open Registration/ }));
    await expect.element(page.getByRole('radio', { name: /^Open Registration/ })).toBeChecked();
    expect(settings.updateServerConfig.mock.calls[1][0]).toMatchObject({
      registration: { admission: AccountCreationPolicy.OPEN }
    });
  });

  it('keeps the saved mode selected when an update fails', async () => {
    settings.updateServerConfig.mockRejectedValue(
      new ConnectError('Unavailable', Code.Unavailable)
    );
    const page = render(RegistrationPage);
    const invitation = page.getByRole('radio', { name: /^Invite Code Required/ });
    await expect.element(invitation).toBeVisible();
    await userEvent.click(invitation);
    await vi.waitFor(() => expect(settings.updateServerConfig).toHaveBeenCalledOnce());
    await expect.element(invitation).toBeEnabled();
    await expect.element(invitation).not.toBeChecked();
    await expect.element(page.getByRole('radio', { name: /^Open Registration/ })).toBeChecked();
  });

  it('explains when neither mode has an enabled signup method', async () => {
    server.scope.store.serverInfo.directRegistrationEnabled = false;
    Object.assign(server.scope.store.serverInfo, {
      authProviders: [signupMethods.authProviders[1]]
    });
    const page = render(RegistrationPage);
    await expect
      .element(
        page.getByText(
          'No signup methods are enabled. Users cannot create accounts in either mode.'
        )
      )
      .toBeVisible();
    await expect
      .element(page.getByText('Email and password', { exact: true }))
      .not.toBeInTheDocument();
  });

  it('does not load or show settings for invitation-only managers', async () => {
    server.permissions.canManageServer = false;
    const page = render(RegistrationPage);
    await expect
      .element(page.getByRole('button', { name: 'Create code', exact: true }))
      .toBeVisible();
    await expect
      .element(page.getByRole('radio', { name: /^Invite Code Required/ }))
      .not.toBeInTheDocument();
    expect(settings.getServerConfig).not.toHaveBeenCalled();
  });

  it('does not load or show codes without invitation permission', async () => {
    server.permissions.canManageInvites = false;
    const page = render(RegistrationPage);
    await expect.element(page.getByRole('radio', { name: /^Invite Code Required/ })).toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Create code', exact: true }))
      .not.toBeInTheDocument();
    expect(invitations.listInvitationCodes).not.toHaveBeenCalled();
  });
});
