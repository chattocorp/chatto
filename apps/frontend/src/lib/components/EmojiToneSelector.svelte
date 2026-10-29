<!-- @component Headerless tone choices for one emoji, with focus and keyboard navigation. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { emojiPreferences } from '$lib/state/emojiPreferences.svelte';
  import { emojiWithTone } from '$lib/emoji';
  import { emojiTones } from '$lib/emojiTones';

  let {
    name,
    onSelect
  }: {
    name: string;
    onSelect: (choice: { name: string; emoji: string }) => void;
  } = $props();

  function handleKeydown(
    event: KeyboardEvent & { currentTarget: HTMLButtonElement },
    index: number
  ) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      event.stopPropagation();
      const next =
        (index + (event.key === 'ArrowDown' ? 1 : -1) + emojiTones.length) % emojiTones.length;
      event.currentTarget.parentElement?.querySelectorAll('button')[next]?.focus();
    } else if (event.key === 'Tab') {
      event.preventDefault();
      event.stopPropagation();
      event.currentTarget.click();
    }
  }
</script>

<div class="max-h-[min(50vh,18rem)] overflow-y-auto menu-section">
  {#each emojiTones as option, index (option.tone)}
    <button
      type="button"
      class="menu-item focus:bg-surface focus:outline-none"
      data-emoji-tone={option.tone}
      {@attach (node) => {
        if (option.tone === emojiPreferences.tone) node.focus();
      }}
      onkeydown={(event) => handleKeydown(event, index)}
      onclick={() => {
        emojiPreferences.tone = option.tone;
        onSelect(emojiWithTone(name, option.tone));
      }}
    >
      <span class="text-xl">{emojiWithTone(name, option.tone).emoji}</span>
      <span class="text-sm text-text">{m(`emoji.${option.label}`)}</span>
    </button>
  {/each}
</div>
