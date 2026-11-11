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
    icon = true
  }: {
    did: string;
    account: ChattoAccountRef;
    /** Show the `@` mark before the handle. Hide it where a row has its own icon. */
    icon?: boolean;
  } = $props();

  const verification = $derived(atprotoLinkVerifier.verify(did, account));
</script>

{#await verification then result}
  {#if result.status === 'verified'}
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
      <span
        role="img"
        aria-label={m('chat.profile.atproto_verified')}
        class="iconify icon-[uil--check-circle] shrink-0 text-success"
      ></span>
    </a>
  {/if}
{/await}
