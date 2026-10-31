<!-- @component General room-group settings with protection for unsaved changes. -->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import UnsavedChangesGuard from '$lib/components/UnsavedChangesGuard.svelte';
  import { useRoomGroupManagement } from './roomGroupManagementContext';
  import RoomGroupGeneralSettingsPanel from './RoomGroupGeneralSettingsPanel.svelte';
  const serverScope = useServerScope();
  const detail = useRoomGroupManagement();
  /** Each section exposes its current draft state directly to the navigation guard. */
  type SettingsSection = { hasChanges(): boolean };
  let generalSection = $state<SettingsSection>();
  // Permission changes must send viewers to a section they can use.
  $effect(() => {
    if (detail.canManageGroup) return;
    void goto(
      resolve('/chat/[serverId]/manage/room-groups/[groupId]/permissions', {
        serverId: serverIdToSegment(serverScope.serverId),
        groupId: detail.groupId
      }),
      { replaceState: true }
    );
  });
</script>

<UnsavedChangesGuard
  enabled={detail.canManageGroup}
  hasChanges={() => generalSection?.hasChanges()}
/>

{#if detail.canManageGroup}
  {#key `${detail.group.id}:${detail.formRevision}`}
    <RoomGroupGeneralSettingsPanel
      bind:this={generalSection}
      group={detail.group}
      saving={detail.saving}
      onSave={detail.saveGeneralSettings}
    />
  {/key}
{/if}
