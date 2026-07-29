<script lang="ts">
  import * as m from '$lib/i18n/messages';

  let {
    inReplyTo,
    replyDisplayName,
    replyExcerpt,
    isEditing,
    oncancelreply,
    oncanceledit
  }: {
    inReplyTo?: string;
    replyDisplayName?: string;
    replyExcerpt?: string;
    isEditing: boolean;
    oncancelreply: () => void;
    oncanceledit: () => void;
  } = $props();
</script>

{#if inReplyTo && replyDisplayName}
  <div
    data-testid="reply-indicator"
    class="flex items-center justify-between rounded-md bg-surface-emphasized px-3 py-2 text-sm"
  >
    <span class="min-w-0 truncate text-text">
      {m['composer.replying_to']()} <strong>{replyDisplayName}</strong>
      {#if replyExcerpt}
        <span class="text-muted"> &mdash; {replyExcerpt}</span>
      {/if}
    </span>
    <button
      type="button"
      onclick={oncancelreply}
      class="hidden shrink-0 cursor-pointer items-center gap-1 text-muted transition-colors hover:text-text sm:flex"
    >
      <kbd class="rounded bg-surface-strong px-1.5 py-0.5 text-xs">Esc</kbd>
      {m['composer.esc_to_cancel']()}
    </button>
    <button
      type="button"
      onclick={oncancelreply}
      class="shrink-0 cursor-pointer rounded bg-surface-strong px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-selected sm:hidden"
    >
      {m['common.cancel']()}
    </button>
  </div>
{/if}

{#if isEditing}
  <div class="flex items-center justify-between rounded-md bg-surface-emphasized px-3 py-2 text-sm">
    <span class="text-text">{m['composer.editing']()}</span>
    <button
      type="button"
      onclick={oncanceledit}
      class="hidden cursor-pointer items-center gap-1 text-muted transition-colors hover:text-text sm:flex"
    >
      <kbd class="rounded bg-surface-strong px-1.5 py-0.5 text-xs">Esc</kbd>
      {m['composer.esc_to_cancel']()}
    </button>
    <button
      type="button"
      onclick={oncanceledit}
      class="cursor-pointer rounded bg-surface-strong px-2.5 py-1 text-xs font-medium text-text transition-colors hover:bg-surface-selected sm:hidden"
    >
      {m['common.cancel']()}
    </button>
  </div>
{/if}
