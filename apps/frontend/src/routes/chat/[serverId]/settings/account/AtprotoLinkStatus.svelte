<!--
@component
Status line of a linked AT Protocol sign-in identity: the verified handle, or
"Linked" while other members' apps cannot verify it (FDR-056). **Verify** then
signs in to the PDS and writes the account link record. Each server's app can
write only that server's records, so for another server this links to its app.
-->
<script lang="ts">
  import type { ChattoAccountRef } from '@chatto/client/atproto/accountLink';
  import {
    atprotoLinkVerifier,
    atprotoOAuthClient,
    ensureAtprotoSession,
    restoreAtprotoAccount
  } from '$lib/atproto/session.svelte';
  import { canWriteAtprotoLinkRecord, ensureAtprotoLinkRecord } from '$lib/atproto/signIn';
  import VerifiedAtprotoHandle from '$lib/components/atproto/VerifiedAtprotoHandle.svelte';
  import { m } from '$lib/i18n/messages';

  let { did, account }: { did: string; account: ChattoAccountRef } = $props();

  const serverOrigin = $derived(new URL(account.serverOrigin).origin);
  const writableHere = $derived(canWriteAtprotoLinkRecord(account));

  /** Increments after a repair, so the verification runs again. */
  let attempt = $state(0);
  let busy = $state(false);
  let failed = $state(false);
  const verification = $derived.by(() => {
    void attempt;
    return atprotoLinkVerifier.verify(did, account);
  });

  /**
   * Load the OAuth library and stored session before the click: browsers
   * block the sign-in window when the click handler waits for them first.
   */
  function prepareSignIn() {
    void restoreAtprotoAccount();
    atprotoOAuthClient().catch(() => {});
  }

  async function verify() {
    busy = true;
    failed = false;
    try {
      const session = await ensureAtprotoSession(did);
      if (session.did !== did) throw new Error('wrong account');
      await ensureAtprotoLinkRecord(account, did);
      attempt++;
    } catch {
      failed = true;
    } finally {
      busy = false;
    }
  }
</script>

{#await verification}
  {m('settings.account.sso.linked')}
{:then result}
  {#if result.status === 'verified'}
    <div class="flex">
      {#key attempt}<VerifiedAtprotoHandle {did} {account} icon={false} />{/key}
    </div>
  {:else}
    <p>
      {m('settings.account.sso.linked')} ·
      {failed ? m('settings.atproto.link_failed') : m('settings.atproto.unverified_no_link')}
      {#if writableHere}
        <button
          type="button"
          class="cursor-pointer link"
          disabled={busy}
          aria-busy={busy}
          {@attach prepareSignIn}
          onclick={() => void verify()}>{m('settings.atproto.verify_button')}</button
        >
      {:else}
        <a
          class="link"
          href={`${serverOrigin}/chat/-/settings/account`}
          target="_blank"
          rel="external noopener noreferrer"
          >{m('settings.atproto.verify_on_server', { server: new URL(serverOrigin).host })}</a
        >
      {/if}
    </p>
  {/if}
{/await}
