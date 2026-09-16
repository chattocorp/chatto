<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import WipeReveal from './WipeReveal.svelte';
  import PillButtonGroup from './PillButtonGroup.svelte';
  const { Story } = defineMeta({ title: 'UI/WipeReveal', component: WipeReveal });
</script>
<script lang="ts">
  let active = $state(false);
</script>
<Story name="Call controls" asChild>
  <div class="grid w-80">
    {#if active}
      <WipeReveal class="col-start-1 row-start-1">
        <PillButtonGroup label="Call controls">
          <button class="pill-button">Camera</button>
          <button class="pill-button-success">Mute</button>
          <button class="pill-button-danger" onclick={() => (active = false)}>Leave</button>
        </PillButtonGroup>
      </WipeReveal>
    {:else}
      <WipeReveal class="col-start-1 row-start-1">
        <button class="btn-action min-h-12 w-full" onclick={() => (active = true)}>Start call</button>
      </WipeReveal>
    {/if}
  </div>
</Story>
