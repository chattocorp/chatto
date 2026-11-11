<!--
@component
A username as `@login`. When the user's verified AT Protocol handle equals the
login, the `@` shows in the accent colour and names the verification for
assistive technology (FDR-056). Verification contacts external systems, so
pass `did` only where a profile is open or for the current user; elsewhere
pass no `did`.
-->
<script lang="ts">
  import type { ChattoAccountRef } from '@chatto/client/atproto/accountLink';
  import { atprotoLinkVerifier } from '$lib/atproto/session.svelte';
  import { m } from '$lib/i18n/messages';

  let {
    login,
    did = null,
    account
  }: {
    login: string;
    /** Published AT Protocol DID of the user; `null` without one. */
    did?: string | null;
    account: ChattoAccountRef;
  } = $props();

  const verification = $derived(did ? atprotoLinkVerifier.verify(did, account) : null);
</script>

<bdi dir="ltr">
  {#await verification}
    @{login}
  {:then result}
    {#if result?.status === 'verified' && result.handle.toLowerCase() === login.toLowerCase()}
      <span
        role="img"
        aria-label={m('chat.profile.atproto_verified')}
        title={m('chat.profile.atproto_verified')}
        class="text-action"
        data-testid="verified-atproto-username">@</span
      >{login}
    {:else}
      @{login}
    {/if}
  {/await}
</bdi>
