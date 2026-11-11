<!--
@component
Publishes or removes the account's AT Protocol identity (FDR-056). The device
signs in to the user's PDS itself, writes the account link record, and then
publishes the DID on the profile. The server never receives PDS tokens.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { CurrentUserState } from '@chatto/client/auth/currentUser';
  import { createAccountAPI } from '@chatto/client/api/account';
  import {
    deleteAccountLinkRecord,
    writeAccountLinkRecord,
    type AtprotoVerification,
    type ChattoAccountRef
  } from '@chatto/client/atproto/accountLink';
  import {
    atprotoAccount,
    atprotoLinkVerifier,
    atprotoOAuthClient,
    atprotoRepoSession,
    connectAtprotoAccount,
    restoreAtprotoAccount
  } from '$lib/atproto/session.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { ConnectError } from '@connectrpc/connect';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { Hint, LoadingFog, Panel } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';

  let { currentUser }: { currentUser: CurrentUserState } = $props();

  const serverScope = useServerScope();
  const userId = $derived(serverScope.store.accountId ?? '');
  const publishedDid = $derived(currentUser.user?.atprotoDid ?? null);
  const accountRef = $derived<ChattoAccountRef>({
    serverOrigin: serverScope.connection.origin,
    userId
  });
  const knownAccount = $derived(atprotoAccount());

  /** Text that the user typed; `null` shows the handle of the device session. */
  let typedHandle = $state<string | null>(null);
  const handle = $derived((typedHandle ?? knownAccount?.handle ?? '').trim().replace(/^@/, ''));
  let busy = $state<'link' | 'unlink' | null>(null);
  let error = $state('');

  onMount(() => {
    // Load the OAuth library and the device session while the panel is open,
    // so a sign-in popup opens directly from the click.
    void atprotoOAuthClient().catch(() => {});
    void restoreAtprotoAccount();
  });

  // Verify the published claim like every other client does.
  const verification = $derived(
    publishedDid && userId ? atprotoLinkVerifier.verify(publishedDid, accountRef) : null
  );

  /** The user signed in to another AT Protocol account than the published one. */
  class WrongAccountError extends Error {}

  // PDS and OAuth errors carry English text from the libraries; show the
  // localized messages instead.
  function failureMessage(err: unknown, fallback: string): string {
    if (err instanceof WrongAccountError) return m('settings.atproto.wrong_account');
    return err instanceof ConnectError ? errorMessage(err, fallback) : fallback;
  }

  function accountAPI() {
    return serverScope.connection.getAPI(createAccountAPI);
  }

  /** Sign in when the device has no session for `did`, or for any DID when `did` is null. */
  async function repoSession(did: string | null) {
    const existing = did ? await atprotoRepoSession(did) : null;
    if (existing) return existing;
    const connected = await connectAtprotoAccount(did ?? handle);
    if (did && connected.did !== did) throw new WrongAccountError();
    const session = await atprotoRepoSession(connected.did);
    if (!session) throw new Error('The AT Protocol session is not available.');
    return session;
  }

  async function link(event: SubmitEvent) {
    event.preventDefault();
    if (busy || !userId) return;
    busy = 'link';
    error = '';
    const account = accountRef;
    const linkedUserId = userId;
    try {
      const session = await repoSession(
        knownAccount && (!handle || handle === knownAccount.handle) ? knownAccount.did : null
      );
      // Write the proof first: a published DID without a record never verifies.
      await writeAccountLinkRecord(session, account);
      let did: string;
      try {
        did = await accountAPI().setAtprotoIdentity(session.did);
      } catch (err) {
        // Older servers do not have this RPC. Do not leave an unused record.
        await deleteAccountLinkRecord(session, account).catch(() => {});
        throw err;
      }
      atprotoLinkVerifier.forget(did, account);
      currentUser.update(linkedUserId, () => ({ atprotoDid: did }));
    } catch (err) {
      error = failureMessage(err, m('settings.atproto.link_failed'));
    } finally {
      busy = null;
    }
  }

  async function unlink() {
    const did = publishedDid;
    if (busy || !did) return;
    busy = 'unlink';
    error = '';
    const account = accountRef;
    const linkedUserId = userId;
    try {
      // Sign in first, so a sign-in popup still opens from the click.
      const session = await repoSession(did);
      // Remove the profile claim first. A remaining record alone shows nothing.
      await accountAPI().clearAtprotoIdentity();
      currentUser.update(linkedUserId, () => ({ atprotoDid: null }));
      await deleteAccountLinkRecord(session, account);
      atprotoLinkVerifier.forget(did, account);
    } catch (err) {
      error = failureMessage(err, m('settings.atproto.unlink_failed'));
    } finally {
      busy = null;
    }
  }

  function unverifiedMessage(
    reason: Extract<AtprotoVerification, { status: 'unverified' }>['reason']
  ) {
    switch (reason) {
      case 'unavailable':
        return m('settings.atproto.unverified_unavailable');
      case 'no_link':
        return m('settings.atproto.unverified_no_link');
      default:
        return m('settings.atproto.unverified_handle');
    }
  }
</script>

<Panel title={m('settings.atproto.title')} icon="iconify icon-[ri--bluesky-fill]">
  <div class="flex max-w-md flex-col gap-4">
    <p class="text-sm text-muted">{m('settings.atproto.description')}</p>
    {#if error}
      <Hint tone="danger">{error}</Hint>
    {/if}

    {#if publishedDid}
      {#await verification}
        <LoadingFog class="h-16 w-full" label={m('settings.atproto.verifying')} />
      {:then result}
        {#if result?.status === 'verified'}
          <Hint tone="success">{m('settings.atproto.verified', { handle: result.handle })}</Hint>
        {:else if result}
          <Hint tone="warning">{unverifiedMessage(result.reason)}</Hint>
        {/if}
      {/await}
      <p class="text-sm text-muted">{m('settings.atproto.unlink_description')}</p>
      <div>
        <Button
          variant="danger-secondary"
          size="sm"
          loading={busy === 'unlink'}
          disabled={busy !== null}
          onclick={() => void unlink()}
        >
          <span aria-hidden="true" class="iconify icon-[uil--link-broken]"></span>
          {m('settings.atproto.unlink_button')}
        </Button>
      </div>
    {:else}
      <form class="flex flex-col gap-3" onsubmit={link}>
        <TextInput
          id="atproto-handle"
          label={m('settings.atproto.handle_label')}
          description={m('settings.atproto.handle_description')}
          placeholder="alice.bsky.social"
          autocomplete="username"
          value={typedHandle ?? knownAccount?.handle ?? ''}
          oninput={(event: Event) =>
            (typedHandle = (event.currentTarget as HTMLInputElement).value)}
          disabled={busy !== null}
        />
        <p class="text-sm text-muted">{m('settings.atproto.privacy_notice')}</p>
        <div>
          <Button
            type="submit"
            variant="secondary"
            size="sm"
            loading={busy === 'link'}
            disabled={busy !== null}
          >
            <span aria-hidden="true" class="iconify icon-[uil--link]"></span>
            {m('settings.atproto.link_button')}
          </Button>
        </div>
      </form>
    {/if}
  </div>
</Panel>
