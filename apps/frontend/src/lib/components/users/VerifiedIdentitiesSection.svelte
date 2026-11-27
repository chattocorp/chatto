<!--
@component
Lists the self-verification identities available for this profile. Popups show
the list in a menu section. The plain variant renders only the list, for a host
that frames it, such as the Profile View's details card.
-->
<script lang="ts">
  import type { VerifiedIdentity } from '@chatto/client/api/users';
  import { providerIcon } from '$lib/auth/providerIcon';
  import { m } from '$lib/i18n/messages';
  import { SelfVerificationItem } from '$lib/ui';

  let {
    identities,
    plain = false
  }: {
    identities: VerifiedIdentity[];
    plain?: boolean;
  } = $props();

  const label = $derived(m('chat.profile.verified_identities'));
</script>

{#snippet identityList()}
  <ul aria-label={label} class="flex min-w-0 flex-col gap-1">
    {#each identities as entry (JSON.stringify([entry.method, entry.value]))}
      <li class="max-w-full leading-none">
        <SelfVerificationItem
          method={entry.methodLabel}
          icon={providerIcon(entry.method)}
          identity={entry.value}
        />
      </li>
    {/each}
  </ul>
{/snippet}

{#if identities.length}
  {#if plain}
    {@render identityList()}
  {:else}
    <section class="menu-section px-3 py-2 text-start">
      {@render identityList()}
    </section>
  {/if}
{/if}
