<!--
@component

General section of a room: its name and description, visibility, Slow Mode,
Threading Mode, and membership activity, each in its own form with Save and Discard actions. A viewer who cannot
change the settings goes to the Members section instead.
-->
<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import FeatureCompatibilityNotice from '$lib/components/chat/FeatureCompatibilityNotice.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { beforeNavigate, goto } from '$app/navigation';
  import type { BeforeNavigate } from '@sveltejs/kit';
  import { base, resolve } from '$app/paths';
  import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
  import { createRoomCommandAPI } from '@chatto/client/api/rooms';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { invalidateAdminRoomLayoutQueries } from '$lib/query/adminInvalidation';
  import { createMutation, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { toast } from '$lib/ui/toast';
  import { ConfirmDialog } from '$lib/ui';
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
  let membershipEventsSection = $state<SettingsSection>();
  let pendingNavigation = $state<BeforeNavigate | null>(null);
  // Approval applies only to the resumed destination, not to later navigation attempts.
  let approvedDestination: string | null = null;
  let approvedUnload = false;

  beforeNavigate((navigation) => {
    const { cancel, type, to } = navigation;
    if (type === 'leave' && approvedUnload) {
      approvedUnload = false;
      return;
    }
    if (to && approvedDestination === to.url.href) {
      approvedDestination = null;
      return;
    }
    // Permission loss must still be able to remove this editor and its private drafts.
    if (!detail.canManageRoom) {
      pendingNavigation = null;
      return;
    }
    const dirty = [
      nameSection,
      visibilitySection,
      slowModeSection,
      threadingSection,
      membershipEventsSection
    ].some((section) => section?.hasChanges());
    if (!dirty) return;
    cancel();
    // Closing or reloading a tab must use the browser's native unload warning.
    if (type !== 'leave' && to) pendingNavigation ??= navigation;
  });

  /** Resumes the selected destination without prompting twice or adding a back-history entry. */
  async function discardAndLeave(): Promise<void> {
    const navigation = pendingNavigation;
    pendingNavigation = null;
    if (!navigation?.to) return;
    if (navigation.type === 'popstate' && navigation.delta) {
      approvedDestination = navigation.to.url.href;
      window.history.go(navigation.delta);
    } else if (navigation.willUnload) {
      approvedUnload = true;
      window.location.assign(navigation.to.url.href);
    } else {
      approvedDestination = navigation.to.url.href;
      try {
        const { pathname, search, hash } = navigation.to.url;
        // The captured URL already contains the deployment base. Resolve the app-relative path.
        const path = pathname.slice(base.length) || '/';
        await goto(resolve(`${path}${search}${hash}` as '/'));
      } catch {
        // Another navigation guard can still cancel. Keep this page's drafts intact.
      } finally {
        approvedDestination = null;
      }
    }
  }

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

{#if detail.canManageRoom}
  {#if pendingNavigation}
    <ConfirmDialog
      title={m('admin.rooms_admin.settings.discard_changes')}
      tone="warning"
      actionLabel={m('admin.rooms_admin.settings.discard_changes')}
      onconfirm={discardAndLeave}
      onclose={() => (pendingNavigation = null)}
    >
      {m('admin.rooms_admin.settings.unsaved_changes')}
    </ConfirmDialog>
  {/if}
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
    <RoomMembershipEventsSettingsPanel
      bind:this={membershipEventsSection}
      room={detail.room}
      onSave={saveSettings}
    />
  {/key}
{/if}
