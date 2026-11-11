<!--
@component
Shows a user's AT Protocol handle after this client verified the user's
account link record (FDR-056). The server's DID claim alone shows nothing.
Verification contacts the identity's DID directory, handle domain, and PDS,
so mount this only where a profile is open.
-->
<script lang="ts">
  import type { ChattoAccountRef } from '@chatto/client/atproto/accountLink';
  import { atprotoLinkVerifier } from '$lib/atproto/session.svelte';
  import { m } from '$lib/i18n/messages';

  let {
    did,
    account,
    icon = true,
    unlessHandle
  }: {
    did: string;
    account: ChattoAccountRef;
    /** Show the `@` mark before the handle. Hide it where a row has its own icon. */
    icon?: boolean;
    /**
     * Show nothing when the verified handle equals this value, for example the
     * username that `AtprotoUsername` already marks as verified.
     */
    unlessHandle?: string;
  } = $props();

  const verification = $derived(atprotoLinkVerifier.verify(did, account));

  function sameHandle(handle: string, other: string | undefined): boolean {
    return other !== undefined && handle.toLowerCase() === other.toLowerCase();
  }
</script>

{#await verification then result}
  {#if result.status === 'verified' && !sameHandle(result.handle, unlessHandle)}
    <a
      class="inline-flex min-w-0 items-center gap-1 link"
      href={`https://bsky.app/profile/${encodeURIComponent(result.handle)}`}
      target="_blank"
      rel="noopener noreferrer"
      title={m('chat.profile.atproto_verified')}
      data-testid="verified-atproto-handle"
    >
      {#if icon}<span aria-hidden="true" class="iconify icon-[mdi--at] shrink-0"></span>{/if}
      <span class="truncate" dir="ltr">@{result.handle}</span>
      <span class="sr-only">{m('chat.profile.atproto_verified')}</span>
    </a>
  {/if}
{/await}
