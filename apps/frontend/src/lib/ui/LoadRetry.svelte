<!--
@component
Fallback for a part of the interface that failed to load, such as a lazily
loaded component or a message timeline. It shows a message and a Retry button.
Pass `secondaryAction` for one more button, such as one that closes the failed
pane.

It also retries without the user while the tab is visible: at an interval,
when the browser goes online, and when the tab becomes visible. Set
`autoRetry` to false for a failure that another attempt cannot repair.

It imports `Button` directly, not through `$lib/ui/form`, so deferred room
bundles do not load the complete form barrel.
-->
<script lang="ts">
  import type { ClassValue } from 'svelte/elements';
  import { m } from '$lib/i18n/messages';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import Button from './form/Button.svelte';

  /** Time between automatic attempts. Requests repeat reads before they fail. */
  const AUTO_RETRY_MS = 5_000;

  let {
    onretry,
    message = m('common.error.network'),
    autoRetry = true,
    secondaryAction,
    class: className
  }: {
    /** Starts a new load attempt. */
    onretry: () => void;
    /** Explains the failure. Defaults to the network error message. */
    message?: string;
    /** Whether to retry without the user. Use it for network failures only. */
    autoRetry?: boolean;
    /** A second button after Retry. It uses the same secondary style. */
    secondaryAction?: { label: string; onclick: () => void };
    /** Sizing and placement classes for the container. */
    class?: ClassValue;
  } = $props();

  /** A hidden tab waits; it retries when it becomes visible. */
  function retryWhenVisible() {
    if (autoRetry && document.visibilityState === 'visible') onretry();
  }
</script>

<svelte:window ononline={retryWhenVisible} />
<svelte:document onvisibilitychange={retryWhenVisible} />
{#if autoRetry}
  <Interval milliseconds={AUTO_RETRY_MS} ontick={retryWhenVisible} />
{/if}

<div
  role="alert"
  class={['flex flex-col items-center justify-center gap-3 p-4 text-center', className]}
>
  <p class="text-sm text-muted">{message}</p>
  <div class="flex gap-2">
    <Button variant="secondary" onclick={onretry}>{m('common.retry')}</Button>
    {#if secondaryAction}
      <Button variant="secondary" onclick={secondaryAction.onclick}>{secondaryAction.label}</Button>
    {/if}
  </div>
</div>
