<!--
@component
Fallback for a lazily loaded part of the interface that failed to load. It
shows a message and a Retry button. Pass `secondaryAction` for one more
button, such as one that closes the failed pane.

It imports `Button` directly, not through `$lib/ui/form`, so deferred room
bundles do not load the complete form barrel.
-->
<script lang="ts">
  import type { ClassValue } from 'svelte/elements';
  import { m } from '$lib/i18n/messages';
  import Button from './form/Button.svelte';

  let {
    onretry,
    message = m('common.error.network'),
    secondaryAction,
    class: className
  }: {
    /** Starts a new load attempt. */
    onretry: () => void;
    /** Explains the failure. Defaults to the network error message. */
    message?: string;
    /** A second button after Retry. It uses the same secondary style. */
    secondaryAction?: { label: string; onclick: () => void };
    /** Sizing and placement classes for the container. */
    class?: ClassValue;
  } = $props();
</script>

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
