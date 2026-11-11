<!--
@component
Verification state of the account's published AT Protocol identity, as other
members' apps see it (FDR-056). When they cannot verify it, **Verify** signs
in to the PDS and writes the account link record.
-->
<script lang="ts">
  import type { ChattoAccountRef } from '@chatto/client/atproto/accountLink';
  import { atprotoLinkVerifier, ensureAtprotoSession } from '$lib/atproto/session.svelte';
  import { ensureAtprotoLinkRecord } from '$lib/atproto/signIn';
  import VerifiedAtprotoHandle from '$lib/components/atproto/VerifiedAtprotoHandle.svelte';
  import { m } from '$lib/i18n/messages';
  import { Button } from '$lib/ui/form';

  let { did, account }: { did: string; account: ChattoAccountRef } = $props();

  /** Increments after a repair, so the verification runs again. */
  let attempt = $state(0);
  let busy = $state(false);
  let failed = $state(false);
  const verification = $derived.by(() => {
    void attempt;
    return atprotoLinkVerifier.verify(did, account);
  });

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

{#await verification then result}
  {#if result.status === 'verified'}
    <div class="flex">
      {#key attempt}<VerifiedAtprotoHandle {did} {account} />{/key}
    </div>
  {:else}
    <p>
      {failed ? m('settings.atproto.link_failed') : m('settings.atproto.unverified_no_link')}
    </p>
    <Button variant="ghost" size="xs" loading={busy} disabled={busy} onclick={() => void verify()}>
      {m('settings.atproto.verify_button')}
    </Button>
  {/if}
{/await}
