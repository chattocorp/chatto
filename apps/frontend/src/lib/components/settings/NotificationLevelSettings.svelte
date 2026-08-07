<!--
@component

Server-wide and per-room notification level settings for the current user.
These preferences are server-side and sync across devices.
-->
<script module lang="ts">
  let nextNotificationSettingsSnapshotVersion = 0;
</script>

<script lang="ts">
  import { NotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { notificationLevelOrDefault } from '$lib/api-client/enumDefaults';
  import { onDestroy, untrack } from 'svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

  import { ChoiceRow, FormSection } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import {
    getServerNotificationPreference,
    updateRoomNotificationPreference,
    updateServerNotificationPreference
  } from '$lib/api-client/notificationPreferences';
  import { createRoomDirectoryAPI, RoomDirectoryScope } from '$lib/api-client/roomDirectory';
  import { getViewerStateViaConnect } from '$lib/api-client/viewer';
  import { registerServerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';

  const serverScope = useServerScope();
  const notificationLevelStore = $derived(serverScope.store.notificationLevels);
  let componentActive = true;
  let privacyGeneration = 0;
  const removeCacheRemovalListener = registerServerQueryCacheRemovalListener((serverId) => {
    if (serverId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    componentActive = false;
    privacyGeneration += 1;
    removeCacheRemovalListener();
  });

  type NotificationPreference = {
    level: NotificationLevel;
    effectiveLevel: NotificationLevel;
  };
  type NotificationSettingsRoom = NotificationPreference & { id: string; name: string };
  type NotificationSettingsSnapshot = {
    version: number;
    serverPreference: NotificationPreference;
    rooms: NotificationSettingsRoom[];
  };
  type NotificationMutationScope = {
    serverId: string;
    connection: ServerConnection;
    queryKey: ReturnType<typeof settingsQueryKeys.notificationPreferences>;
    privacyGeneration: number;
  };
  type ServerPreferenceVariables = NotificationMutationScope & { level: NotificationLevel };
  type RoomPreferenceVariables = NotificationMutationScope & {
    roomId: string;
    level: NotificationLevel;
  };

  const preferencesQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: settingsQueryKeys.notificationPreferences(serverId, connection),
        queryFn: ({ signal }) => loadNotificationSettings(serverId, connection, signal),
        refetchOnMount: 'always' as const
      };
    },
    () => queryClient
  );
  let synchronizedSnapshotVersion = untrack(() => preferencesQuery.data?.version ?? 0);

  const snapshot = $derived(preferencesQuery.data ?? null);
  const serverLevel = $derived(
    snapshot?.serverPreference.level === NotificationLevel.DEFAULT
      ? NotificationLevel.NORMAL
      : (snapshot?.serverPreference.level ?? NotificationLevel.NORMAL)
  );
  const serverEffectiveLevel = $derived(
    snapshot?.serverPreference.effectiveLevel ?? NotificationLevel.NORMAL
  );
  const rooms = $derived(snapshot?.rooms ?? []);
  const loading = $derived(preferencesQuery.isPending && snapshot === null);
  const error = $derived.by(() => {
    const queryError = preferencesQuery.error;
    if (!queryError) return '';
    return queryError instanceof Error
      ? queryError.message
      : m('settings.notifications.levels.load_failed');
  });

  // The query owns the bounded settings snapshot; the realtime store remains the shared
  // rendering owner. Never regress it from a cached mount snapshot: synchronize only after
  // this observer has received an authoritative response or mutation update.
  $effect(() => {
    const current = snapshot;
    if (!current || preferencesQuery.isError || current.version === synchronizedSnapshotVersion) {
      return;
    }
    synchronizedSnapshotVersion = current.version;
    notificationLevelStore.setServerPreference(
      current.serverPreference.level,
      current.serverPreference.effectiveLevel
    );
    for (const room of current.rooms) {
      notificationLevelStore.setRoomPreference(room.id, room.level, room.effectiveLevel);
    }
  });

  async function loadNotificationSettings(
    serverId: string,
    connection: ServerConnection,
    signal: AbortSignal
  ): Promise<NotificationSettingsSnapshot> {
    const config = { ...connection.apiConfig, serverId };
    const [serverPreference, viewer, channelRooms] = await Promise.all([
      getServerNotificationPreference(config, { signal }),
      getViewerStateViaConnect(config, { signal }),
      connection.getAPI(createRoomDirectoryAPI).listRooms(RoomDirectoryScope.CHANNELS, { signal })
    ]);
    const mappedServerPreference = notificationPreferenceFromAPI(serverPreference);
    const roomPreferences = new Map(
      viewer.roomNotificationPreferences.map((preference) => [preference.roomId, preference])
    );
    return {
      version: ++nextNotificationSettingsSnapshotVersion,
      serverPreference: mappedServerPreference,
      rooms: channelRooms.map((room) => {
        const preference = roomPreferences.get(room.id);
        return {
          id: room.id,
          name: room.name,
          level: preference?.level ?? NotificationLevel.DEFAULT,
          effectiveLevel: preference?.effectiveLevel ?? NotificationLevel.NORMAL
        };
      })
    };
  }

  function mutationScope(): NotificationMutationScope {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      serverId,
      connection,
      queryKey: settingsQueryKeys.notificationPreferences(serverId, connection),
      privacyGeneration
    };
  }

  function isCurrentSession(
    variables: NotificationMutationScope | undefined
  ): variables is NotificationMutationScope {
    return (
      variables !== undefined &&
      componentActive &&
      serverScope.isCurrent() &&
      variables.serverId === serverScope.serverId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  const serverPreferenceMutation = createMutation(
    () => ({
      onMutate: ({ queryKey }: ServerPreferenceVariables) =>
        queryClient.cancelQueries({ queryKey, exact: true }),
      mutationFn: ({ serverId, connection, level }: ServerPreferenceVariables) =>
        updateServerNotificationPreference({ ...connection.apiConfig, serverId }, level),
      onSuccess: (preference, variables) => {
        if (!isCurrentSession(variables)) return;
        const mapped = notificationPreferenceFromAPI(preference);
        notificationLevelStore.setServerPreference(mapped.level, mapped.effectiveLevel);
        for (const room of snapshot?.rooms ?? []) {
          if (room.level === NotificationLevel.DEFAULT) {
            notificationLevelStore.setRoomPreference(room.id, room.level, mapped.effectiveLevel);
          }
        }
        queryClient.setQueryData<NotificationSettingsSnapshot>(variables.queryKey, (current) =>
          current
            ? {
                ...current,
                serverPreference: mapped,
                rooms: current.rooms.map((room) =>
                  room.level === NotificationLevel.DEFAULT
                    ? { ...room, effectiveLevel: mapped.effectiveLevel }
                    : room
                )
              }
            : current
        );
        toast.success(m('settings.notifications.levels.server_updated'));
      },
      onError: (mutationError, variables) => {
        if (!isCurrentSession(variables)) return;
        toast.error(
          mutationError instanceof Error
            ? mutationError.message
            : m('settings.notifications.levels.update_failed')
        );
      },
      onSettled: async (_data, _error, variables) => {
        try {
          if (isCurrentSession(variables)) {
            await queryClient.invalidateQueries({ queryKey: variables.queryKey, exact: true });
          }
        } finally {
          if (componentActive) preferenceMutationLocked = false;
        }
      }
    }),
    () => queryClient
  );

  const roomPreferenceMutation = createMutation(
    () => ({
      onMutate: ({ queryKey }: RoomPreferenceVariables) =>
        queryClient.cancelQueries({ queryKey, exact: true }),
      mutationFn: ({ serverId, connection, roomId, level }: RoomPreferenceVariables) =>
        updateRoomNotificationPreference({ ...connection.apiConfig, serverId }, roomId, level),
      onSuccess: (preference, variables) => {
        if (!isCurrentSession(variables)) return;
        const mapped = notificationPreferenceFromAPI(preference);
        notificationLevelStore.setRoomPreference(
          variables.roomId,
          mapped.level,
          mapped.effectiveLevel
        );
        queryClient.setQueryData<NotificationSettingsSnapshot>(variables.queryKey, (current) =>
          current
            ? {
                ...current,
                rooms: current.rooms.map((room) =>
                  room.id === variables.roomId ? { ...room, ...mapped } : room
                )
              }
            : current
        );
        toast.success(m('settings.notifications.levels.room_updated'));
      },
      onError: (mutationError, variables) => {
        if (!isCurrentSession(variables)) return;
        toast.error(
          mutationError instanceof Error
            ? mutationError.message
            : m('settings.notifications.levels.update_failed')
        );
      },
      onSettled: async (_data, _error, variables) => {
        try {
          if (isCurrentSession(variables)) {
            await queryClient.invalidateQueries({ queryKey: variables.queryKey, exact: true });
          }
        } finally {
          if (componentActive) preferenceMutationLocked = false;
        }
      }
    }),
    () => queryClient
  );

  let preferenceMutationLocked = $state(false);
  const savingPreference = $derived(
    preferenceMutationLocked ||
      serverPreferenceMutation.isPending ||
      roomPreferenceMutation.isPending
  );
  const savingRoomId = $derived(
    roomPreferenceMutation.isPending && isCurrentSession(roomPreferenceMutation.variables)
      ? roomPreferenceMutation.variables.roomId
      : null
  );

  function handleServerLevelChange(newLevel: NotificationLevel) {
    if (preferenceMutationLocked || newLevel === serverLevel) return;
    preferenceMutationLocked = true;
    serverPreferenceMutation.mutate({ ...mutationScope(), level: newLevel });
  }

  function handleRoomLevelChange(roomId: string, newLevel: NotificationLevel) {
    if (preferenceMutationLocked) return;
    preferenceMutationLocked = true;
    roomPreferenceMutation.mutate({ ...mutationScope(), roomId, level: newLevel });
  }

  const levelOptions = $derived<
    Array<{ value: NotificationLevel; label: string; description: string }>
  >([
    {
      value: NotificationLevel.DEFAULT,
      label: m('settings.notifications.levels.default.label'),
      description: m('settings.notifications.levels.default.description')
    },
    {
      value: NotificationLevel.MUTED,
      label: m('settings.notifications.levels.muted.label'),
      description: m('settings.notifications.levels.muted.description')
    },
    {
      value: NotificationLevel.NORMAL,
      label: m('settings.notifications.levels.normal.label'),
      description: m('settings.notifications.levels.normal.description')
    },
    {
      value: NotificationLevel.ALL_MESSAGES,
      label: m('settings.notifications.levels.all_messages.label'),
      description: m('settings.notifications.levels.all_messages.description')
    }
  ]);

  function notificationPreferenceFromAPI(pref: {
    level: NotificationLevel;
    effectiveLevel: NotificationLevel;
  }): NotificationPreference {
    return {
      level: notificationLevelOrDefault(pref.level),
      effectiveLevel: notificationLevelOrDefault(pref.effectiveLevel)
    };
  }

  const serverLevelOptions = $derived(
    levelOptions.filter((o) => o.value !== NotificationLevel.DEFAULT)
  );

  function levelLabel(level: NotificationLevel): string {
    return levelOptions.find((o) => o.value === level)?.label ?? String(level);
  }
</script>

{#if loading}
  <div class="text-muted">{m('settings.notifications.levels.loading')}</div>
{:else if error}
  <div class="max-w-lg">
    <FormError {error} />
  </div>
{:else}
  <FormSection title={m('settings.notifications.levels.server_title')} maxWidth="max-w-lg">
    <p class="mb-3 text-sm text-muted">
      {m('settings.notifications.levels.server_description')}
    </p>

    <div
      class="flex flex-col gap-2"
      role="radiogroup"
      aria-label={m('settings.notifications.levels.server_title')}
    >
      {#each serverLevelOptions as option (option.value)}
        {@const isSelected = serverLevel === option.value}
        <ChoiceRow
          label={option.label}
          description={option.description}
          selected={isSelected}
          disabled={savingPreference}
          onclick={() => handleServerLevelChange(option.value)}
        />
      {/each}
    </div>
  </FormSection>

  {#if rooms.length > 0}
    <FormSection title={m('settings.notifications.levels.room_title')} maxWidth="max-w-lg" bordered>
      <p class="mb-3 text-sm text-muted">
        {m('settings.notifications.levels.room_description', {
          level: levelLabel(serverEffectiveLevel)
        })}
      </p>

      <div class="flex flex-col gap-2">
        {#each rooms as room (room.id)}
          {@const isSaving = savingRoomId === room.id}
          <div
            data-testid={`room-notification-${room.name}`}
            class={[
              'flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2',
              room.effectiveLevel === NotificationLevel.MUTED ? 'opacity-60' : ''
            ]}
          >
            <div class="min-w-0">
              <div class="flex items-center gap-1.5">
                <span class="text-muted">#</span>
                <span class="truncate font-medium">{room.name}</span>
              </div>
              {#if room.level !== NotificationLevel.DEFAULT}
                <div class="text-xs text-muted">
                  {m('settings.notifications.levels.effective', {
                    level: levelLabel(room.effectiveLevel)
                  })}
                </div>
              {/if}
            </div>
            <select
              aria-label={m('settings.notifications.levels.room_level_label', { room: room.name })}
              value={String(room.level)}
              disabled={savingPreference}
              onchange={(e) =>
                handleRoomLevelChange(
                  room.id,
                  Number((e.target as HTMLSelectElement).value) as NotificationLevel
                )}
              class={['input w-auto min-w-[120px] text-sm', isSaving ? 'opacity-50' : '']}
            >
              {#each levelOptions as option (option.value)}
                <option value={String(option.value)}>{option.label}</option>
              {/each}
            </select>
          </div>
        {/each}
      </div>
    </FormSection>
  {/if}
{/if}
