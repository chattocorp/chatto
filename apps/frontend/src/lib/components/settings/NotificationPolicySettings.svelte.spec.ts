import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { flushSync } from 'svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import { queryClient } from '$lib/query/client';
import { settingsQueryKeys } from '$lib/query/settings';
import {
  refreshRegisteredServerQueries,
  removeRegisteredServerQueries
} from '$lib/query/cacheRegistry';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  NotificationDeliveryMode,
  notificationPolicyScopeKey,
  type NotificationPolicyScope,
  type ScopedNotificationPolicy
} from '$lib/api-client/notifications';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    connection: { queryScope: 'test-0', getAPI: vi.fn() },
    batch: vi.fn(),
    update: vi.fn(),
    isCurrent: vi.fn(() => true)
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'test-server',
    connection: mocks.connection,
    isCurrent: mocks.isCurrent,
    store: {
      serverInfo: { name: 'Test Server' },
      navigation: {
        roomGroups: [{ id: 'group-1', name: 'Channels', roomIds: ['room-1', 'room-2'] }],
        rooms: [
          { id: 'room-1', name: 'general', viewerIsMember: true, type: 1 },
          { id: 'room-2', name: 'private', viewerIsMember: false, type: 1 },
          { id: 'dm-1', name: 'Taylor', viewerIsMember: true, type: 2 }
        ]
      }
    }
  })
}));

import NotificationPolicySettings from './NotificationPolicySettings.svelte';

