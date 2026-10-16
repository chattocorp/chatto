import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { Timestamp } from '@bufbuild/protobuf';
import { InviteLinkService } from '@chatto/api-types/api/v1/invitations_connect';
import { queryClient } from '$lib/query/client';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { InviteLinkStatus } from '@chatto/api-types/api/v1/invitations_pb';
import { limitedServerFeatures } from '@chatto/client/server/compatibility';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import InviteLinksPage from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

const invitations = mockService(InviteLinkService);
const users = mockService(UserService);
const link = 'https://chat.example/join/test-invitation';

describe('Invitation link copying', () => {
  beforeEach(() => {
    queryClient.clear();
    toast.clear();
    createTestServerScope({
      routes: (router) => router.service(InviteLinkService, invitations)
    });
    invitations.listInviteLinks.mockReturnValue({
      inviteLinks: [
        { id: 'invitation-1', link, createdAt: Timestamp.fromDate(new Date('2026-01-01')) }
      ],
      page: { totalCount: 1n, hasMore: false }
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    toast.clear();
    queryClient.clear();
  });

  it('reports a denied clipboard write and lets the user retry', async () => {
    const writeText = vi
      .spyOn(navigator.clipboard, 'writeText')
      .mockRejectedValueOnce(new Error('private browser detail'))
      .mockResolvedValue(undefined);
    const { container } = render(InviteLinksPage);
    const copyButton = () =>
      [...container.querySelectorAll('button')].find(
        (button) => button.textContent?.trim() === 'Copy link'
      );
    await vi.waitFor(() => expect(copyButton()).toBeDefined());

    await userEvent.click(copyButton()!);

    expect(getToasts()).toMatchObject([{ tone: 'error', message: 'Failed to copy link' }]);
    toast.clear();

    await userEvent.click(copyButton()!);

    expect(writeText).toHaveBeenNthCalledWith(2, link);
    expect(getToasts()).toMatchObject([{ tone: 'success', message: 'Invite link copied' }]);
  });

  it('shows personal management without creation or global authority', async () => {
    const screen = render(InviteLinksPage);
    await expect.element(screen.getByRole('heading', { name: 'Your invite links' })).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Copy link' })).toBeVisible();
    expect(screen.container.textContent).not.toContain('Create invite link');
  });

  it('explains older-server support without requesting invite links', async () => {
    createTestServerScope({
      serverInfo: { limitedFeatures: limitedServerFeatures('0.5.0-beta.17') },
      permissions: { canManageInviteLinks: true },
      routes: (router) => router.service(InviteLinkService, invitations)
    });
    invitations.listInviteLinks.mockClear();
    const screen = render(InviteLinksPage);
    await expect
      .element(screen.getByRole('link', { name: 'View compatibility' }))
      .toHaveAttribute('href', '/chat/-/info#inviteLinks');
    expect(invitations.listInviteLinks).not.toHaveBeenCalled();
    expect(screen.container.querySelector('table')).toBeNull();
  });

  it('clears a global link and its revoke dialog when shield mode ends, including late reads', async () => {
    const fixture = createTestServerScope({
      permissions: { canManageInviteLinks: true },
      routes: (router) => {
        router.service(InviteLinkService, invitations);
        router.service(UserService, users);
      }
    });
    users.batchGetUsers.mockReturnValue({ users: [] });
    const foreign = {
      id: 'foreign',
      link: 'https://chat.example/invite/foreign',
      createdBy: 'other-user',
      status: InviteLinkStatus.ACTIVE,
      createdAt: Timestamp.fromDate(new Date('2026-01-01'))
    };
    invitations.listInviteLinks.mockReturnValue({ inviteLinks: [foreign] });
    const screen = render(InviteLinksPage);
    await expect.element(screen.getByText('other-user')).toBeVisible();
    await screen.getByRole('button', { name: 'Revoke', exact: true }).click();
    await expect.element(screen.getByRole('dialog')).toBeVisible();

    let finishOld!: (value: { inviteLinks: (typeof foreign)[] }) => void;
    invitations.listInviteLinks.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishOld = resolve;
        })
    );
    void queryClient.invalidateQueries();
    await vi.waitFor(() => expect(finishOld).toBeTypeOf('function'));
    invitations.listInviteLinks.mockReturnValue({ inviteLinks: [] });
    fixture.permissions = { ...fixture.permissions, canManageInviteLinks: false };
    removeRegisteredAdminQueries(fixture.serverId);
    await expect.element(screen.getByRole('heading', { name: 'Your invite links' })).toBeVisible();
    await vi.waitFor(() => expect(screen.container.textContent).not.toContain('other-user'));
    await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument();
    finishOld({ inviteLinks: [foreign] });
    await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
    expect(screen.container.textContent).not.toContain('other-user');
    expect(screen.container.querySelectorAll('button')).toHaveLength(0);
  });
});
