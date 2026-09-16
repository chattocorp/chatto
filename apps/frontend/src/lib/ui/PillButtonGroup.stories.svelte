<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PillButtonGroup from './PillButtonGroup.svelte';

  const { Story } = defineMeta({
    title: 'UI/PillButtonGroup',
    component: PillButtonGroup,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let muted = $state(false);
</script>

{#snippet controls()}
  <button class="pill-button" aria-label="Devices">
    <span class="iconify icon-[uil--setting]" aria-hidden="true"></span>
  </button>
  <button
    class={muted ? 'pill-button' : 'pill-button-success'}
    aria-label="Microphone"
    aria-pressed={!muted}
    onclick={() => (muted = !muted)}
  >
    <span
      class={['iconify', muted ? 'icon-[uil--microphone-slash]' : 'icon-[uil--microphone]']}
      aria-hidden="true"
    ></span>
  </button>
  <button class="pill-button" aria-label="Camera" disabled>
    <span class="iconify icon-[uil--video-slash]" aria-hidden="true"></span>
  </button>
  <button class="pill-button" aria-label="Share screen">
    <span class="iconify icon-[uil--desktop]" aria-hidden="true"></span>
  </button>
  <button class="pill-button-danger" aria-label="Leave call">
    <span class="iconify icon-[uil--phone-slash]" aria-hidden="true"></span>
  </button>
{/snippet}

<Story name="Shell row" asChild>
  <div class="w-64">
    <PillButtonGroup label="Call controls">{@render controls()}</PillButtonGroup>
  </div>
</Story>

<Story name="Compact user card row" asChild>
  <div class="flex w-64 flex-col gap-1">
    <PillButtonGroup label="Call controls" compact>{@render controls()}</PillButtonGroup>
    <div class="flex h-12 items-center gap-2 shell-surface px-2">
      <span class="flex size-8 items-center justify-center rounded-full bg-surface-selected">A</span
      >
      Alice
    </div>
  </div>
</Story>

<Story name="Continuous compact row" asChild>
  <div class="w-64">
    <PillButtonGroup label="Call controls" compact gaps={false}
      >{@render controls()}</PillButtonGroup
    >
  </div>
</Story>

<Story name="Bevel on hover" asChild>
  <div class="w-64">
    <PillButtonGroup label="Call controls" compact bevelOnHover>{@render controls()}</PillButtonGroup>
  </div>
</Story>
