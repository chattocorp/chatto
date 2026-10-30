import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { signal } from '@chatto/client/reactivity';
import { InvitationCodeService } from '@chatto/api-types/api/v1/invitations_connect';
import { Code, ConnectError } from '@connectrpc/connect';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { mockService } from '$lib/test-utils';
import InvitePeopleControl from './InvitePeopleControl.svelte';

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

const invitations = mockService(InvitationCodeService);
const code = 'ABCD-EFGH-IJKL-MNOP-QRST';
let scope: ReturnType<typeof createTestServerScope>;

beforeEach(() => {
  vi.clearAllMocks();
  route.state = signal<App.PageState>({});
  vi.spyOn(history, 'back').mockImplementation(() => route.state.set({}));
  scope = createTestServerScope({
    permissions: { canInviteUsers: true },
    serverInfo: { accountCreationPolicy: 'invite_only' },
    routes: (router) => router.service(InvitationCodeService, invitations)
  });
  invitations.createInvitationCode.mockReturnValue({
    invitationCode: { id: 'invitation-1', code }
  });
});
afterEach(() => vi.restoreAllMocks());

describe('member invitation codes', () => {
  it('hides issuance for open or unknown registration and discards a code when registration opens', async () => {
    const policy = signal<'open' | 'invite_only' | null>('open');
    scope = createTestServerScope({
      permissions: { canInviteUsers: true },
      serverInfo: {
        get accountCreationPolicy() {
          return policy.get();
        }
      },
      routes: (router) => router.service(InvitationCodeService, invitations)
    });
    const view = render(InvitePeopleControl);
    await expect
      .element(view.getByRole('button', { name: 'Invite people' }))
      .not.toBeInTheDocument();
    policy.set(null);
    await expect
      .element(view.getByRole('button', { name: 'Invite people' }))
      .not.toBeInTheDocument();
    policy.set('invite_only');
    await userEvent.click(view.getByRole('button', { name: 'Invite people' }));
    await userEvent.click(view.getByRole('button', { name: 'Create code' }));
    await expect.element(view.getByText(code)).toBeVisible();
    policy.set('open');
    await expect.element(view.getByRole('dialog')).not.toBeInTheDocument();
    policy.set('invite_only');
    await expect.element(view.getByRole('button', { name: 'Create code' })).toBeVisible();
    await expect.element(view.getByText(code)).not.toBeInTheDocument();
  });

  it('creates and copies a code without admin access, then forgets it on Back and Forward', async () => {
    const view = render(InvitePeopleControl);
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    await userEvent.click(view.getByRole('button', { name: 'Invite people' }));
    const modalState = route.state.get();
    expect(invitations.createInvitationCode).not.toHaveBeenCalled();
    await userEvent.click(view.getByRole('button', { name: 'Create code' }));
    await expect.element(view.getByText(code)).toBeVisible();
    expect(route.state.get()).toEqual(modalState);
    await userEvent.click(view.getByRole('button', { name: 'Copy to clipboard' }));
    expect(clipboard).toHaveBeenCalledWith(code);
    history.back();
    await expect.element(view.getByRole('dialog')).not.toBeInTheDocument();
    route.state.set(modalState);
    await expect.element(view.getByRole('button', { name: 'Create code' })).toBeVisible();
    await expect.element(view.getByText(code)).not.toBeInTheDocument();
    expect(invitations.createInvitationCode).toHaveBeenCalledOnce();
  });

  it('hides the action without issuance permission and removes a code on permission loss', async () => {
    const view = render(InvitePeopleControl);
    await userEvent.click(view.getByRole('button', { name: 'Invite people' }));
    await userEvent.click(view.getByRole('button', { name: 'Create code' }));
    await expect.element(view.getByText(code)).toBeVisible();
    scope.permissions.canInviteUsers = false;
    await expect.element(view.getByRole('dialog')).not.toBeInTheDocument();
    await expect
      .element(view.getByRole('button', { name: 'Invite people' }))
      .not.toBeInTheDocument();
    scope.permissions.canInviteUsers = true;
    await expect.element(view.getByRole('button', { name: 'Create code' })).toBeVisible();
    await expect.element(view.getByText(code)).not.toBeInTheDocument();
  });

  it('does not retry a failed creation automatically', async () => {
    invitations.createInvitationCode.mockRejectedValue(
      new ConnectError('Unavailable', Code.Unavailable)
    );
    const view = render(InvitePeopleControl);
    await userEvent.click(view.getByRole('button', { name: 'Invite people' }));
    await userEvent.click(view.getByRole('button', { name: 'Create code' }));
    await expect.element(view.getByRole('button', { name: 'Create code' })).toBeEnabled();
    expect(invitations.createInvitationCode).toHaveBeenCalledOnce();
    await expect.element(view.getByText(code)).not.toBeInTheDocument();
  });

  it('discards a late result after dismissal and prevents a second pending request', async () => {
    let finish!: () => void;
    invitations.createInvitationCode.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = () => resolve({ invitationCode: { id: 'late', code } });
        })
    );
    const view = render(InvitePeopleControl);
    await userEvent.click(view.getByRole('button', { name: 'Invite people' }));
    await userEvent.click(view.getByRole('button', { name: 'Create code' }));
    await vi.waitFor(() => expect(invitations.createInvitationCode).toHaveBeenCalledOnce());
    await expect.element(view.getByRole('button', { name: 'Create code' })).toBeDisabled();
    history.back();
    await expect.element(view.getByRole('dialog')).not.toBeInTheDocument();
    finish();
    await userEvent.click(view.getByRole('button', { name: 'Invite people' }));
    await expect.element(view.getByText(code)).not.toBeInTheDocument();
    expect(invitations.createInvitationCode).toHaveBeenCalledOnce();
  });
});
