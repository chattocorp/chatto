<!--
@component

Full emoji picker with search and categories.
Pure content component — rendered inside a ContextMenu by the parent.
Uses the same section styling as MessageActionMenu (rounded-md bg-background sections).

**Props:**
- `serverId` - The active server. Used to scope the per-server "Recently Used" list.
- `onSelect` - Callback when an emoji is selected
- `onClose` - Callback to dismiss the picker (Escape key)
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { searchEmojis, EMOJI_BY_CATEGORY } from '$lib/emoji';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
  import { getRecentEmojis, MAX_RECENT_EMOJIS } from '$lib/state/recentEmojis.svelte';

  let {
    serverId,
    onSelect,
    onClose
  }: {
    serverId: string;
    onSelect: (emoji: string) => void;
    onClose: () => void;
  } = $props();

  let query = $state('');

  const recentStore = $derived(getRecentEmojis(serverId));
  const recent = $derived(recentStore.recent.slice(0, MAX_RECENT_EMOJIS));

  const searchResults = $derived(query.trim() ? searchEmojis(query.trim(), 50) : []);
  const isSearching = $derived(query.trim().length > 0);

  function focusSearchInput(node: HTMLInputElement) {
    if (shouldAutoFocus()) queueMicrotask(() => node.focus());
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (query) {
        query = '';
        e.stopPropagation();
      } else {
        onClose();
      }
    }
  }

  function selectEmoji(emoji: string) {
    recentStore.record(emoji);
    onSelect(emoji);
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="flex w-88 max-w-full flex-col gap-2 compact-input:w-72 compact-input:gap-1" onkeydown={handleKeydown}>
  <!-- Search section -->
  <div class="menu-section p-2 compact-input:p-1">
    <input
      {@attach focusSearchInput}
      bind:value={query}
      type="text"
      placeholder={m('emoji.search_placeholder')}
      class="w-full rounded bg-surface px-3 py-2.5 text-base outline-none placeholder:text-muted compact-input:px-2.5 compact-input:py-1.5 compact-input:text-sm"
    />
  </div>

  <!-- Emoji grid section -->
  <div class="menu-section p-2 compact-input:p-1">
    <!-- Emoji grid -->
    <div class="max-h-[min(50vh,18rem)] overflow-y-auto">
      {#if isSearching}
        {#if searchResults.length === 0}
          <div class="py-6 text-center text-sm text-muted">{m('emoji.no_results')}</div>
        {:else}
          <div class="grid grid-cols-[repeat(auto-fit,minmax(44px,1fr))] compact-input:grid-cols-8">
            {#each searchResults as result (result.name)}
              <button
                class="flex min-h-11 cursor-pointer items-center justify-center rounded text-xl hover:bg-surface active:bg-surface compact-input:min-h-8 compact-input:text-base"
                onclick={() => selectEmoji(result.emoji)}
                title={result.name}
              >
                {result.emoji}
              </button>
            {/each}
          </div>
        {/if}
      {:else}
        {#if recent.length > 0}
          <div
            class="mt-1 mb-1 px-1 text-sm font-medium text-muted compact-input:mt-0 compact-input:mb-0.5 compact-input:px-0 compact-input:text-xs"
          >
            Recently Used
          </div>
          <div class="grid grid-cols-[repeat(auto-fit,minmax(44px,1fr))] compact-input:grid-cols-8">
            {#each recent as emoji (emoji)}
              <button
                class="flex min-h-11 cursor-pointer items-center justify-center rounded text-xl hover:bg-surface active:bg-surface compact-input:min-h-8 compact-input:text-base"
                onclick={() => selectEmoji(emoji)}
              >
                {emoji}
              </button>
            {/each}
          </div>
        {/if}
        {#each EMOJI_BY_CATEGORY as cat (cat.name)}
          <div
            class="mt-3 mb-1 px-1 text-sm font-medium text-muted compact-input:mt-1 compact-input:mb-0.5 compact-input:px-0 compact-input:text-xs"
          >
            {cat.name}
          </div>
          <div class="grid grid-cols-[repeat(auto-fit,minmax(44px,1fr))] compact-input:grid-cols-8">
            {#each cat.emojis as entry (entry.name)}
              <button
                class="flex min-h-11 cursor-pointer items-center justify-center rounded text-xl hover:bg-surface active:bg-surface compact-input:min-h-8 compact-input:text-base"
                onclick={() => selectEmoji(entry.emoji)}
                title={entry.name}
              >
                {entry.emoji}
              </button>
            {/each}
          </div>
        {/each}
      {/if}
    </div>
  </div>
</div>
