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
  import { tick } from 'svelte';
  import { emojiToneBase } from '$lib/emojiTones';
  import { m } from '$lib/i18n/messages';
  import { searchEmojis, EMOJI_BY_CATEGORY, emojiWithTone } from '$lib/emoji';
  import { emojiPreferences } from '$lib/state/emojiPreferences.svelte';
  import EmojiToneSelector from './EmojiToneSelector.svelte';
  import { shouldAutoFocus } from '$lib/utils/shouldAutoFocus';
  import { getRecentEmojis } from '$lib/state/recentEmojis.svelte';

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
  let qualifying = $state<string | null>(null);
  // Keep the source button and grid mounted so Escape restores focus and scroll.
  let returnFocus: HTMLButtonElement | null = null;

  const recentStore = $derived(getRecentEmojis(serverId));
  const recent = $derived(recentStore.recent);

  const searchResults = $derived(
    query.trim()
      ? searchEmojis(query.trim(), 50).map((result) => ({
          ...result,
          ...emojiWithTone(result.name, emojiPreferences.tone)
        }))
      : []
  );
  const categories = $derived(
    EMOJI_BY_CATEGORY.map((category) => ({
      ...category,
      emojis: category.emojis.map((entry) => emojiWithTone(entry.name, emojiPreferences.tone))
    }))
  );
  const isSearching = $derived(query.trim().length > 0);

  function focusSearchInput(node: HTMLInputElement) {
    if (shouldAutoFocus()) queueMicrotask(() => node.focus());
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (qualifying) {
        e.preventDefault();
        e.stopPropagation();
        backToResults();
        return;
      }
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

  function backToResults() {
    qualifying = null;
    void tick().then(() => returnFocus?.focus());
  }

  function chooseEmoji(entry: { name: string; emoji: string }, button: HTMLButtonElement) {
    const baseName = emojiToneBase(entry.name);
    if (baseName) {
      returnFocus = button;
      qualifying = baseName;
    } else {
      selectEmoji(entry.emoji);
    }
  }
</script>

<!-- Escape returns to results before dismissing the picker. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="flex w-88 max-w-full flex-col gap-2 in-data-[menu-presentation=sheet]:w-full compact-density:w-72 compact-density:gap-1"
  onkeydown={handleKeydown}
>
  {#if qualifying}
    <div class="menu-section">
      <button type="button" class="menu-item" onclick={backToResults}>
        <span class="iconify icon-[uil--arrow-left]" aria-hidden="true"></span>
        {m('common.back')}
      </button>
    </div>
    <EmojiToneSelector name={qualifying} onSelect={(choice) => selectEmoji(choice.emoji)} />
  {/if}
  <div class={qualifying ? 'hidden' : 'contents'}>
    <!-- Search section -->
    <div class="menu-section p-2 compact-density:p-1">
      <input
        {@attach focusSearchInput}
        bind:value={query}
        type="text"
        placeholder={m('emoji.search_placeholder')}
        aria-label={m('emoji.search_placeholder')}
        class="w-full rounded bg-surface px-3 py-2.5 text-base outline-none placeholder:text-muted compact-density:px-2.5 compact-density:py-1.5 compact-density:text-sm"
      />
    </div>

    <!-- Emoji grid section -->
    <div class="menu-section p-2 compact-density:p-1">
      <!-- Emoji grid -->
      <div class="max-h-[min(50vh,18rem)] overflow-y-auto">
        {#if isSearching}
          {#if searchResults.length === 0}
            <div class="py-6 text-center text-sm text-muted">{m('emoji.no_results')}</div>
          {:else}
            <div
              class="grid grid-cols-[repeat(auto-fill,minmax(44px,1fr))] compact-density:grid-cols-8"
            >
              {#each searchResults as result (result.name)}
                <button
                  type="button"
                  class="flex min-h-11 cursor-pointer items-center justify-center rounded text-xl hover:bg-surface active:bg-surface compact-density:min-h-8 compact-density:text-base"
                  onclick={(event) => chooseEmoji(result, event.currentTarget)}
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
              class="mt-1 mb-1 px-1 text-sm font-medium text-muted compact-density:mt-0 compact-density:mb-0.5 compact-density:px-0 compact-density:text-xs"
            >
              {m('emoji.recently_used')}
            </div>
            <div
              class="grid grid-cols-[repeat(auto-fill,minmax(44px,1fr))] compact-density:grid-cols-8"
            >
              {#each recent as emoji (emoji)}
                <button
                  type="button"
                  class="flex min-h-11 cursor-pointer items-center justify-center rounded text-xl hover:bg-surface active:bg-surface compact-density:min-h-8 compact-density:text-base"
                  onclick={() => selectEmoji(emoji)}
                >
                  {emoji}
                </button>
              {/each}
            </div>
          {/if}
          {#each categories as cat (cat.name)}
            <div
              class="mt-3 mb-1 px-1 text-sm font-medium text-muted compact-density:mt-1 compact-density:mb-0.5 compact-density:px-0 compact-density:text-xs"
            >
              {cat.name}
            </div>
            <div
              class="grid grid-cols-[repeat(auto-fill,minmax(44px,1fr))] compact-density:grid-cols-8"
            >
              {#each cat.emojis as entry (entry.name)}
                <button
                  type="button"
                  class="flex min-h-11 cursor-pointer items-center justify-center rounded text-xl hover:bg-surface active:bg-surface compact-density:min-h-8 compact-density:text-base"
                  onclick={(event) => chooseEmoji(entry, event.currentTarget)}
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
</div>
