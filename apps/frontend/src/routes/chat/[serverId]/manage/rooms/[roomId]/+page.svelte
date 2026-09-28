<!--
@component

General section of a room: its name and description, visibility, Slow Mode,
and Threading Mode, each in its own panel. The name and description submit
together; the other settings save as soon as they change. A viewer who cannot
change the settings goes to the Members section instead.
-->
<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { invalidateAdminRoomLayoutQueries } from '$lib/query/adminInvalidation';
  import { createMutation, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { toast } from '$lib/ui/toast';
  import type { RoomSettingsPatch } from './roomSettings';
  import RoomNameSettingsPanel from './RoomNameSettingsPanel.svelte';
  import RoomSlowModeSettingsPanel from './RoomSlowModeSettingsPanel.svelte';
  import RoomThreadingModeSettingsPanel from './RoomThreadingModeSettingsPanel.svelte';
  import RoomVisibilitySettingsPanel from './RoomVisibilitySettingsPanel.svelte';
  import { useRoomManagement, type RoomMutationScope } from './roomManagementContext';

  const serverScope = useServerScope();
  const detail = useRoomManagement();
  /** Increases after a saved name or description, so the name form reseeds its drafts. */
  let detailsRevision = $state(0);

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
        // Panels save concurrently, and each response describes the whole
        // room. Apply only the fields of this patch, so an older response
        // cannot restore a value that another panel changed. Realtime room
        // refreshes reconcile the other fields.
        const { patch } = variables;
        queryClient.setQueryData<AdminManagedRoom | null>(
          adminQueryKeys.room(variables.serverId, variables.connection, variables.roomId),
          (current) =>
            current
              ? {
                  ...current,
                  ...('name' in patch && { name: updated.name }),
                  ...('description' in patch && { description: updated.description || null }),
                  ...('universal' in patch && { isUniversal: updated.universal }),
                  ...('slowModeSeconds' in patch && { slowModeSeconds: updated.slowModeSeconds }),
                  ...('threadingMode' in patch && { threadingMode: updated.threadingMode })
                }
              : current
        );
        if ('name' in variables.patch || 'description' in variables.patch) {
          detailsRevision += 1;
        }
      }
      invalidateAdminRoomLayoutQueries(variables.serverId, variables.connection, variables.roomId);
      void serverScope.store.adminRoomLayout.refresh();
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
    return detail.isCurrentRoom(variables);
  }
</script>

{#if detail.canManageRoom}
  {#key `${detail.room.id}:${detailsRevision}`}
    <RoomNameSettingsPanel room={detail.room} onSave={saveSettings} />
  {/key}
  <RoomVisibilitySettingsPanel room={detail.room} onSave={saveSettings} />
  <RoomSlowModeSettingsPanel room={detail.room} onSave={saveSettings} />
  <RoomThreadingModeSettingsPanel room={detail.room} onSave={saveSettings} />
{/if}
