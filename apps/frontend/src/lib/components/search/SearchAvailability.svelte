<!--
@component

Shows search availability and retry controls. Renders children when search can
be used. The owner can frame status content and set its loading-state layout.
Request ownership stays with the caller.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import { MessageSearchState } from '$lib/api-client/messageSearch';
  import { m } from '$lib/i18n/messages';
  import { EmptyState } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let {
    state,
    checking,
    error,
    onRetry,
    checkingClass,
    frame,
    children
  }: {
    state: MessageSearchState;
    checking: boolean;
    error: boolean;
    onRetry: () => void;
    checkingClass: ClassValue;
    /** Optional surface-owned frame for the status message. */
    frame?: Snippet<[Snippet, boolean]>;
    children: Snippet;
  } = $props();

  const unavailable = $derived(error || state === MessageSearchState.UNAVAILABLE);
  const indexing = $derived(
    state === MessageSearchState.STARTING || state === MessageSearchState.INDEXING
  );
</script>

{#snippet statusContent()}
  {#if checking}
    <div class={checkingClass} aria-live="polite">
      <span class="iconify me-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"></span>
      {m('search.checking')}
    </div>
  {:else if unavailable}
    <EmptyState icon="icon-[uil--cloud-slash]" title={m('search.unavailable.title')}>
      <p>{m('search.unavailable.description')}</p>
      <div class="mt-4">
        <Button variant="secondary" onclick={onRetry}>{m('common.retry')}</Button>
      </div>
    </EmptyState>
  {:else if state === MessageSearchState.DISABLED}
    <EmptyState icon="icon-[uil--search-alt]" title={m('search.disabled.title')}>
      {m('search.disabled.description')}
    </EmptyState>
  {:else if indexing}
    <EmptyState icon="icon-[uil--database]" title={m('search.indexing.title')}>
      <p>{m('search.indexing.description')}</p>
      <div class="mt-4">
        <Button variant="secondary" onclick={onRetry}>{m('search.check_again')}</Button>
      </div>
    </EmptyState>
  {/if}
{/snippet}

{#if checking || unavailable || state === MessageSearchState.DISABLED || indexing}
  {#if frame}
    {@render frame(statusContent, checking)}
  {:else}
    {@render statusContent()}
  {/if}
{:else}
  {@render children()}
{/if}
