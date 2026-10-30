import { beforeAll } from 'vitest';
import { loadLocaleMessages } from '$lib/i18n/messages';

// Standalone management components do not run the route catalog loader.
beforeAll(() => loadLocaleMessages('en-GB'));

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { signal } from '@chatto/client/reactivity';
import { render } from 'vitest-browser-svelte';
import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { AdminInvitationCodeService } from '@chatto/api-types/admin/v1/invitations_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { getUserStore } from '@chatto/client/server/users';
import { InvitationCodeStatus } from '@chatto/api-types/admin/v1/invitations_pb';
import { queryClient } from '$lib/query/client';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import InvitationCodes from './InvitationCodes.svelte';

const route = vi.hoisted(() => ({
  state: null as unknown as ReturnType<typeof signal<App.PageState>>
}));
vi.mock('$app/state', () => ({
  page: {
    get state() {
      return route.state.get();
    }
  }
}));
vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: (_url: string, state: App.PageState) => route.state.set(state)
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

const invitations = mockService(AdminInvitationCodeService);
const code = 'ABCD-EFGH-IJKL-MNOP-QRST';

describe('Invitation codes', () => {
  it('hydrates creators in a batch and displays their names', async () => {
    const profiles = mockService(UserService);
    profiles.batchGetUsers.mockReturnValue({
      users: [{ user: { id: 'creator', displayName: 'Bob', login: 'bob' } }]
    });
    const users = getUserStore('server-1', 'server-1-session');
    users.clear();
    createTestServerScope({
      routes: (router) =>
        router.service(AdminInvitationCodeService, invitations).service(UserService, profiles),
      store: { projection: { users } }
    });
    invitations.listInvitationCodes.mockReturnValue({
      invitationCodes: [
        {
          id: 'first',
          code,
          status: InvitationCodeStatus.ACTIVE,
          createdBy: 'creator',
          createdAt: Timestamp.fromDate(new Date('2026-01-01'))
        },
        {
          id: 'second',
          code,
          status: InvitationCodeStatus.ACTIVE,
          createdBy: 'creator',
          createdAt: Timestamp.fromDate(new Date('2026-01-01'))
        }
      ]
    });
    const { container } = render(InvitationCodes);
    await vi.waitFor(() =>
      expect(
        [...container.querySelectorAll('[data-testid="user-identity"] bdi')].filter(
          (node) => node.textContent === 'Bob'
        )
      ).toHaveLength(2)
    );
    expect(profiles.batchGetUsers).toHaveBeenCalledOnce();
    expect(profiles.batchGetUsers.mock.calls[0][0].userIds).toEqual(['creator']);
    expect(container.querySelectorAll('[data-testid="user-identity"]')).toHaveLength(2);
    expect(container.querySelectorAll('thead th')).toHaveLength(5);
    expect(
      container.querySelector('tbody tr td:nth-child(3) [data-testid="user-identity"]')
    ).not.toBeNull();
    users.clear();
  });

  beforeEach(() => {
    vi.clearAllMocks();
    route.state = signal<App.PageState>({});
    vi.spyOn(history, 'back').mockImplementation(() => route.state.set({}));
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
          note: 'For the design team <not markup>',
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
    const { container } = render(InvitationCodes);
    const copyButton = () =>
      [...container.querySelectorAll('button')].find(
        (button) => button.getAttribute('aria-label') === 'Copy to clipboard'
      );
    await vi.waitFor(() => expect(copyButton()).toBeDefined());

    await userEvent.click(copyButton()!);

    expect(getToasts()).toMatchObject([
      {
        tone: 'error',
        message: 'Could not copy to clipboard. Select the text and copy it manually.'
      }
    ]);
    toast.clear();

    await userEvent.click(copyButton()!);

    expect(writeText).toHaveBeenNthCalledWith(2, code);
    expect(getToasts()).toMatchObject([{ tone: 'success', message: 'Copied to clipboard' }]);
  });

  it('renders notes as plain text beneath their codes', async () => {
    const page = render(InvitationCodes);
    await expect
      .element(page.getByText('For the design team <not markup>', { exact: true }))
      .toBeVisible();
    expect(page.container.querySelector('not')).toBeNull();
  });

  it('requires confirmation before creating one code with a seven-day expiry', async () => {
    invitations.createInvitationCode.mockReturnValue({
      invitationCode: {
        id: 'new-invitation',
        code,
        status: InvitationCodeStatus.ACTIVE,
        createdAt: Timestamp.fromDate(new Date())
      }
    });
    const page = render(InvitationCodes);
    await expect.element(page.getByText(/New codes expire after 7 days/)).not.toBeInTheDocument();
    const before = Date.now();
    await userEvent.click(page.getByRole('button', { name: 'Create code', exact: true }));
    const dialog = page.getByRole('dialog', { name: 'Create code', exact: true });
    await expect.element(dialog).toBeVisible();
    await expect.element(dialog.getByText(/New codes expire after 7 days/)).toBeVisible();
    expect(invitations.createInvitationCode).not.toHaveBeenCalled();
    await userEvent.fill(
      dialog.getByRole('textbox', { name: 'Note (optional)' }),
      'For Alex from the design team'
    );
    await userEvent.click(dialog.getByRole('button', { name: 'Create code', exact: true }));
    await vi.waitFor(() => expect(invitations.createInvitationCode).toHaveBeenCalledOnce());
    const expiresAt = invitations.createInvitationCode.mock.calls[0][0]
      .expiresAt!.toDate()
      .getTime();
    const week = 7 * 24 * 60 * 60 * 1000;
    expect(expiresAt).toBeGreaterThanOrEqual(before + week);
    expect(expiresAt).toBeLessThanOrEqual(Date.now() + week);
    expect(invitations.createInvitationCode.mock.calls[0][0].note).toBe(
      'For Alex from the design team'
    );
    await expect.element(dialog).not.toBeInTheDocument();
    await expect.element(page.getByLabelText('Expiry')).not.toBeInTheDocument();
  });

  it('does not create a code when the dialog is cancelled', async () => {
    const page = render(InvitationCodes);
    await userEvent.click(page.getByRole('button', { name: 'Create code', exact: true }));
    const dialog = page.getByRole('dialog', { name: 'Create code', exact: true });
    await userEvent.fill(
      dialog.getByRole('textbox', { name: 'Note (optional)' }),
      'Discard this draft'
    );
    await userEvent.click(dialog.getByRole('button', { name: 'Cancel', exact: true }));
    await expect.element(dialog).not.toBeInTheDocument();
    expect(invitations.createInvitationCode).not.toHaveBeenCalled();
    await userEvent.click(page.getByRole('button', { name: 'Create code', exact: true }));
    await expect.element(page.getByRole('textbox', { name: 'Note (optional)' })).toHaveValue('');
  });

  it('counts Unicode characters and retains the note after a failed request', async () => {
    invitations.createInvitationCode.mockRejectedValue(
      new ConnectError('Unavailable', Code.Unavailable)
    );
    const page = render(InvitationCodes);
    await userEvent.click(page.getByRole('button', { name: 'Create code', exact: true }));
    const dialog = page.getByRole('dialog', { name: 'Create code', exact: true });
    const input = dialog.getByRole('textbox', { name: 'Note (optional)' });
    const create = dialog.getByRole('button', { name: 'Create code', exact: true });
    await userEvent.fill(input, '🌿'.repeat(251));
    await expect.element(create).toBeDisabled();
    await expect.element(dialog.getByText('Use 250 characters or fewer.')).toBeVisible();
    expect(invitations.createInvitationCode).not.toHaveBeenCalled();
    const note = '🌿'.repeat(250);
    await userEvent.fill(input, note);
    await expect.element(create).toBeEnabled();
    await userEvent.click(create);
    await vi.waitFor(() => expect(invitations.createInvitationCode).toHaveBeenCalledOnce());
    await expect.element(create).toBeEnabled();
    await expect.element(input).toHaveValue(note);
    expect(invitations.createInvitationCode.mock.calls[0][0].note).toBe(note);
  });

  it('blocks repeated submissions while creation is pending', async () => {
    let finish!: () => void;
    invitations.createInvitationCode.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = () =>
            resolve({
              invitationCode: {
                id: 'new-invitation',
                code,
                status: InvitationCodeStatus.ACTIVE,
                createdAt: Timestamp.fromDate(new Date())
              }
            });
        })
    );
    const page = render(InvitationCodes);
    await userEvent.click(page.getByRole('button', { name: 'Create code', exact: true }));
    const dialog = page.getByRole('dialog', { name: 'Create code', exact: true });
    await userEvent.click(dialog.getByRole('button', { name: 'Create code', exact: true }));
    await vi.waitFor(() => expect(invitations.createInvitationCode).toHaveBeenCalledOnce());
    await expect.element(dialog.getByRole('button', { name: /Create code/ })).toBeDisabled();
    dialog
      .element()
      .querySelector('form')!
      .dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));
    expect(invitations.createInvitationCode).toHaveBeenCalledOnce();
    finish();
    await expect.element(dialog).not.toBeInTheDocument();
  });

  it('shows inactive codes without copy or revoke actions', async () => {
    invitations.listInvitationCodes.mockReturnValue({
      invitationCodes: [
        InvitationCodeStatus.USED,
        InvitationCodeStatus.EXPIRED,
        InvitationCodeStatus.REVOKED
      ].map((status, index) => ({
        id: `inactive-${index}`,
        code,
        status,
        createdAt: Timestamp.fromDate(new Date('2026-01-01'))
      })),
      page: { totalCount: 3n, hasMore: false }
    });
    const page = render(InvitationCodes);
    await expect.element(page.getByText('Used', { exact: true })).toBeVisible();
    await expect.element(page.getByText('Expired', { exact: true })).toBeVisible();
    await expect.element(page.getByText('Revoked', { exact: true })).toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Copy to clipboard', exact: true }))
      .not.toBeInTheDocument();
    await expect
      .element(page.getByRole('button', { name: 'Revoke', exact: true }))
      .not.toBeInTheDocument();
  });
});
