<!--
@component
Reply and edit state for the message composer. The composer renders these rows
above its input surface so that they never move the input. The rows have no
fill of their own; they sit directly on the room's work plane.
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import type { AccountNameIdentity } from '$lib/render/accountName';
  import { m } from '$lib/i18n/messages';

  let {
    inReplyTo,
    replyDisplayName,
    replyIdentity,
    replyExcerpt,
    isEditing,
    oncancelreply,
    oncanceledit
  }: {
    inReplyTo?: string;
    replyDisplayName?: string;
    replyIdentity?: AccountNameIdentity;
    replyExcerpt?: string;
    isEditing: boolean;
    oncancelreply: () => void;
    oncanceledit: () => void;
  } = $props();
</script>

{#if inReplyTo && replyDisplayName}
  <div data-testid="reply-indicator" class="flex items-center justify-between gap-2 px-0.5 text-sm">
    <span class="min-w-0 truncate text-text">
      {m('composer.replying_to')}
      <strong class="inline-flex max-w-full min-w-0 font-semibold"
        ><AccountName name={replyDisplayName} identity={replyIdentity} /></strong
      >
      {#if replyExcerpt}
        <span class="text-muted"> &mdash; {replyExcerpt}</span>
      {/if}
    </span>
    <button
      type="button"
      onclick={oncancelreply}
      class="hidden shrink-0 cursor-pointer items-center gap-1 text-muted transition-colors feedback-quick hover:text-text sm:flex"
    >
      <kbd class="keycap">Esc</kbd>
      {m('composer.esc_to_cancel')}
    </button>
    <button
      type="button"
      onclick={oncancelreply}
      class="shrink-0 cursor-pointer text-muted transition-colors feedback-quick hover:text-text sm:hidden"
    >
      {m('common.cancel')}
    </button>
  </div>
{/if}

{#if isEditing}
  <div class="flex items-center justify-between gap-2 px-0.5 text-sm">
    <span class="text-text">{m('composer.editing')}</span>
    <button
      type="button"
      onclick={oncanceledit}
      class="hidden cursor-pointer items-center gap-1 text-muted transition-colors feedback-quick hover:text-text sm:flex"
    >
      <kbd class="keycap">Esc</kbd>
      {m('composer.esc_to_cancel')}
    </button>
    <button
      type="button"
      onclick={oncanceledit}
      class="shrink-0 cursor-pointer text-muted transition-colors feedback-quick hover:text-text sm:hidden"
    >
      {m('common.cancel')}
    </button>
  </div>
{/if}
