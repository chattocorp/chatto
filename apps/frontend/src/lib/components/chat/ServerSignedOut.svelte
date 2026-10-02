<!--
@component

Replaces the server chrome when the client knows a remote server but cannot
use a session for it: the user signed out, or the server rejected the stored
session and no chat data is loaded. `showsServerSignedOut` decides this. The
view names the server and lets the user log in again. Only the button starts
sign-in; opening the route does not. The origin server does not use this view:
its route redirects to the origin's own sign-in page.
-->
<script lang="ts">
  import ServerStatusView from './ServerStatusView.svelte';
  import { startRemoteReauthentication } from '$lib/auth/reauth';
  import { m } from '$lib/i18n/messages';
  import type { RegisteredServer } from '@chatto/client/server/registry';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  let {
    registration
  }: {
    /** Saved catalogue entry of the remote server. Sign-in uses its URL. */
    registration: RegisteredServer;
  } = $props();

  let signingIn = $state(false);

  async function logIn(): Promise<void> {
    if (signingIn) return;
    signingIn = true;
    try {
      await startRemoteReauthentication(registration);
    } catch {
      toast.error(m('add_server.start_failed'));
    } finally {
      signingIn = false;
    }
  }
</script>

<ServerStatusView
  {registration}
  title={m('chat.server_signed_out.title')}
  body={m('chat.server_signed_out.body')}
  testId="server-signed-out"
>
  <Button loading={signingIn} onclick={() => void logIn()}>
    {m('chat.server_gutter.log_in')}
  </Button>
</ServerStatusView>
