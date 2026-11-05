<script lang="ts">
  import type { NativeSession } from './session.svelte.ts';
  import { appearance } from './ui/scale.svelte.ts';
  import { inputTheme } from './ui/theme.ts';
  import ActionButton from './ActionButton.svelte';
  let { session }: { session: NativeSession } = $props();
  let draft = $state('');
  const percentage = $derived(Math.round(appearance.scale * 100));
  function apply() {
    const value = Number(draft.trim().replace(/%$/, ''));
    if (draft.trim() && Number.isFinite(value)) session.setUiScale(value / 100);
    draft = '';
  }
</script>

<div class="scale-controls">
  <text class="heading">UI scale</text>
  <text class="hint"
    >Scale the whole interface from 50% to 200%. Enter a percentage and press Enter.</text
  >
  <div class="controls">
    <ActionButton
      label="Smaller"
      onclick={() => session.setUiScale(appearance.scale - 0.1)}
      disabled={appearance.scale <= 0.5}
      testid="scale-smaller"
    />
    <input
      class="percentage"
      theme={inputTheme}
      aria-label="UI scale percentage"
      value={draft || String(percentage)}
      onchange={(event) => (draft = event.value)}
      onsubmit={apply}
      testid="scale-percentage"
    />
    <text>%</text>
    <ActionButton label="Apply" onclick={apply} testid="scale-apply" />
    <ActionButton
      label="Larger"
      onclick={() => session.setUiScale(appearance.scale + 0.1)}
      disabled={appearance.scale >= 2}
      testid="scale-larger"
    />
    <ActionButton
      label="Reset"
      onclick={() => {
        draft = '';
        session.setUiScale(1);
      }}
      testid="scale-reset"
    />
  </div>
  <text class="hint"
    >Shortcuts: ⌘+ / ⌘− / ⌘0 (Ctrl on other platforms). Changes are saved automatically.</text
  >
</div>

<style>
  .scale-controls {
    display: flex;
    flex-direction: column;
    gap: var(--size-8);
    padding: var(--size-12);
    background-color: var(--color-surface);
    border-radius: var(--radius-panel);
  }
  .heading {
    font-size: var(--text-heading);
    font-weight: 600;
  }
  .hint {
    font-size: var(--text-small);
    color: var(--color-muted);
    white-space: normal;
  }
  .controls {
    display: flex;
    flex-direction: row;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--size-8);
  }
  .percentage {
    width: var(--size-60);
    height: var(--size-32);
    font-family: var(--font-sans);
    font-size: var(--text-body);
  }
</style>
