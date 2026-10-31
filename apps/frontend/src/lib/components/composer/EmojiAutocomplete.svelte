<!--
@component

Discord-style emoji autocomplete popup.
Shows matching emojis when typing :shortcode in chat input.

**Props:**
- `query` - Current search query (without the leading colon)
- `results` - Ranked matches from the autocomplete state
- `onSelect` - Callback with the emoji, name, and final selection key
- `onClose` - Callback to close the popup
-->
<script lang="ts">
  import { emojiWithTone, emojiToName, type EmojiResult } from '$lib/emoji';
  import { emojiPreferences } from '$lib/state/emojiPreferences.svelte';
  import AutocompletePopup from './AutocompletePopup.svelte';
  import { emojiTones, emojiToneBase } from '$lib/emojiTones';
  import { m } from '$lib/i18n/messages';

  type Props = {
    query: string;
    results: EmojiResult[];
    onSelect: (emoji: string, name: string, key: string) => void;
    onClose: () => void;
  };

  let { query, results, onSelect, onClose }: Props = $props();

  // Cursor offsets can change the owner's snapshot without changing the list.
  const items = $derived(results);
  // Editing the query cancels the second step without changing the draft.
  let qualifying = $derived.by((): EmojiResult | null => {
    void query;
    return null;
  });
  let previousKey = $derived.by((): string | undefined => {
    void query;
    return undefined;
  });

  let searchPopup = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);
  let tonePopup = $state<{ handleKeyDown: (e: KeyboardEvent) => boolean } | null>(null);

  export function handleKeyDown(event: KeyboardEvent): boolean {
    return (qualifying ? tonePopup : searchPopup)?.handleKeyDown(event) ?? false;
  }

  function handleSelect(result: EmojiResult, key: string) {
    if (emojiToneBase(result.name)) {
      previousKey = result.name;
      qualifying = result;
      return;
    }
    const choice = emojiWithTone(result.name, emojiPreferences.tone);
    onSelect(choice.emoji, choice.name, key);
  }
</script>

{#if qualifying}
  {@const chosen = qualifying}
  <AutocompletePopup
    bind:this={tonePopup}
    items={[...emojiTones]}
    getKey={(option) => String(option.tone)}
    initialSelectedKey={String(emojiPreferences.tone)}
    onSelect={(option, key) => {
      const choice = emojiWithTone(chosen.name, option.tone);
      emojiPreferences.tone = option.tone;
      onSelect(choice.emoji, choice.name, key);
    }}
    onClose={() => (qualifying = null)}
  >
    {#snippet item({ item: option })}
      <span class="flex h-6 w-6 shrink-0 items-center justify-center text-xl"
        >{emojiWithTone(chosen.name, option.tone).emoji}</span
      >
      <span class="text-sm text-text">{m(`emoji.${option.label}`)}</span>
    {/snippet}
  </AutocompletePopup>
{:else}
  <AutocompletePopup
    bind:this={searchPopup}
    {items}
    getKey={(r) => r.name}
    initialSelectedKey={previousKey}
    onSelect={handleSelect}
    {onClose}
  >
    {#snippet item({ item: result })}
      {@const choice = emojiWithTone(result.name, emojiPreferences.tone)}
      <span class="flex h-6 w-6 shrink-0 items-center justify-center text-xl">{choice.emoji}</span>
      <span class="min-w-0 truncate text-sm text-text"
        >:{emojiToName(result.emoji) ?? result.name}:</span
      >
    {/snippet}
  </AutocompletePopup>
{/if}
