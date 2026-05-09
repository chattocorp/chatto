<!--
@component

Per-space notification level preferences page.
Allows the user to set space-level and per-room notification levels.

**Levels:**
- Default - Inherit from parent (space default for rooms, Normal for spaces)
- Muted - No notifications, no unread markers
- Normal - Standard behavior (unread + mentions/DMs/threads)
- All Messages - Like Normal, plus a notification for every root message
-->
<script lang="ts">
  import { page } from '$app/state';
  import { getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphql } from '$lib/gql';
  import { NotificationLevel } from '$lib/gql/graphql';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';

  const getInstanceId = getActiveInstance();
  const notificationLevelStore = instanceRegistry.getStore(getInstanceId()).notificationLevels;
  import { PaneHeader, FormSection } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const spaceId = $derived(getActiveInstanceSpaceId()());
  const connection = useConnection();

  // Space-level preference
  let spaceLevel = $state<NotificationLevel>(NotificationLevel.Default);
  let spaceEffectiveLevel = $state<NotificationLevel>(NotificationLevel.Normal);

  // Room preferences
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
  let savingSpaceLevel = $state(false);
  let savingRoomId = $state<string | null>(null);

  // Load preferences when spaceId changes
  $effect(() => {
    if (spaceId) {
      loadPreferences(spaceId);
    }
  });

  async function loadPreferences(sid: string) {
    loading = true;
    error = '';

    try {
      const result = await connection().client
        .query(
          graphql(`
            query GetSpaceNotificationPreferences {
              instance {
                viewerNotificationPreference {
                  level
                  effectiveLevel
                }
              }
              me {
                rooms {
                  id
                  name
                  viewerNotificationPreference {
                    level
                    effectiveLevel
                  }
                }
              }
            }
          `),
          {}
        )
        .toPromise();

      if (result.error) {
        error = result.error.message;
        return;
      }

      if (result.data?.instance?.viewerNotificationPreference) {
        const pref = result.data.instance.viewerNotificationPreference;
        // Space can't inherit (nothing above it), so DEFAULT maps to NORMAL for display
        spaceLevel =
          pref.level === NotificationLevel.Default ? NotificationLevel.Normal : pref.level;
        spaceEffectiveLevel = pref.effectiveLevel;
        notificationLevelStore.setSpacePreference(sid, pref.level, pref.effectiveLevel);
      }

      if (result.data?.me?.rooms) {
        rooms = result.data.me.rooms.map((room) => ({
          id: room.id,
          name: room.name,
          level: room.viewerNotificationPreference?.level ?? NotificationLevel.Default,
          effectiveLevel:
            room.viewerNotificationPreference?.effectiveLevel ?? NotificationLevel.Normal
        }));

        // Update the notification level store for each room
        for (const room of rooms) {
          notificationLevelStore.setRoomPreference(sid, room.id, room.level, room.effectiveLevel);
        }
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load preferences';
    } finally {
      loading = false;
    }
  }

  async function handleSpaceLevelChange(newLevel: NotificationLevel) {
    if (!spaceId) return;
    const sid = spaceId;
    savingSpaceLevel = true;

    try {
      const result = await connection().client
        .mutation(
          graphql(`
            mutation SetSpaceNotificationLevel($input: SetSpaceNotificationLevelInput!) {
              setSpaceNotificationLevel(input: $input) {
                level
                effectiveLevel
              }
            }
          `),
          { input: { spaceId: sid, level: newLevel } }
        )
        .toPromise();

      if (result.error) {
        toast.error(result.error.message);
        return;
      }

      if (result.data?.setSpaceNotificationLevel) {
        const pref = result.data.setSpaceNotificationLevel;
        spaceLevel = pref.level;
        spaceEffectiveLevel = pref.effectiveLevel;
        notificationLevelStore.setSpacePreference(sid, pref.level, pref.effectiveLevel);

        // Reload room preferences since effective levels may have changed
        // (rooms set to DEFAULT inherit from space)
        await loadPreferences(sid);
        toast.success('Space notification level updated');
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update');
    } finally {
      savingSpaceLevel = false;
    }
  }

  async function handleRoomLevelChange(roomId: string, newLevel: NotificationLevel) {
    if (!spaceId) return;
    const sid = spaceId;
    savingRoomId = roomId;

    try {
      const result = await connection().client
        .mutation(
          graphql(`
            mutation SetRoomNotificationLevel($input: SetRoomNotificationLevelInput!) {
              setRoomNotificationLevel(input: $input) {
                level
                effectiveLevel
              }
            }
          `),
          { input: { spaceId: sid, roomId, level: newLevel } }
        )
        .toPromise();

      if (result.error) {
        toast.error(result.error.message);
        return;
      }

      if (result.data?.setRoomNotificationLevel) {
        const pref = result.data.setRoomNotificationLevel;

        // Update local state
        const idx = rooms.findIndex((r) => r.id === roomId);
        if (idx !== -1) {
          rooms[idx] = { ...rooms[idx], level: pref.level, effectiveLevel: pref.effectiveLevel };
        }

        notificationLevelStore.setRoomPreference(sid, roomId, pref.level, pref.effectiveLevel);
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

  // Space-level options exclude DEFAULT (space can't inherit from anything above it)
  const spaceLevelOptions = levelOptions.filter((o) => o.value !== NotificationLevel.Default);

  function levelLabel(level: NotificationLevel): string {
    return levelOptions.find((o) => o.value === level)?.label ?? level;
  }
</script>

<PageTitle title="Preferences" />

<PaneHeader title="Preferences" subtitle="Notification settings for this space" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if loading}
    <div class="text-muted">Loading...</div>
  {:else if error}
    <div class="max-w-lg">
      <FormError {error} />
    </div>
  {:else}
    <!-- Space-level notification level -->
    <FormSection title="Space Notification Level" maxWidth="max-w-lg">
      <p class="mb-3 text-sm text-muted">
        Controls how you receive notifications for all rooms in this space. Individual rooms can
        override this setting.
      </p>

      <div class="flex flex-col gap-2">
        {#each spaceLevelOptions as option (option.value)}
          {@const isSelected = spaceLevel === option.value}
          <button
            type="button"
            disabled={savingSpaceLevel}
            class={[
              'flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2 text-left transition-colors',
              isSelected
                ? 'border-accent bg-accent/10'
                : 'hover:border-border-highlighted border-border hover:bg-surface-100',
              savingSpaceLevel ? 'opacity-50' : ''
            ]}
            onclick={() => handleSpaceLevelChange(option.value)}
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

    <!-- Per-room notification levels -->
    {#if rooms.length > 0}
      <FormSection title="Room Overrides" maxWidth="max-w-lg" bordered>
        <p class="mb-3 text-sm text-muted">
          Override the space-level setting for individual rooms. Rooms set to "Default" inherit the
          space setting ({levelLabel(spaceEffectiveLevel)}).
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
</div>
