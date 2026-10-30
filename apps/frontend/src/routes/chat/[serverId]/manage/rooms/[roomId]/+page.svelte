<!--
@component

General section of a room: its name and description, visibility, Slow Mode,
Threading Mode, and membership activity. Membership activity saves immediately;
the other panels have Save and Discard actions. A viewer who cannot
change the settings goes to the Members section instead.
-->
<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import FeatureCompatibilityNotice from '$lib/components/chat/FeatureCompatibilityNotice.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { createRoomCommandAPI } from '@chatto/client/api/rooms';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { invalidateAdminRoomLayoutQueries } from '$lib/query/adminInvalidation';
  import { createMutation, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { toast } from '$lib/ui/toast';
  import UnsavedChangesGuard from '$lib/components/UnsavedChangesGuard.svelte';
  import { applyRoomSettingsResponse, type RoomSettingsPatch } from './roomSettings';
  import RoomNameSettingsPanel from './RoomNameSettingsPanel.svelte';
  import RoomSlowModeSettingsPanel from './RoomSlowModeSettingsPanel.svelte';
  import RoomThreadingModeSettingsPanel from './RoomThreadingModeSettingsPanel.svelte';
  import RoomVisibilitySettingsPanel from './RoomVisibilitySettingsPanel.svelte';
  import RoomMembershipEventsSettingsPanel from './RoomMembershipEventsSettingsPanel.svelte';
  import { useRoomManagement, type RoomMutationScope } from './roomManagementContext';

  const serverScope = useServerScope();
  const detail = useRoomManagement();
  /** Each section exposes its current draft state directly to the navigation guard. */
  type SettingsSection = { hasChanges(): boolean };
  let nameSection = $state<SettingsSection>();
  let visibilitySection = $state<SettingsSection>();
  let slowModeSection = $state<SettingsSection>();
  let threadingSection = $state<SettingsSection>();
  type UpdateRoomVariables = RoomMutationScope & { patch: RoomSettingsPatch };

  // Only managers can change the settings. Other viewers of the page may
  // manage the room permissions, so send them to a section they can use.
  $effect(() => {
    if (detail.canManageRoom) return;
    void goto(
      resolve('/chat/[serverId]/manage/rooms/[roomId]/members', {
        serverId: serverIdToSegment(serverScope.serverId),
        roomId: detail.roomId
      }),
      { replaceState: true }
    );
  });

  const updateRoomMutation = createMutation(() => ({
    mutationFn: async ({ connection, roomId, patch }: UpdateRoomVariables) => {
      const updated = await connection
        .getAPI(createRoomCommandAPI)
        .updateRoom({ roomId, ...patch });
      if (!updated) throw new Error('Room update returned no room');
      return updated;
    },
    onSuccess: (updated, variables) => {
      if (!detail.isCurrentRoom(variables)) return;
      if (detail.canApplyRoomSnapshot(variables)) {
        const { patch } = variables;
        queryClient.setQueryData<AdminManagedRoom | null>(
          adminQueryKeys.room(variables.serverId, variables.connection, variables.roomId),
          (current) => (current ? applyRoomSettingsResponse(current, updated, patch) : current)
        );
      }
      invalidateAdminRoomLayoutQueries(variables.serverId, variables.connection, variables.roomId);
      void serverUi(serverScope.store).adminRoomLayout.refresh();
      toast.success(m('admin.rooms_admin.room_updated'));
    },
    onError: (error, variables) => {
      if (!detail.isCurrentRoom(variables)) return;
      toast.error(
        m('admin.rooms_admin.update_room_failed', {
          error: errorMessage(error)
        })
      );
    }
  }));

  /**
   * Saves one panel's sparse patch. Panels save independently, so a change in
   * one panel does not wait for or reset another. Resolves to true when the
   * room accepted the patch and the page still shows that room.
   */
  async function saveSettings(patch: RoomSettingsPatch): Promise<boolean> {
    if (!detail.canManageRoom) return false;
    const variables = { ...detail.mutationScope(), patch };
    try {
      await updateRoomMutation.mutateAsync(variables);
    } catch {
      // The mutation reports the failure.
      return false;
    }
    return detail.isCurrentRoom(variables) && detail.canManageRoom;
  }
</script>

<UnsavedChangesGuard
  enabled={detail.canManageRoom}
  hasChanges={() =>
    [nameSection, visibilitySection, slowModeSection, threadingSection].some((section) =>
      section?.hasChanges()
    )}
/>

{#if detail.canManageRoom}
  {#key detail.room.id}
    <RoomNameSettingsPanel bind:this={nameSection} room={detail.room} onSave={saveSettings} />
    <RoomVisibilitySettingsPanel
      bind:this={visibilitySection}
      room={detail.room}
      onSave={saveSettings}
    />
    <RoomSlowModeSettingsPanel
      bind:this={slowModeSection}
      room={detail.room}
      onSave={saveSettings}
    />
    {#if detail.room.threadSlowModeSeconds === undefined}
      <FeatureCompatibilityNotice
        serverSegment={serverIdToSegment(serverScope.serverId)}
        features={serverScope.store.serverInfo.limitedFeatures.filter(
          (feature) => feature === 'independentSlowMode'
        )}
      />
    {/if}
    <RoomThreadingModeSettingsPanel
      bind:this={threadingSection}
      room={detail.room}
      onSave={saveSettings}
    />
    <RoomMembershipEventsSettingsPanel room={detail.room} onSave={saveSettings} />
  {/key}
{/if}
