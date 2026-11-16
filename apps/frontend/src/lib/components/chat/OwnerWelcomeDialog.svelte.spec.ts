import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { markOwnerOnboardingPending, takeOwnerOnboarding } from '$lib/state/ownerOnboarding';
import OwnerWelcomeDialog from './OwnerWelcomeDialog.svelte';

const mocks = vi.hoisted(() => ({
  isOrigin: true,
  setPrivilegedMode: vi.fn(),
  viewer: null as { privilegedMode: { available: boolean; active: boolean } } | null
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
    mocks.viewer = { privilegedMode: { available: true, active: false } };
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
    expect(mocks.setPrivilegedMode).toHaveBeenCalledExactlyOnceWith(true);
    await expect
      .element(view.getByRole('link', { name: 'Rooms' }))
      .toHaveAttribute('href', '/chat/-/manage/rooms');
    expect(takeOwnerOnboarding()).toBe(false);
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
