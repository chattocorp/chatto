import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { InviteLinkService } from '@chatto/api-types/api/v1/invitations_connect';
import { queryClient } from '$lib/query/client';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import InvitePage from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: vi.fn()
}));
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));
const invitations = mockService(InviteLinkService);

describe('member invite creation', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    queryClient.clear();
  });
  afterEach(() => {
    vi.restoreAllMocks();
    queryClient.clear();
  });

  it('creates a one-use seven-day link without global management and shows it for copying', async () => {
    createTestServerScope({
      permissions: { canCreateInviteLinks: true },
      routes: (router) => router.service(InviteLinkService, invitations)
    });
    const link = 'https://chat.example/invite/member';
    invitations.createInviteLink.mockReturnValue({ inviteLink: { id: 'I1', link } });
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
    const screen = render(InvitePage);
    await screen.getByRole('button', { name: 'Create invite link', exact: true }).click();
    await expect.element(screen.getByText(link, { exact: true })).toBeVisible();
    const request = invitations.createInviteLink.mock.calls[0][0];
    expect(request.maxUses).toBe(1);
    expect(
      Math.abs(request.expiresAt!.toDate().getTime() - Date.now() - 7 * 86_400_000)
    ).toBeLessThan(10_000);
    await screen.getByRole('button', { name: 'Copy link', exact: true }).click();
    expect(clipboard).toHaveBeenCalledWith(link);
    await expect.element(screen.getByRole('link', { name: 'Your invite links' })).toBeVisible();
  });

  it('does not offer creation to a manager without create permission', async () => {
    createTestServerScope({ permissions: { canManageInviteLinks: true } });
    const screen = render(InvitePage);
    await expect.element(screen.getByText('Access Denied', { exact: true })).toBeVisible();
    expect(invitations.createInviteLink).not.toHaveBeenCalled();
  });

  it('discards a late creation response after permissions change', async () => {
    const fixture = createTestServerScope({
      permissions: { canCreateInviteLinks: true },
      routes: (router) => router.service(InviteLinkService, invitations)
    });
    let finish!: (value: { inviteLink: { id: string; link: string } }) => void;
    invitations.createInviteLink.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    const screen = render(InvitePage);
    await screen.getByRole('button', { name: 'Create invite link', exact: true }).click();
    await vi.waitFor(() => expect(finish).toBeTypeOf('function'));
    fixture.permissions = { ...fixture.permissions, canCreateInviteLinks: false };
    removeRegisteredAdminQueries(fixture.serverId);
    finish({ inviteLink: { id: 'I1', link: 'https://chat.example/invite/late' } });
    await expect.element(screen.getByText('Access Denied', { exact: true })).toBeVisible();
    expect(screen.container.textContent).not.toContain('https://chat.example/invite/late');
  });
});
