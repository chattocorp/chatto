<!--
@component

Server-wide and per-room notification level settings for the current user.
These preferences are server-side and sync across devices.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { NotificationLevel } from '$lib/render/types';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { FormSection } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import {
    getServerNotificationPreference,
    setRoomNotificationLevel,
    setServerNotificationLevel
  } from '$lib/api/notificationPreferences';
  import { createRoomDirectoryAPI, RoomDirectoryScope } from '$lib/api/roomDirectory';
  import { getViewerStateViaConnect } from '$lib/api/viewer';
  import { NotificationLevel as ApiNotificationLevel } from '@chatto/api-types/api/v1/notification_preferences_pb';

  const serverId = getActiveServer();
  const notificationLevelStore = serverRegistry.getStore(serverId).notificationLevels;
  const connection = useConnection();

  let serverLevel = $state<NotificationLevel>(NotificationLevel.Default);
  let serverEffectiveLevel = $state<NotificationLevel>(NotificationLevel.Normal);

  let rooms = $state<
    Array<{
      id: string;
      name: string;
      level: NotificationLevel;
      effectiveLevel: NotificationLevel;
    }>
  >([]);

  let loading = $state(true);
  let error = $state('');
  let savingServerLevel = $state(false);
  let savingRoomId = $state<string | null>(null);

  type NotificationPreference = {
    level: NotificationLevel;
    effectiveLevel: NotificationLevel;
  };

  onMount(() => {
    void loadPreferences();
  });

  async function loadPreferences() {
    loading = true;
    error = '';

    try {
      const config = connectConfig();
      const [serverPref, viewer, channelRooms] = await Promise.all([
        getServerNotificationPreference(config),
        getViewerStateViaConnect(config),
        createRoomDirectoryAPI(config).listRooms(RoomDirectoryScope.CHANNELS)
      ]);

      const mappedServerPref = notificationPreferenceFromAPI(serverPref);
      serverLevel =
        mappedServerPref.level === NotificationLevel.Default
          ? NotificationLevel.Normal
          : mappedServerPref.level;
      serverEffectiveLevel = mappedServerPref.effectiveLevel;
      notificationLevelStore.setServerPreference(
        mappedServerPref.level,
        mappedServerPref.effectiveLevel
      );

      const roomPreferences = new Map(
        viewer.roomNotificationPreferences.map((pref) => [pref.roomId, pref])
      );
      rooms = channelRooms.map((room) => {
        const pref = roomPreferences.get(room.id);
        return {
          id: room.id,
          name: room.name,
          level: pref?.level ?? NotificationLevel.Default,
          effectiveLevel: pref?.effectiveLevel ?? NotificationLevel.Normal
        };
      });

      for (const room of rooms) {
        notificationLevelStore.setRoomPreference(room.id, room.level, room.effectiveLevel);
      }
    } catch (e) {
      error = e instanceof Error ? e.message : m['settings.notifications.levels.load_failed']();
    } finally {
      loading = false;
    }
  }

  async function handleServerLevelChange(newLevel: NotificationLevel) {
    savingServerLevel = true;

    try {
      const pref = notificationPreferenceFromAPI(
        await setServerNotificationLevel(connectConfig(), notificationLevelToAPI(newLevel))
      );
      serverLevel = pref.level;
      serverEffectiveLevel = pref.effectiveLevel;
      notificationLevelStore.setServerPreference(pref.level, pref.effectiveLevel);

      await loadPreferences();
      toast.success(m['settings.notifications.levels.server_updated']());
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : m['settings.notifications.levels.update_failed']()
      );
    } finally {
      savingServerLevel = false;
    }
  }

  async function handleRoomLevelChange(roomId: string, newLevel: NotificationLevel) {
    savingRoomId = roomId;

    try {
      const pref = await setRoomLevel(roomId, newLevel);
      const idx = rooms.findIndex((r) => r.id === roomId);
      if (idx !== -1) {
        rooms[idx] = { ...rooms[idx], level: pref.level, effectiveLevel: pref.effectiveLevel };
      }

      notificationLevelStore.setRoomPreference(roomId, pref.level, pref.effectiveLevel);
      toast.success(m['settings.notifications.levels.room_updated']());
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : m['settings.notifications.levels.update_failed']()
      );
    } finally {
      savingRoomId = null;
    }
  }

  const levelOptions = $derived<
    Array<{ value: NotificationLevel; label: string; description: string }>
  >([
    {
      value: NotificationLevel.Default,
      label: m['settings.notifications.levels.default.label'](),
      description: m['settings.notifications.levels.default.description']()
    },
    {
      value: NotificationLevel.Muted,
      label: m['settings.notifications.levels.muted.label'](),
      description: m['settings.notifications.levels.muted.description']()
    },
    {
      value: NotificationLevel.Normal,
      label: m['settings.notifications.levels.normal.label'](),
      description: m['settings.notifications.levels.normal.description']()
    },
    {
      value: NotificationLevel.AllMessages,
      label: m['settings.notifications.levels.all_messages.label'](),
      description: m['settings.notifications.levels.all_messages.description']()
    }
  ]);

  async function setRoomLevel(
    roomId: string,
    newLevel: NotificationLevel
  ): Promise<NotificationPreference> {
    const pref = await setRoomNotificationLevel(
      connectConfig(),
      roomId,
      notificationLevelToAPI(newLevel)
    );
    return notificationPreferenceFromAPI(pref);
  }

  function connectConfig() {
    const conn = connection();
    return {
      serverId,
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    };
  }

  function notificationPreferenceFromAPI(pref: {
    level: ApiNotificationLevel;
    effectiveLevel: ApiNotificationLevel;
  }): NotificationPreference {
    return {
      level: notificationLevelFromAPI(pref.level),
      effectiveLevel: notificationLevelFromAPI(pref.effectiveLevel)
    };
  }

  const serverLevelOptions = $derived(
    levelOptions.filter((o) => o.value !== NotificationLevel.Default)
  );

  function levelLabel(level: NotificationLevel): string {
    return levelOptions.find((o) => o.value === level)?.label ?? level;
  }

  function notificationLevelToAPI(level: NotificationLevel): ApiNotificationLevel {
    switch (level) {
      case NotificationLevel.Muted:
        return ApiNotificationLevel.MUTED;
      case NotificationLevel.Normal:
        return ApiNotificationLevel.NORMAL;
      case NotificationLevel.AllMessages:
        return ApiNotificationLevel.ALL_MESSAGES;
      case NotificationLevel.Default:
      default:
        return ApiNotificationLevel.DEFAULT;
    }
  }

  function notificationLevelFromAPI(level: ApiNotificationLevel): NotificationLevel {
    switch (level) {
      case ApiNotificationLevel.MUTED:
        return NotificationLevel.Muted;
      case ApiNotificationLevel.NORMAL:
        return NotificationLevel.Normal;
      case ApiNotificationLevel.ALL_MESSAGES:
        return NotificationLevel.AllMessages;
      case ApiNotificationLevel.DEFAULT:
      case ApiNotificationLevel.UNSPECIFIED:
      default:
        return NotificationLevel.Default;
    }
  }
