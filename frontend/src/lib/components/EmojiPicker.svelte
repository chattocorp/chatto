<!--
@component

Full emoji picker with search and categories.
Pure content component — rendered inside a ContextMenu by the parent.
Uses the same section styling as MessageContextMenu (rounded-md bg-background sections).

**Props:**
- `serverId` - The active server. Used to scope the per-server "Recently Used" list.
- `onSelect` - Callback when an emoji is selected
- `onClose` - Callback to dismiss the picker (Escape key)
-->
<script lang="ts">
  import { tick } from 'svelte';
  import { searchEmojis, EMOJI_BY_CATEGORY } from '$lib/emoji';
  import { isTouchDevice } from '$lib/utils/isTouchDevice';
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
  let searchInput: HTMLInputElement | undefined = $state();
  const isTouch = isTouchDevice();

  const recentStore = $derived(getRecentEmojis(serverId));
  const recent = $derived(recentStore.recent.slice(0, MAX_RECENT_EMOJIS));

  const searchResults = $derived(query.trim() ? searchEmojis(query.trim(), 50) : []);
  const isSearching = $derived(query.trim().length > 0);

  $effect(() => {
    if (!isTouch && searchInput) {
      tick().then(() => searchInput?.focus());
    }
  });

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
<div class="flex w-full flex-col gap-2 md:w-72 md:gap-1" onkeydown={handleKeydown}>
  <!-- Search section -->
  <div class="menu-section p-2 md:p-1">
    <input
      bind:this={searchInput}
      bind:value={query}
      type="text"
      placeholder="Search emojis..."
      class="w-full rounded bg-surface-100 px-3 py-2.5 text-base outline-none placeholder:text-muted md:px-2.5 md:py-1.5 md:text-sm"
    />
  </div>

  <!-- Emoji grid section -->
  <div class="menu-section p-2 md:p-1">
    <!-- Emoji grid -->
    <div class="max-h-[50vh] overflow-y-auto md:max-h-72">
      {#if isSearching}
        {#if searchResults.length === 0}
          <div class="py-6 text-center text-sm text-muted">No emojis found</div>
        {:else}
          <div class="grid grid-cols-7 md:grid-cols-8">
            {#each searchResults as result (result.name)}
              <button
                class="flex aspect-square cursor-pointer items-center justify-center rounded text-3xl hover:bg-surface-100 active:bg-surface-100 md:h-8 md:w-8 md:text-base"
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
            class="mt-1 mb-1 px-1 text-sm font-medium text-muted md:mt-0 md:mb-0.5 md:px-0 md:text-xs"
          >
            Recently Used
          </div>
          <div class="grid grid-cols-7 md:grid-cols-8">
            {#each recent as emoji (emoji)}
              <button
                class="flex aspect-square cursor-pointer items-center justify-center rounded text-3xl hover:bg-surface-100 active:bg-surface-100 md:h-8 md:w-8 md:text-base"
                onclick={() => selectEmoji(emoji)}
              >
                {emoji}
              </button>
            {/each}
          </div>
        {/if}
        {#each EMOJI_BY_CATEGORY as cat (cat.name)}
          <div
            class="mt-3 mb-1 px-1 text-sm font-medium text-muted md:mt-1 md:mb-0.5 md:px-0 md:text-xs"
          >
            {cat.name}
          </div>
          <div class="grid grid-cols-7 md:grid-cols-8">
            {#each cat.emojis as entry (entry.name)}
              <button
                class="flex aspect-square cursor-pointer items-center justify-center rounded text-3xl hover:bg-surface-100 active:bg-surface-100 md:h-8 md:w-8 md:text-base"
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
