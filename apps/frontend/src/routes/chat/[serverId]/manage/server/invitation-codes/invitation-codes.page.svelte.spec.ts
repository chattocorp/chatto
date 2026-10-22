import { beforeAll } from 'vitest';
import { loadLocaleMessages } from '$lib/i18n/messages';

// Standalone management components do not run the route catalog loader.
beforeAll(() => loadLocaleMessages('en-GB'));

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { Timestamp } from '@bufbuild/protobuf';
import { AdminInvitationCodeService } from '@chatto/api-types/admin/v1/invitations_connect';
import { InvitationCodeStatus } from '@chatto/api-types/admin/v1/invitations_pb';
import { queryClient } from '$lib/query/client';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import InvitationCodesPage from './+page.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

const invitations = mockService(AdminInvitationCodeService);
const code = 'ABCD-EFGH-IJKL-MNOP-QRST';

describe('Invitation code copying', () => {
  beforeEach(() => {
    queryClient.clear();
    toast.clear();
    createTestServerScope({
      routes: (router) => router.service(AdminInvitationCodeService, invitations)
    });
    invitations.listInvitationCodes.mockReturnValue({
      invitationCodes: [
        {
          id: 'invitation-1',
          code,
          status: InvitationCodeStatus.ACTIVE,
          createdAt: Timestamp.fromDate(new Date('2026-01-01'))
        }
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
    const { container } = render(InvitationCodesPage);
    const copyButton = () =>
      [...container.querySelectorAll('button')].find(
        (button) => button.textContent?.trim() === 'Copy code'
      );
    await vi.waitFor(() => expect(copyButton()).toBeDefined());

    await userEvent.click(copyButton()!);

    expect(getToasts()).toMatchObject([{ tone: 'error', message: 'Failed to copy code' }]);
    toast.clear();

    await userEvent.click(copyButton()!);

    expect(writeText).toHaveBeenNthCalledWith(2, code);
    expect(getToasts()).toMatchObject([{ tone: 'success', message: 'Invitation code copied' }]);
  });
});
