import { describe, expect, it, vi } from 'vitest';
import type { ViewerState } from '$lib/api-client/viewer';
import { NotificationLevel, PresenceStatus } from '$lib/api-client/renderTypes';
import { ViewerStateStore } from './viewer.svelte';

function viewer(displayName = 'Alice'): ViewerState {
  return {
    user: {
      id: 'U1',
      login: 'alice',
      displayName,
      presenceStatus: PresenceStatus.Online,
      hasVerifiedEmail: true,
      hasPassword: true,
      viewerCanDeleteAccount: true
    },
    canViewAdmin: false,
    canStartDMs: true,
    canAdminViewUsers: false,
    canAdminManageAccounts: false,
    canAssignRoles: false,
    canAdminViewRoles: false,
    canAdminManageRoles: false,
    canAdminViewSystem: false,
    canAdminViewAudit: false,
    canManageUserPermissions: false,
    viewerPermissions: {},
    viewerHasUnreadRooms: false,
    viewerHasUnreadFollowedThreads: false,
    viewerHasPendingFollowedThreadNotifications: false,
    serverNotificationPreference: {
      level: NotificationLevel.Default,
      effectiveLevel: NotificationLevel.Normal
    },
    roomNotificationPreferences: []
  };
}

describe('ViewerStateStore', () => {
  it('shares one request across concurrent consumers and caches the result', async () => {
    let resolve!: (value: ViewerState) => void;
    const loader = vi.fn(
      () =>
        new Promise<ViewerState>((done) => {
          resolve = done;
        })
    );
    const store = new ViewerStateStore(
      { baseUrl: '/api/connect', bearerToken: null },
      loader
    );

    const first = store.load();
    const second = store.load();
    expect(loader).toHaveBeenCalledOnce();

    resolve(viewer());
    await expect(Promise.all([first, second])).resolves.toHaveLength(2);
    await expect(store.load()).resolves.toEqual(viewer());
    expect(loader).toHaveBeenCalledOnce();
  });

  it('supports explicit seeding and refresh', async () => {
    const onUpdate = vi.fn();
    const loader = vi.fn().mockResolvedValue(viewer('Fresh'));
    const store = new ViewerStateStore(
      { baseUrl: '/api/connect', bearerToken: null },
      loader,
      onUpdate
    );

    store.seed(viewer('Seeded'));
    await expect(store.load()).resolves.toEqual(viewer('Seeded'));
    await expect(store.refresh()).resolves.toEqual(viewer('Fresh'));

    expect(loader).toHaveBeenCalledOnce();
    expect(onUpdate).toHaveBeenLastCalledWith(viewer('Fresh'));
  });
});