function policy(
  scope: NotificationPolicyScope,
  directMentions = NotificationDeliveryMode.PUSH_NOTIFICATION
): ScopedNotificationPolicy {
  return {
    scope,
    overrides: {
      directMessages: null,
      roomMessages: null,
      directMentions: null,
      replies: null,
      roleMentions: null,
      hereMentions: null,
      allMentions: null,
      followedThreads: null,
      followedRooms: null,
      reactions: null
    },
    effective: {
      directMessages: NotificationDeliveryMode.PUSH_NOTIFICATION,
      roomMessages: NotificationDeliveryMode.UNREAD_BADGE,
      directMentions,
      replies: NotificationDeliveryMode.PUSH_NOTIFICATION,
      roleMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
      hereMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
      allMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
      followedThreads: NotificationDeliveryMode.IN_APP_NOTIFICATION,
      followedRooms: NotificationDeliveryMode.OFF,
      reactions: NotificationDeliveryMode.IN_APP_NOTIFICATION
    }
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe('NotificationPolicySettings', () => {
  let queryScope = 0;
  const scopes: NotificationPolicyScope[] = [
    { kind: 'server' },
    { kind: 'roomGroup', id: 'group-1' },
    { kind: 'room', id: 'room-1' },
    { kind: 'room', id: 'dm-1' }
  ];

  function seedPolicies(): void {
    queryClient.setQueryData(
      settingsQueryKeys.notificationPolicies(
        'test-server',
        mocks.connection,
        scopes.map(notificationPolicyScopeKey)
      ),
      Object.fromEntries(scopes.map((scope) => [notificationPolicyScopeKey(scope), policy(scope)]))
    );
  }

  beforeEach(async () => {
    vi.clearAllMocks();
    queryClient.clear();
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    mocks.connection.queryScope = `test-${++queryScope}`;
    mocks.connection.getAPI.mockImplementation(() => ({
      batchGetNotificationPolicies: mocks.batch,
      updateScopedNotificationPolicy: mocks.update
    }));
    mocks.batch.mockImplementation(async (requested: NotificationPolicyScope[]) =>
      requested.map((scope) => policy(scope))
    );
    mocks.update.mockImplementation(async (scope: NotificationPolicyScope) => policy(scope));
    seedPolicies();
  });

  it('renders all causes and orders member-visible scopes by inheritance context', async () => {
    const { container } = render(NotificationPolicySettings);

    await vi.waitFor(() => expect(mocks.batch).toHaveBeenCalled());
    expect(
      [...container.querySelectorAll('th[data-notification-scope]')].map((cell) =>
        cell.getAttribute('data-notification-scope')
      )
    ).toEqual(['server', 'roomGroup:group-1', 'room:room-1', 'room:dm-1']);
    expect(container.querySelector('th[data-notification-scope="room:room-2"]')).toBeNull();
    expect(container.querySelector('[data-notification-field="followedRooms"]')).toBeNull();
    expect(container.querySelectorAll('[data-matrix-row]')).toHaveLength(9 * 4);
    expect(container.textContent).not.toContain('Room invitations');
    expect(container.textContent).not.toContain('Followed rooms');
    expect(container.textContent).not.toContain('Reset to defaults');
  });

  it.each([390, 1280])('explains every activity from its row heading at %ipx', async (width) => {
    await page.viewport(width, 900);
    const { container } = render(NotificationPolicySettings);
    const helpButtons = container.querySelectorAll(
      'tbody th button[aria-label^="More information: "]'
    );

    expect(helpButtons).toHaveLength(9);
    expect(helpButtons[0]?.getAttribute('aria-label')).toBe('More information: Direct messages');
    (helpButtons[0] as HTMLButtonElement).dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();
    expect(container.querySelector('[role="tooltip"]')?.textContent?.trim()).toBe(
      'Messages that other members send in direct-message conversations.'
    );
    expect(container.querySelectorAll('td[data-notification-field] [title]')).toHaveLength(0);
    await page.viewport(1280, 900);
  });

  it('marks direct messages as not applicable to room groups and channel rooms', () => {
    const { container } = render(NotificationPolicySettings);
    const groupCell = container.querySelector(
      'td[data-notification-scope="roomGroup:group-1"][data-notification-field="directMessages"]'
    )!;
    const channelCell = container.querySelector(
      'td[data-notification-scope="room:room-1"][data-notification-field="directMessages"]'
    )!;
    const serverCell = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMessages"]'
    )!;
    const dmCell = container.querySelector(
      'td[data-notification-scope="room:dm-1"][data-notification-field="directMessages"]'
    )!;

    for (const [cell, scope] of [
      [groupCell, 'Channels'],
      [channelCell, 'general']
    ] as const) {
      expect(cell.querySelector('button')).toBeNull();
      const placeholder = cell.querySelector('[role="img"]')!;
      expect(placeholder.textContent?.trim()).toBe('—');
      expect(placeholder.getAttribute('aria-label')).toBe(
        `Direct messages, ${scope}. Not applicable.`
      );
    }
    expect(serverCell.querySelector('button')).not.toBeNull();
    expect(dmCell.querySelector('button')).not.toBeNull();
  });

  it('marks room messages as not applicable to direct-message rooms', () => {
    const { container } = render(NotificationPolicySettings);
    const serverCell = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="roomMessages"]'
    )!;
    const groupCell = container.querySelector(
      'td[data-notification-scope="roomGroup:group-1"][data-notification-field="roomMessages"]'
    )!;
    const channelCell = container.querySelector(
      'td[data-notification-scope="room:room-1"][data-notification-field="roomMessages"]'
    )!;
    const dmCell = container.querySelector(
      'td[data-notification-scope="room:dm-1"][data-notification-field="roomMessages"]'
    )!;

    expect(serverCell.querySelector('button')).not.toBeNull();
    expect(groupCell.querySelector('button')).not.toBeNull();
    expect(channelCell.querySelector('button')).not.toBeNull();
    expect(dmCell.querySelector('button')).toBeNull();
    expect(dmCell.querySelector('[role="img"]')?.getAttribute('aria-label')).toBe(
      'Room messages, Taylor. Not applicable.'
    );
  });

  it('highlights the active column heading together with its cells', () => {
    const { container } = render(NotificationPolicySettings);
    const cell = container.querySelector(
      'td[data-notification-scope="room:room-1"][data-notification-field="directMentions"]'
    ) as HTMLTableCellElement;
    const heading = container.querySelector(
      'th[data-notification-scope="room:room-1"] span[title]'
    ) as HTMLElement;

    expect(heading.className).toContain('text-muted');
    cell.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();

    expect(heading.className).toContain('text-action');
    expect(heading.className).not.toContain('text-muted');
  });

  it('retains a matched room group and cycles a server default to Off', async () => {
    const { container } = render(NotificationPolicySettings);
    const input = container.querySelector(
      '[data-testid="notification-scope-filter"] input, input[data-testid="notification-scope-filter"]'
    ) as HTMLInputElement;
    expect(input).not.toBeNull();
    input.value = 'general';
    input.dispatchEvent(new Event('input', { bubbles: true }));

    await vi.waitFor(() => {
      expect(
        [...container.querySelectorAll('th[data-notification-scope]')].map((cell) =>
          cell.getAttribute('data-notification-scope')
        )
      ).toEqual(['server', 'roomGroup:group-1', 'room:room-1']);
    });

    const button = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
    ) as HTMLButtonElement;
    button.click();
    expect(mocks.update).toHaveBeenCalledWith(
      { kind: 'server' },
      { directMessages: NotificationDeliveryMode.OFF }
    );
    expect(button.ariaLabel).toContain('Default: Push notification');
    expect(button.ariaLabel).toContain('Activate to set Off');
    expect(button.querySelector('[class~="icon-[uil--link]"]')).toBeNull();
  });

  it('explains the four delivery modes in the legend', () => {
    const { container } = render(NotificationPolicySettings);

    const legend = container.querySelector('[aria-label="Notification delivery modes"]');
    expect(legend?.textContent).toContain('Off');
    expect(legend?.textContent).toContain('Badge');
    expect(legend?.textContent).toContain('Notification');
    expect(legend?.textContent).toContain('Push notification');
    expect(legend?.querySelector('[class~="icon-[ph--bell-slash-fill]"]')).not.toBeNull();
    expect(legend?.querySelector('[class~="icon-[ph--bell-fill]"]')).not.toBeNull();
    expect(legend?.querySelector('[class~="icon-[ph--phone-fill]"]')).not.toBeNull();
    expect(legend?.querySelector('button[aria-label="More information: Badge"]')).not.toBeNull();
    expect(legend?.querySelector('[title*="neutral unread dot"]')).toBeNull();
    expect(legend?.textContent).not.toContain('Inherit');
    expect(legend?.querySelector('[class~="icon-[uil--link]"]')).toBeNull();
  });

  it('uses stable keys for every loaded scope', async () => {
    render(NotificationPolicySettings);
    await vi.waitFor(() => expect(mocks.batch).toHaveBeenCalled());
    const loaded = mocks.batch.mock.calls.at(-1)?.[0] as NotificationPolicyScope[];
    expect(loaded.map(notificationPolicyScopeKey)).toEqual([
      'server',
      'roomGroup:group-1',
      'room:room-1',
      'room:dm-1'
    ]);
  });

  it('keeps the new filter result when the previous read finishes later', async () => {
    queryClient.clear();
    const first = deferred<ScopedNotificationPolicy[]>();
    const second = deferred<ScopedNotificationPolicy[]>();
    mocks.batch.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { container } = render(NotificationPolicySettings);
    await vi.waitFor(() => expect(mocks.batch).toHaveBeenCalledTimes(1));

    const input = container.querySelector(
      '[data-testid="notification-scope-filter"] input, input[data-testid="notification-scope-filter"]'
    ) as HTMLInputElement;
    input.value = 'general';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await vi.waitFor(() => expect(mocks.batch).toHaveBeenCalledTimes(2));
    second.resolve(scopes.slice(0, 3).map((scope) => policy(scope, NotificationDeliveryMode.OFF)));
    await vi.waitFor(() =>
      expect(
        container
          .querySelector(
            'td[data-notification-scope="room:room-1"][data-notification-field="directMentions"] button'
          )
          ?.getAttribute('aria-label')
      ).toContain('Effective: Off')
    );

    first.resolve(scopes.map((scope) => policy(scope)));
    await first.promise;
    expect(
      container
        .querySelector(
          'td[data-notification-scope="room:room-1"][data-notification-field="directMentions"] button'
        )
        ?.getAttribute('aria-label')
    ).toContain('Effective: Off');
    await vi.waitFor(() =>
      expect(
        queryClient.getQueryCache().find({
          queryKey: settingsQueryKeys.notificationPolicies(
            'test-server',
            mocks.connection,
            scopes.map(notificationPolicyScopeKey)
          ),
          exact: true
        })
      ).toBeUndefined()
    );
  });

  it('refreshes inherited policy values after a server update', async () => {
    mocks.batch
      .mockImplementationOnce(async (requested: NotificationPolicyScope[]) =>
        requested.map((scope) => policy(scope))
      )
      .mockImplementation(async (requested: NotificationPolicyScope[]) =>
        requested.map((scope) => policy(scope, NotificationDeliveryMode.OFF))
      );
    mocks.update.mockResolvedValue(policy({ kind: 'server' }, NotificationDeliveryMode.OFF));
    const { container } = render(NotificationPolicySettings);
    await vi.waitFor(() => expect(mocks.batch).toHaveBeenCalledTimes(1));
    const button = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMentions"] button'
    ) as HTMLButtonElement;
    button.click();

    await vi.waitFor(() => expect(mocks.batch).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(
        queryClient.getQueryData<Record<string, ScopedNotificationPolicy>>(
          settingsQueryKeys.notificationPolicies(
            'test-server',
            mocks.connection,
            scopes.map(notificationPolicyScopeKey)
          )
        )?.['room:room-1']?.effective.directMentions
      ).toBe(NotificationDeliveryMode.OFF)
    );
  });

  it('renders per-cell loading placeholders while the visible scopes load', () => {
    queryClient.clear();
    mocks.batch.mockReturnValue(new Promise(() => undefined));

    const { container } = render(NotificationPolicySettings);

    expect(container.querySelectorAll('[class~="icon-[uil--spinner]"]')).toHaveLength(9 * 4 - 3);
    expect(container.querySelectorAll('td[data-notification-field] button')).toHaveLength(0);
    const placeholders = container.querySelectorAll(
      'td[data-notification-field] > span[role="status"]'
    );
    expect(placeholders).toHaveLength(9 * 4 - 3);
    expect([...placeholders].every((item) => item.textContent?.trim() === 'Loading...')).toBe(true);
  });

  it('keeps independent cells available while one update is pending', () => {
    const first = deferred<ScopedNotificationPolicy>();
    mocks.update.mockReturnValueOnce(first.promise);
    const { container } = render(NotificationPolicySettings);
    const pending = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
    ) as HTMLButtonElement;
    const available = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMentions"] button'
    ) as HTMLButtonElement;

    pending.click();
    flushSync();
    expect(pending.disabled).toBe(false);
    expect(pending.getAttribute('aria-disabled')).toBe('true');
    expect(pending.querySelector('[class~="icon-[uil--spinner]"]')).not.toBeNull();
    pending.click();
    expect(mocks.update).toHaveBeenCalledTimes(1);

    expect(available.getAttribute('aria-disabled')).toBeNull();
    available.click();
    expect(mocks.update).toHaveBeenCalledWith(
      { kind: 'server' },
      { directMentions: NotificationDeliveryMode.OFF }
    );
    first.resolve(policy({ kind: 'server' }));
  });

  it('shows localized load and save errors without hiding the matrix', async () => {
    queryClient.clear();
    mocks.batch.mockRejectedValue(
      new ConnectError('Policy service unavailable', Code.PermissionDenied)
    );
    const loadFailure = render(NotificationPolicySettings);

    await vi.waitFor(() =>
      expect(loadFailure.container.textContent).toContain('Policy service unavailable')
    );
    expect(loadFailure.container.querySelectorAll('[data-matrix-row]')).toHaveLength(9 * 4);
    loadFailure.unmount();

    queryClient.clear();
    seedPolicies();
    mocks.batch.mockImplementation(async (requested: NotificationPolicyScope[]) =>
      requested.map((scope) => policy(scope))
    );
    mocks.update.mockRejectedValue(new Error('Update was rejected'));
    const saveFailure = render(NotificationPolicySettings);
    const button = saveFailure.container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
    ) as HTMLButtonElement;
    button.click();

    await vi.waitFor(() =>
      expect(saveFailure.container.textContent).toContain(
        'Failed to save notification policy: Update was rejected'
      )
    );
    expect(
      saveFailure.container.querySelectorAll('td[data-notification-field] button')
    ).toHaveLength(9 * 4 - 3);
  });

  it('does not restore a saved policy after the server cache is removed', async () => {
    const save = deferred<ScopedNotificationPolicy>();
    mocks.update.mockReturnValue(save.promise);
    const { container } = render(NotificationPolicySettings);
    const button = container.querySelector(
      'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
    ) as HTMLButtonElement;
    button.click();
    expect(mocks.update).toHaveBeenCalledTimes(1);

    removeRegisteredServerQueries('test-server');
    save.resolve(policy({ kind: 'server' }));
    await save.promise;
    expect(
      queryClient.getQueriesData({
        queryKey: settingsQueryKeys.notificationPoliciesRoot('test-server', mocks.connection)
      })
    ).toEqual([]);
  });

  it('keeps a new save pending when an old save finishes after a privacy refresh', async () => {
    const oldSave = deferred<ScopedNotificationPolicy>();
    const newSave = deferred<ScopedNotificationPolicy>();
    mocks.update.mockReturnValueOnce(oldSave.promise).mockReturnValueOnce(newSave.promise);
    const { container } = render(NotificationPolicySettings);
    const button = () =>
      container.querySelector(
        'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
      ) as HTMLButtonElement;

    button().click();
    await refreshRegisteredServerQueries('test-server');
    await vi.waitFor(() => expect(button()).not.toBeNull());
    button().click();
    expect(mocks.update).toHaveBeenCalledTimes(2);

    oldSave.resolve(policy({ kind: 'server' }));
    await oldSave.promise;
    expect(button().getAttribute('aria-disabled')).toBe('true');
    newSave.resolve(policy({ kind: 'server' }));
  });
});
