<!--
@component

Test harness that renders the role detail layout with section pages, as
SvelteKit does for the matching route. Specs can render several sections at
once to check that they share the layout's role.
-->
<script lang="ts">
  import RoleDetailLayout from './+layout.svelte';
  import RoleGeneralPage from './+page.svelte';
  import RolePermissionsPage from './permissions/+page.svelte';
  import RoleMembersPage from './members/+page.svelte';

  type Section = 'general' | 'permissions' | 'members';

  let { sections = ['general'] }: { sections?: Section[] } = $props();
</script>

<RoleDetailLayout>
  {#each sections as section (section)}
    {#if section === 'general'}
      <RoleGeneralPage />
    {:else if section === 'permissions'}
      <RolePermissionsPage />
    {:else}
      <RoleMembersPage />
    {/if}
  {/each}
</RoleDetailLayout>
