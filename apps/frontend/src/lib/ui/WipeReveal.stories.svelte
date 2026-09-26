<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import WipeReveal from './WipeReveal.svelte';
  import PillButtonGroup from './PillButtonGroup.svelte';
  const { Story } = defineMeta({ title: 'UI/WipeReveal', component: WipeReveal });
</script>

<script lang="ts">
  let active = $state(false);
  let visible = $state(true);
</script>

<Story name="Call controls" asChild>
  <button class="mb-2 btn-secondary" onclick={() => (visible = !visible)}>Toggle sidebar</button>
  {#if visible}
    <WipeReveal {active} class="w-80">
      {#snippet children(joined)}
        {#if joined}
          <PillButtonGroup label="Call controls">
            <button class="pill-button">Camera</button>
            <button class="pill-button-success">Mute</button>
            <button class="pill-button-danger" onclick={() => (active = false)}>Leave</button>
          </PillButtonGroup>
        {:else}
          <button class="btn-action min-h-12 w-full" onclick={() => (active = true)}
            >Start call</button
          >
        {/if}
      {/snippet}
    </WipeReveal>
  {/if}
</Story>
