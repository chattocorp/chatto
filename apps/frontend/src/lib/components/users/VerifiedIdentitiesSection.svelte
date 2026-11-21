<!--
@component
Lists the self-verification identities available for this profile. Sidebar
callers supply a storage key for the disclosure. Popups show the list directly.
-->
<script lang="ts">
  import type { VerifiedIdentity } from '@chatto/client/api/users';
  import { providerIcon } from '$lib/auth/providerIcon';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import { m } from '$lib/i18n/messages';
  import { SelfVerificationPill } from '$lib/ui';

  let {
    identities,
    sidebarKey
  }: {
    identities: VerifiedIdentity[];
    sidebarKey?: string;
  } = $props();

  const label = $derived(m('chat.profile.verified_identities'));
</script>

{#snippet identityList()}
  <ul aria-label={label} class="flex min-w-0 flex-wrap gap-1.5">
    {#each identities as entry (JSON.stringify([entry.method, entry.value]))}
      <li class="flex max-w-full flex-wrap items-center gap-x-2 gap-y-1 leading-none">
        <SelfVerificationPill
          method={entry.methodLabel}
          icon={providerIcon(entry.method)}
          identity={entry.value}
        />
        {#if entry.method === 'atproto' && entry.profileTypes?.includes('app.bsky.actor.profile')}
          <a
            href={`https://bsky.app/profile/${encodeURIComponent(entry.value)}`}
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-link underline">{m('chat.profile.view_on_bluesky')}</a
          >
        {/if}
      </li>
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
