<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FadeScale from './FadeScale.svelte';
  import PillButtonGroup from './PillButtonGroup.svelte';

  const { Story } = defineMeta({ title: 'UI/FadeScale', component: FadeScale });
</script>

<script lang="ts">
  let visible = $state(false);
</script>

<Story name="Toolbar reveal" asChild>
  <div class="flex w-64 flex-col gap-1">
    {#if visible}
      <FadeScale>
        <PillButtonGroup compact label="Call controls">
          <button class="pill-button">Mute</button>
          <button class="pill-button">Camera</button>
          <button class="pill-button-danger">Leave</button>
        </PillButtonGroup>
      </FadeScale>
    {/if}
    <button class="btn" onclick={() => (visible = !visible)} aria-expanded={visible}>
      Toggle toolbar
    </button>
  </div>
</Story>
