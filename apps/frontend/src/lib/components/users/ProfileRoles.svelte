<!--
@component

A profile's assigned roles as filled chips in role-catalogue order. Names come
from the shared catalogue; a role missing from it shows its stored name. The
list renders nothing for a user without roles.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { useProfileDetails } from './profileDetailsQuery.svelte';

  let {
    userId,
    chipClass = 'bg-surface',
    class: className
  }: {
    userId: string;
    /** Chip fill: one surface level above the chips' container. */
    chipClass?: string;
    class?: string;
  } = $props();

  const serverScope = useServerScope();
  const details = useProfileDetails(() => userId);

  const roles = $derived.by(() => {
    const assigned = details.data?.roles ?? [];
    if (!assigned.length) return [];
    const catalogue = serverScope.store.mentionRoles.roles;
    const position = new Map(catalogue.map((role, index) => [role.name, index]));
    const label = new Map(catalogue.map((role) => [role.name, role.displayName || role.name]));
    return [...assigned]
      .sort((a, b) => (position.get(a) ?? Infinity) - (position.get(b) ?? Infinity))
      .map((name) => ({ name, label: label.get(name) ?? name }));
  });
</script>

{#if roles.length}
  <ul
    class={['flex flex-wrap gap-1.5', className]}
    aria-label={m('chat.profile.roles')}
    data-testid="profile-roles"
  >
    {#each roles as role (role.name)}
      <li class={['max-w-full truncate rounded-full px-2.5 py-0.5', chipClass]}>
        <bdi>{role.label}</bdi>
      </li>
    {/each}
  </ul>
{/if}
