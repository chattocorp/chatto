<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { loadAndClearFlowState } from '$lib/oauth/pkce';
  import {
    oauthPopupChannelName,
    oauthPopupResponseFromURL,
    type OAuthPopupResponse
  } from '$lib/oauth/popup';
  import { completeServerOAuthFlow } from '$lib/auth/reauth';
  import { serverIdToSegment } from '$lib/navigation';
  import * as m from '$lib/i18n/messages';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';

  let status = $state<'loading' | 'error'>('loading');
  let errorMessage = $state('');

  function returnToOpeningClient(response: OAuthPopupResponse) {
    let delivered = false;

    if (typeof BroadcastChannel !== 'undefined') {
      const channel = new BroadcastChannel(oauthPopupChannelName(response.state));
      channel.postMessage(response);
      window.setTimeout(() => channel.close(), 100);
      delivered = true;
    }

    if (window.opener && !window.opener.closed) {
      window.opener.postMessage(response, window.location.origin);
      delivered = true;
    }

    if (delivered) {
      // Give BroadcastChannel/postMessage a task boundary before closing the
      // script-opened popup. Browsers that refuse window.close keep showing
      // the harmless completion state instead.
      window.setTimeout(() => window.close(), 100);
    }
    return delivered;
  }

  onMount(async () => {
    if (page.url.searchParams.get('mode') === 'popup') {
      const popupResponse = oauthPopupResponseFromURL(page.url);
      if (!popupResponse) {
        status = 'error';
        errorMessage = m['auth.callback.no_code']();
        return;
      }
      if (!returnToOpeningClient(popupResponse)) {
        status = 'error';
        errorMessage = m['auth.callback.missing_flow']();
      }
      return;
    }

    const code = page.url.searchParams.get('code');
    const state = page.url.searchParams.get('state');
    const errorParam = page.url.searchParams.get('error');

    // Handle error responses from the authorization server
    if (errorParam) {
      status = 'error';
      errorMessage =
        page.url.searchParams.get('error_description') ||
        m['auth.callback.authorization_failed']({ error: errorParam });
      return;
    }

    if (!code) {
      status = 'error';
      errorMessage = m['auth.callback.no_code']();
      return;
    }

    // Load the saved flow state (verifier, remote URL, etc.)
    const flow = loadAndClearFlowState();
    if (!flow) {
      status = 'error';
      errorMessage = m['auth.callback.missing_flow']();
      return;
    }

    // Validate state parameter (CSRF protection)
    if (state !== flow.state) {
      status = 'error';
      errorMessage = m['auth.callback.invalid_state']();
      return;
    }

    // Build the redirect_uri that legacy full-page flows used in the
    // authorize request.
    const redirectUri = `${window.location.origin}/servers/callback`;

    try {
      const serverId = await completeServerOAuthFlow(flow, code, redirectUri);
      await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }));
    } catch (err) {
      status = 'error';
      if (err instanceof DOMException && err.name === 'AbortError') {
        errorMessage = m['auth.callback.token_exchange_timeout']();
      } else {
        errorMessage = m['auth.callback.token_exchange_failed']();
      }
    }
  });
</script>

<PageTitle title={m['auth.callback.connecting_title']()} />

<div class="flex min-h-0 flex-1 items-center justify-center p-8">
  {#if status === 'loading'}
    <div class="flex flex-col items-center gap-4">
      <span class="iconify animate-spin text-3xl text-muted mdi--loading"></span>
      <p class="text-muted">{m['auth.callback.completing']()}</p>
    </div>
  {:else}
    <div class="flex max-w-md flex-col items-center gap-4 text-center">
      <span class="iconify text-4xl text-danger uil--exclamation-triangle"></span>
      <p class="font-medium">{m['auth.callback.failed_title']()}</p>
      <p class="text-sm text-muted">{errorMessage}</p>
      <Button href={resolve('/')} variant="secondary">{m['common.retry']()}</Button>
    </div>
  {/if}
</div>
