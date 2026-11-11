<!-- @component Completes an AT Protocol sign-in that `connectAtprotoAccount` started in a popup. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { completeAtprotoCallback } from '$lib/atproto/session.svelte';
  import { m } from '$lib/i18n/messages';
  import { EmptyState, LoadingPage, PageTitle } from '$lib/ui';

  let status = $state<'loading' | 'complete' | 'failed'>('loading');

  onMount(() => {
    // The opening window receives the result. A browser can refuse to close
    // this window; it then asks the user to close it.
    completeAtprotoCallback().then(
      () => (status = 'complete'),
      () => (status = 'failed')
    );
  });
</script>

<PageTitle title={m('auth.atproto.callback_title')} />

{#if status === 'complete'}
  <div class="pane-page">
    <EmptyState icon="icon-[uil--check-circle]" title={m('auth.callback.complete_title')}>
      <p class="max-w-md">{m('auth.callback.complete_close')}</p>
    </EmptyState>
  </div>
{:else if status === 'failed'}
  <div class="pane-page">
    <EmptyState
      icon="icon-[uil--exclamation-triangle]"
      title={m('auth.atproto.callback_failed')}
    >
      <p class="max-w-md">{m('auth.callback.complete_close')}</p>
    </EmptyState>
  </div>
{:else}
  <LoadingPage />
{/if}
