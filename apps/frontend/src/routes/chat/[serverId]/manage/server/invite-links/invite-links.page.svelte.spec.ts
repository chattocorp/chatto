import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { Timestamp } from '@bufbuild/protobuf';
import { AdminInviteLinkService } from '@chatto/api-types/admin/v1/invitations_connect';
import { queryClient } from '$lib/query/client';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import InviteLinksPage from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

const invitations = mockService(AdminInviteLinkService);
const link = 'https://chat.example/join/test-invitation';

describe('Invitation link copying', () => {
  beforeEach(() => {
    queryClient.clear();
    toast.clear();
    createTestServerScope({
      routes: (router) => router.service(AdminInviteLinkService, invitations)
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
});
