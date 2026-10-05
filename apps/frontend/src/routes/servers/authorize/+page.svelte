<!--
@component

Launch page of an authorization window. The opening client stores the target
URL in a launch record after its asynchronous preparation; this page then
replaces itself with that URL. See `openAuthorizationWindow`.
-->
<script lang="ts">
  import { page } from '$app/state';
  import Deadline from '$lib/lifecycle/Deadline.svelte';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { m } from '$lib/i18n/messages';
  import { replaceLaunchPage, readAuthorizationLaunchTarget } from '$lib/oauth/authorizationWindow';
  import { EmptyState, LoadingPage, PageTitle } from '$lib/ui';

  const LAUNCH_POLL_INTERVAL_MS = 100;
  // The opener fetches server information with a 10-second timeout first.
  const LAUNCH_TIMEOUT_MS = 30_000;

  const launchId = page.url.hash.slice(1);
  const startedAt = Date.now();
  let status = $state<'waiting' | 'launching' | 'error'>(launchId ? 'waiting' : 'error');

  function launch() {
    // Firefox for Android can also load this page in a detached, hidden window.
    // Only the window that the user sees continues to the target.
    if (status !== 'waiting' || document.visibilityState !== 'visible') return;
    try {
      const target = readAuthorizationLaunchTarget(launchId);
      if (!target) return;
      status = 'launching';
      replaceLaunchPage(target);
    } catch {
      status = 'error';
    }
  }
</script>

<PageTitle title={m('auth.callback.connecting_title')} />

{#if status === 'waiting'}
  <Interval milliseconds={LAUNCH_POLL_INTERVAL_MS} ontick={launch} />
  <Deadline
    at={startedAt + LAUNCH_TIMEOUT_MS}
    onreached={() => {
      if (status === 'waiting') status = 'error';
    }}
  />
{/if}

<div class="flex min-h-0 flex-1 flex-col p-8">
  {#if status === 'error'}
    <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('auth.callback.failed_title')}>
      <p class="max-w-md">{m('auth.launch.failed')}</p>
    </EmptyState>
  {:else}
    <LoadingPage message={m('auth.launch.opening')} />
  {/if}
</div>