</script>

{#if loading}
  <div class="text-muted">{m['settings.notifications.levels.loading']()}</div>
{:else if error}
  <div class="max-w-lg">
    <FormError {error} />
  </div>
{:else}
  <FormSection title={m['settings.notifications.levels.server_title']()} maxWidth="max-w-lg">
    <p class="mb-3 text-sm text-muted">
      {m['settings.notifications.levels.server_description']()}
    </p>

    <div class="flex flex-col gap-2">
      {#each serverLevelOptions as option (option.value)}
        {@const isSelected = serverLevel === option.value}
        <button
          type="button"
          disabled={savingServerLevel}
          class={['choice-row', isSelected && 'choice-row-selected']}
          onclick={() => handleServerLevelChange(option.value)}
        >
          <span class={['choice-indicator', isSelected && 'choice-indicator-selected']}>
            {#if isSelected}
              <span class="choice-indicator-dot"></span>
            {/if}
          </span>
          <div>
            <div class={isSelected ? 'font-medium' : ''}>{option.label}</div>
            <div class="text-sm text-muted">{option.description}</div>
          </div>
        </button>
      {/each}
    </div>
  </FormSection>

  {#if rooms.length > 0}
    <FormSection
      title={m['settings.notifications.levels.room_title']()}
      maxWidth="max-w-lg"
      bordered
    >
      <p class="mb-3 text-sm text-muted">
        {m['settings.notifications.levels.room_description']({
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
              room.effectiveLevel === NotificationLevel.Muted ? 'opacity-60' : ''
            ]}
          >
            <div class="min-w-0">
              <div class="flex items-center gap-1.5">
                <span class="text-muted">#</span>
                <span class="truncate font-medium">{room.name}</span>
              </div>
              {#if room.level !== NotificationLevel.Default}
                <div class="text-xs text-muted">
                  {m['settings.notifications.levels.effective']({
                    level: levelLabel(room.effectiveLevel)
                  })}
                </div>
              {/if}
            </div>
            <select
              value={room.level}
              disabled={isSaving}
              onchange={(e) =>
                handleRoomLevelChange(
                  room.id,
                  (e.target as HTMLSelectElement).value as NotificationLevel
                )}
              class={['input w-auto min-w-[120px] text-sm', isSaving ? 'opacity-50' : '']}
            >
              {#each levelOptions as option (option.value)}
                <option value={option.value}>{option.label}</option>
              {/each}
            </select>
          </div>
        {/each}
      </div>
    </FormSection>
  {/if}
{/if}
