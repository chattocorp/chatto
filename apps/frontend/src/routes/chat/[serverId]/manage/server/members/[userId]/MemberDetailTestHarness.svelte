<!--
@component

Test harness that renders the member detail layout with one section page, as
SvelteKit does for the matching route.
-->
<script lang="ts">
  import MemberDetailLayout from './(sections)/+layout.svelte';
  import MemberProfilePage from './(sections)/+page.svelte';
  import MemberAccountPage from './(sections)/account/+page.svelte';
  import MemberRolesPage from './(sections)/roles/+page.svelte';
  import MemberPermissionsPage from './(sections)/permissions/+page.svelte';

  let { section }: { section: 'profile' | 'account' | 'roles' | 'permissions' } = $props();
</script>

<MemberDetailLayout>
  {#if section === 'profile'}
    <MemberProfilePage />
  {:else if section === 'account'}
    <MemberAccountPage />
  {:else if section === 'roles'}
    <MemberRolesPage />
  {:else}
    <MemberPermissionsPage />
  {/if}
</MemberDetailLayout>
