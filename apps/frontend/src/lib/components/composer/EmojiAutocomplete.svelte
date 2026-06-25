<!--
@component

Discord-style emoji autocomplete popup.
Shows matching emojis when typing :shortcode in chat input.

**Props:**
- `query` - Current search query (without the leading colon)
- `onSelect` - Callback when an emoji is selected
- `onClose` - Callback to close the popup
-->
<script lang="ts">
  import { searchEmojis, type EmojiResult } from '$lib/emoji';
  import AutocompletePopup from './AutocompletePopup.svelte';

  type Props = {
    query: string;
    onSelect: (emoji: string, name: string) => void;
    onClose: () => void;
  };

  let { query, onSelect, onClose }: Props = $props();

  let results = $derived(searchEmojis(query, 10));

  let popupRef = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);

  export function handleKeyDown(event: KeyboardEvent): boolean {
    return popupRef?.handleKeyDown(event) ?? false;
  }

  function handleSelect(result: EmojiResult, _key: string) {
    onSelect(result.emoji, result.name);
  }
</script>

<AutocompletePopup
  bind:this={popupRef}
  items={results}
  getKey={(r) => r.name}
  selectKeys={['Enter', 'Tab']}
  onSelect={handleSelect}
  {onClose}
  class="md:w-64"
>
  {#snippet item({ item: result })}
    <span class="text-xl">{result.emoji}</span>
    <span class="min-w-0 truncate text-sm text-text">:{result.name}:</span>
  {/snippet}
</AutocompletePopup>
