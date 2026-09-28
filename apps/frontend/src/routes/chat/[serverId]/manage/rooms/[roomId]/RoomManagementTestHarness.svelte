<!--
@component

Test harness that renders the room management layout with section pages.
SvelteKit renders one section at a time; a spec can render several to check
that they share the layout's room and realtime handling.
-->
<script lang="ts">
  import RoomManagementLayout from './+layout.svelte';
  import RoomGeneralPage from './+page.svelte';
  import RoomMembersPage from './members/+page.svelte';
  import RoomPermissionsPage from './permissions/+page.svelte';

  let { sections }: { sections: ReadonlyArray<'general' | 'members' | 'permissions'> } = $props();
</script>

<RoomManagementLayout>
  {#if sections.includes('general')}
    <RoomGeneralPage />
  {/if}
  {#if sections.includes('members')}
    <RoomMembersPage />
  {/if}
  {#if sections.includes('permissions')}
    <RoomPermissionsPage />
  {/if}
</RoomManagementLayout>
