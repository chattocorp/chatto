<!--
@component
Lists the self-verification identities available for this profile. Sidebar
callers supply a storage key for the disclosure. Popups show the list directly.
-->
<script lang="ts">
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import { m } from '$lib/i18n/messages';
  import { SelfVerificationPill } from '$lib/ui';

  let {
    identities,
    sidebarKey
  }: {
    identities: { method: string; icon: string; identity: string }[];
    sidebarKey?: string;
  } = $props();

  const label = $derived(m('chat.profile.verified_identities'));
</script>

{#snippet identityList()}
  <ul aria-label={label} class="flex min-w-0 flex-wrap gap-1.5">
    {#each identities as entry (`${entry.method}/${entry.identity}`)}
      <li class="max-w-full leading-none"><SelfVerificationPill {...entry} /></li>
    {/each}
  </ul>
{/snippet}

{#if identities.length}
  {#if sidebarKey}
    <div class="-mx-4 mt-6">
      <RoomGroupSection {label} persistKey={sidebarKey} items={[]} separated>
        {#snippet content()}
          <div class="px-1 py-2">{@render identityList()}</div>
        {/snippet}
      </RoomGroupSection>
    </div>
  {:else}
    <section class="menu-section px-3 py-2 text-start">
      {@render identityList()}
    </section>
  {/if}
{/if}
