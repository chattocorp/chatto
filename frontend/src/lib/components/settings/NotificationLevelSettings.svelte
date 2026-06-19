<!--
@component

Server-wide and per-room notification level settings for the current user.
These preferences are server-side and sync across devices.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
  import { FormSection } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import {
    NotificationLevel,
    notificationLevelFromWire,
    notificationLevelToWire
  } from '$lib/preferences/notificationLevel';
  import {
    ListMyRoomsRequest,
    SetRoomNotificationLevelRequest,
    SetServerNotificationLevelRequest
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { RoomKind } from '$lib/pb/chatto/core/v1/models_pb';

  const activeServerId = getActiveServer();
  const notificationLevelStore = serverRegistry.getStore(activeServerId).notificationLevels;

  function wireClient() {
    const client = wireEventBusManager.getClient(activeServerId);
    if (!client) {
      throw new Error('No server connection');
    }
    return client;
  }

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

  $effect(() => {
    loadPreferences();
  });

  async function loadPreferences() {
    loading = true;
    error = '';

    try {
      const [viewerResult, roomsResult] = await Promise.all([
        wireClient().getViewer(),
        wireClient().listMyRooms(new ListMyRoomsRequest({ kind: RoomKind.CHANNEL }))
      ]);

      const serverPref = viewerResult.viewer?.serverNotificationPreference;
      if (serverPref) {
        const level = notificationLevelFromWire(serverPref.level);
        const effectiveLevel = notificationLevelFromWire(serverPref.effectiveLevel);
        serverLevel = level === NotificationLevel.Default ? NotificationLevel.Normal : level;
        serverEffectiveLevel = effectiveLevel;
        notificationLevelStore.setServerPreference(level, effectiveLevel);
      }

      rooms = roomsResult.roomViews
        .map((roomView) => ({
          id: roomView.room?.id ?? '',
          name: roomView.room?.name ?? '',
          level: notificationLevelFromWire(roomView.viewerNotificationPreference?.level),
          effectiveLevel: notificationLevelFromWire(
            roomView.viewerNotificationPreference?.effectiveLevel
          )
        }))
        .filter((room) => room.id && room.name);

      for (const room of rooms) {
        notificationLevelStore.setRoomPreference(room.id, room.level, room.effectiveLevel);
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load notification preferences';
    } finally {
      loading = false;
    }
  }

  async function handleServerLevelChange(newLevel: NotificationLevel) {
    savingServerLevel = true;

    try {
      const result = await wireClient().setServerNotificationLevel(
        new SetServerNotificationLevelRequest({ level: notificationLevelToWire(newLevel) })
      );

      if (result.preference) {
        const pref = result.preference;
        const level = notificationLevelFromWire(pref.level);
        const effectiveLevel = notificationLevelFromWire(pref.effectiveLevel);
        serverLevel = level;
        serverEffectiveLevel = effectiveLevel;
        notificationLevelStore.setServerPreference(level, effectiveLevel);
        await loadPreferences();
        toast.success('Server notification level updated');
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update');
    } finally {
      savingServerLevel = false;
    }
  }

  async function handleRoomLevelChange(roomId: string, newLevel: NotificationLevel) {
    savingRoomId = roomId;

    try {
      const result = await wireClient().setRoomNotificationLevel(
        new SetRoomNotificationLevelRequest({
          roomId,
          level: notificationLevelToWire(newLevel)
        })
      );

      if (result.preference) {
        const pref = result.preference;
        const level = notificationLevelFromWire(pref.level);
        const effectiveLevel = notificationLevelFromWire(pref.effectiveLevel);
        const idx = rooms.findIndex((r) => r.id === roomId);
        if (idx !== -1) {
          rooms[idx] = { ...rooms[idx], level, effectiveLevel };
        }

        notificationLevelStore.setRoomPreference(roomId, level, effectiveLevel);
        toast.success('Room notification level updated');
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update');
    } finally {
      savingRoomId = null;
    }
  }

  const levelOptions: Array<{ value: NotificationLevel; label: string; description: string }> = [
    {
      value: NotificationLevel.Default,
      label: 'Default',
      description: 'Use the inherited default'
    },
    {
      value: NotificationLevel.Muted,
      label: 'Muted',
      description: 'No notifications or unread markers'
    },
    {
      value: NotificationLevel.Normal,
      label: 'Normal',
      description: 'Unread markers + mentions, DMs, and thread replies'
    },
    {
      value: NotificationLevel.AllMessages,
      label: 'All Messages',
      description: 'Normal + notification for every new message'
    }
  ];

  const serverLevelOptions = levelOptions.filter((o) => o.value !== NotificationLevel.Default);

  function levelLabel(level: NotificationLevel): string {
    return levelOptions.find((o) => o.value === level)?.label ?? level;
  }
</script>

{#if loading}
  <div class="text-muted">Loading...</div>
{:else if error}
  <div class="max-w-lg">
    <FormError {error} />
  </div>
{:else}
  <FormSection title="Server Notification Level" maxWidth="max-w-lg">
    <p class="mb-3 text-sm text-muted">
      Controls how you receive notifications for all rooms in this server. Individual rooms can
      override this setting.
    </p>

    <div class="flex flex-col gap-2">
      {#each serverLevelOptions as option (option.value)}
        {@const isSelected = serverLevel === option.value}
        <button
          type="button"
          disabled={savingServerLevel}
          class={[
            'flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2 text-left transition-colors',
            isSelected
              ? 'border-accent bg-accent/10'
              : 'hover:border-border-highlighted border-border hover:bg-surface-100',
            savingServerLevel ? 'opacity-50' : ''
          ]}
          onclick={() => handleServerLevelChange(option.value)}
        >
          <span
            class={[
              'flex h-5 w-5 shrink-0 items-center justify-center rounded-full border-2 transition-colors',
              isSelected ? 'border-accent bg-accent' : 'border-muted'
            ]}
          >
            {#if isSelected}
              <span class="h-2 w-2 rounded-full bg-white"></span>
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
    <FormSection title="Room Overrides" maxWidth="max-w-lg" bordered>
      <p class="mb-3 text-sm text-muted">
        Override the server-level setting for individual rooms. Rooms set to "Default" inherit the
        server setting ({levelLabel(serverEffectiveLevel)}).
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
                  Effective: {levelLabel(room.effectiveLevel)}
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
