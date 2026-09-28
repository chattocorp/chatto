<!-- @component Test harness: a positioned composer surface whose input forwards keys to the popup, like the editor does. -->
<script lang="ts">
  import AutocompletePopup from './AutocompletePopup.svelte';

  let { labels, onSelect }: { labels: string[]; onSelect: (label: string) => void } = $props();

  let popup = $state<{ handleKeyDown: (event: KeyboardEvent) => boolean }>();
</script>

<div style="position: fixed; left: 40px; bottom: 40px; width: 300px">
  <div class="relative h-12" data-testid="surface">
    <input
      data-testid="editor"
      aria-label="Editor"
      onkeydown={(event) => popup?.handleKeyDown(event)}
    />
    <AutocompletePopup
      bind:this={popup}
      items={labels}
      getKey={(label) => label}
      onSelect={(label) => onSelect(label)}
      onClose={() => {}}
      testid="popup"
    >
      {#snippet item({ item })}
        <span class="entry">{item}</span>
      {/snippet}
    </AutocompletePopup>
  </div>
</div>
