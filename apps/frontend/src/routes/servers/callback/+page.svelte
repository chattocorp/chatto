<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import {
    oauthPopupChannelName,
    oauthPopupResponseFromURL,
    type OAuthPopupResponse
  } from '$lib/oauth/popup';
  import { m } from '$lib/i18n/messages';
  import { EmptyState, LoadingPage, PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let status = $state<'loading' | 'complete' | 'error'>('loading');
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
      // popup. A browser can refuse window.close, for example in a Firefox for
      // Android Custom Tab; the window then tells the user to close it.
      window.setTimeout(() => {
        window.close();
        status = 'complete';
      }, 100);
    }
    return delivered;
  }

  // Each registered callback carries a mode. A callback without one has no
  // waiting client, so this page never exchanges a code itself.
  onMount(() => {
    const popupResponse = ['popup', 'provider'].includes(page.url.searchParams.get('mode') ?? '')
      ? oauthPopupResponseFromURL(page.url)
      : null;
    if (!popupResponse) {
      status = 'error';
      errorMessage = m('auth.callback.no_code');
      return;
    }
    if (!returnToOpeningClient(popupResponse)) {
      status = 'error';
      errorMessage = m('auth.callback.missing_flow');
    }
  });
</script>

<PageTitle title={m('auth.callback.connecting_title')} />

<div class="flex min-h-0 flex-1 flex-col p-8">
  {#if status === 'loading'}
    <LoadingPage message={m('auth.callback.completing')} />
  {:else if status === 'complete'}
    <EmptyState icon="icon-[uil--check-circle]" title={m('auth.callback.complete_title')}>
      <p class="max-w-md">{m('auth.callback.complete_close')}</p>
    </EmptyState>
  {:else}
    <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('auth.callback.failed_title')}>
      <div class="flex max-w-md flex-col items-center gap-4">
        <p>{errorMessage}</p>
        <Button href={resolve('/')} variant="secondary">{m('common.retry')}</Button>
      </div>
    </EmptyState>
  {/if}
</div>
