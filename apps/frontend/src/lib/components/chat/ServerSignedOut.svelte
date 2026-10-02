<!--
@component

Replaces the server chrome when the client knows a remote server but has no
usable session for it: the user joined the server and did not sign in yet,
signed out of it, or the server rejected the stored session and no chat data
is loaded. `showsServerSignedOut` decides this. The view names the server and
lets the user log in. Only the button starts sign-in; opening the route does
not. The origin server never uses this view: without a session, its route
redirects to `/login`; during reauthentication, it keeps the server chrome and
the reconnect notice.
-->
<script lang="ts">
  import ServerStatusView from './ServerStatusView.svelte';
  import { isRemoteSignInPending, startRemoteSignIn } from '$lib/auth/remoteSignIn.svelte';
  import { m } from '$lib/i18n/messages';
  import type { RegisteredServer } from '@chatto/client/server/registry';
  import { Button } from '$lib/ui/form';

  let {
    registration
  }: {
    /** Saved catalogue entry of the remote server. Sign-in uses its URL. */
    registration: RegisteredServer;
  } = $props();
</script>

<ServerStatusView
  {registration}
  title={m('chat.server_signed_out.title')}
  body={m('chat.server_signed_out.body')}
  testId="server-signed-out"
>
  <Button
    loading={isRemoteSignInPending(registration.id)}
    onclick={() => void startRemoteSignIn(registration)}
  >
    {m('chat.server_gutter.log_in')}
  </Button>
</ServerStatusView>
