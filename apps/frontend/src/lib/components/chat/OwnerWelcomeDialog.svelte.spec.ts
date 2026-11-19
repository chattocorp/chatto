import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { markOwnerOnboardingPending, takeOwnerOnboarding } from '$lib/state/ownerOnboarding';
import OwnerWelcomeDialog from './OwnerWelcomeDialog.svelte';

const mocks = vi.hoisted(() => ({
  isOrigin: true,
  setPrivilegedMode: vi.fn(),
  viewer: null as {
    privilegedMode: { availableRoles?: string[]; available: boolean; active: boolean };
  } | null
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: { isOriginServer: () => mocks.isOrigin, getServer: () => undefined }
}));

describe('OwnerWelcomeDialog', () => {
  beforeEach(() => {
    takeOwnerOnboarding();
    mocks.isOrigin = true;
    mocks.viewer = {
      privilegedMode: { availableRoles: ['owner', 'admin'], available: true, active: false }
    };
    mocks.setPrivilegedMode.mockReset().mockResolvedValue(undefined);
    createTestServerScope({
      serverId: 'origin',
      store: {
        projection: {
          get viewer() {
            return mocks.viewer;
          }
        },
        setPrivilegedMode: mocks.setPrivilegedMode
      }
    });
  });

  it('welcomes the new owner once and activates Shield Mode', async () => {
    markOwnerOnboardingPending();
    const view = render(OwnerWelcomeDialog);
    await expect
      .element(view.getByRole('dialog', { name: 'Your server is ready' }))
      .toBeInTheDocument();
    expect(mocks.setPrivilegedMode).toHaveBeenCalledExactlyOnceWith(true, ['owner']);
    await expect
      .element(view.getByRole('link', { name: 'Rooms' }))
      .toHaveAttribute('href', '/chat/-/manage/rooms');
    expect(takeOwnerOnboarding()).toBe(false);
  });

  it('uses direct-grant activation with legacy fallback when the server reports no roles', async () => {
    mocks.viewer = { privilegedMode: { available: true, active: false } };
    markOwnerOnboardingPending();
    render(OwnerWelcomeDialog);
    await vi.waitFor(() =>
      expect(mocks.setPrivilegedMode).toHaveBeenCalledExactlyOnceWith(true, [])
    );
  });

  it('shows no Shield Mode hint while activation is still running', async () => {
    mocks.setPrivilegedMode.mockReturnValue(new Promise(() => {}));
    markOwnerOnboardingPending();
    const view = render(OwnerWelcomeDialog);
    await expect
      .element(view.getByRole('dialog', { name: 'Your server is ready' }))
      .toBeInTheDocument();
    await expect.element(view.getByText(/shield button/)).not.toBeInTheDocument();
  });

  it('tells the owner to use the shield button when activation fails', async () => {
    mocks.setPrivilegedMode.mockRejectedValue(new Error('unavailable'));
    markOwnerOnboardingPending();
    const view = render(OwnerWelcomeDialog);
    await expect
      .element(view.getByText(/To set up your server, activate Shield Mode/))
      .toBeInTheDocument();
  });

  it('closes when the owner follows a next step', async () => {
    markOwnerOnboardingPending();
    const view = render(OwnerWelcomeDialog);
    const link = view.getByRole('link', { name: 'Invites' });
    link.element().addEventListener('click', (event) => event.preventDefault());
    await link.click();
    await expect
      .element(view.getByRole('dialog', { name: 'Your server is ready' }))
      .not.toBeInTheDocument();
  });

  it('stays hidden without a pending setup hand-off', async () => {
    const view = render(OwnerWelcomeDialog);
    await expect.element(view.getByRole('dialog')).not.toBeInTheDocument();
    expect(mocks.setPrivilegedMode).not.toHaveBeenCalled();
  });

  it('leaves the hand-off pending on other servers', async () => {
    mocks.isOrigin = false;
    markOwnerOnboardingPending();
    const view = render(OwnerWelcomeDialog);
    await expect.element(view.getByRole('dialog')).not.toBeInTheDocument();
    expect(takeOwnerOnboarding()).toBe(true);
  });
});
