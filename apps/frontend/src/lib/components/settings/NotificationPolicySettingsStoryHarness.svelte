<script lang="ts">
  import { SvelteMap } from 'svelte/reactivity';
  import { RoomKind } from '$lib/api-client/roomDirectory';
  import {
    NotificationDeliveryMode,
    notificationPolicyScopeKey,
    type NotificationAPI,
    type NotificationPolicyField,
    type NotificationPolicyModes,
    type NotificationPolicyOverrides,
    type NotificationPolicyPatch,
    type NotificationPolicyScope,
    type ScopedNotificationPolicy
  } from '$lib/api-client/notifications';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { NotificationPolicyMatrixState } from '$lib/state/server/notificationPolicies.svelte';
  import type { ServerStateStore } from '$lib/state/server/store.svelte';
  import NotificationPolicySettings from './NotificationPolicySettings.svelte';

  let { loadFailure = false }: { loadFailure?: boolean } = $props();

  const fields: NotificationPolicyField[] = [
    'directMessages',
    'directMentions',
    'replies',
    'roleMentions',
    'hereMentions',
    'allMentions',
    'followedThreads',
    'followedRooms',
    'reactions'
  ];
  const productDefaults: NotificationPolicyModes = {
    directMessages: NotificationDeliveryMode.PUSH_NOTIFICATION,
    directMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
    replies: NotificationDeliveryMode.IN_APP_NOTIFICATION,
    roleMentions: NotificationDeliveryMode.IN_APP_NOTIFICATION,
    hereMentions: NotificationDeliveryMode.OFF,
    allMentions: NotificationDeliveryMode.OFF,
    followedThreads: NotificationDeliveryMode.IN_APP_NOTIFICATION,
    followedRooms: NotificationDeliveryMode.OFF,
    reactions: NotificationDeliveryMode.IN_APP_NOTIFICATION
  };
  const groupForRoom: Record<string, string> = {
    general: 'community',
    support: 'community'
  };
  const overrides = new SvelteMap<string, NotificationPolicyOverrides>();

  function emptyOverrides(): NotificationPolicyOverrides {
    return Object.fromEntries(fields.map((field) => [field, null])) as NotificationPolicyOverrides;
  }

  function configuredOverrides(patch: NotificationPolicyPatch = {}): NotificationPolicyOverrides {
    return { ...emptyOverrides(), ...patch };
  }

  overrides.set(
    'server',
    configuredOverrides({ roleMentions: NotificationDeliveryMode.PUSH_NOTIFICATION })
  );
  overrides.set(
    'roomGroup:community',
    configuredOverrides({
      directMentions: NotificationDeliveryMode.OFF,
      followedThreads: NotificationDeliveryMode.PUSH_NOTIFICATION
    })
  );
  overrides.set(
    'room:general',
    configuredOverrides({ replies: NotificationDeliveryMode.PUSH_NOTIFICATION })
  );
  overrides.set('room:dm-alex', configuredOverrides({ reactions: NotificationDeliveryMode.OFF }));

  function applyOverrides(
    effective: NotificationPolicyModes,
    values: NotificationPolicyOverrides | undefined
  ): NotificationPolicyModes {
    const next = { ...effective };
    if (!values) return next;
    for (const field of fields) {
      if (values[field] !== null) next[field] = values[field];
    }
    return next;
  }

  function resolvePolicy(scope: NotificationPolicyScope): ScopedNotificationPolicy {
    let effective = applyOverrides(productDefaults, overrides.get('server'));
    if (scope.kind === 'roomGroup') {
      effective = applyOverrides(effective, overrides.get(notificationPolicyScopeKey(scope)));
    } else if (scope.kind === 'room') {
      const groupID = groupForRoom[scope.id];
      if (groupID) {
        effective = applyOverrides(effective, overrides.get(`roomGroup:${groupID}`));
      }
      effective = applyOverrides(effective, overrides.get(notificationPolicyScopeKey(scope)));
    }
    return {
      scope,
      overrides: overrides.get(notificationPolicyScopeKey(scope)) ?? emptyOverrides(),
      effective
    };
  }

  const api = {
    async batchGetNotificationPolicies(scopes: NotificationPolicyScope[]) {
      if (loadFailure) throw new Error('Example policy service is unavailable');
      return scopes.map(resolvePolicy);
    },
    async updateScopedNotificationPolicy(
      scope: NotificationPolicyScope,
      patch: NotificationPolicyPatch
    ) {
      const key = notificationPolicyScopeKey(scope);
      overrides.set(key, { ...(overrides.get(key) ?? emptyOverrides()), ...patch });
      return resolvePolicy(scope);
    }
  } as unknown as NotificationAPI;

  const matrixState = new NotificationPolicyMatrixState(api);
  provideServerScope({
    serverId: 'storybook',
    connection: {} as ServerConnection,
    store: {
      notifications: { notificationPolicies: matrixState },
      serverInfo: { name: 'Example server' },
      navigation: {
        roomGroups: [
          {
            id: 'community',
            name: 'Community',
            roomIds: ['general', 'support']
          }
        ],
        rooms: [
          { id: 'general', name: 'general', viewerIsMember: true, type: RoomKind.CHANNEL },
          { id: 'support', name: 'support', viewerIsMember: true, type: RoomKind.CHANNEL },
          { id: 'staff', name: 'staff', viewerIsMember: false, type: RoomKind.CHANNEL },
          { id: 'dm-alex', name: 'Alex', viewerIsMember: true, type: RoomKind.DM }
        ]
      }
    } as unknown as ServerStateStore,
    isCurrent: () => true
  });
</script>

<NotificationPolicySettings />
