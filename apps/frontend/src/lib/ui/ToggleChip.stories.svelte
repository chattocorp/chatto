<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ToggleChip from './ToggleChip.svelte';

  const componentDescription = `
    Use ToggleChip for compact binary or independently selectable toggles inside dense editors. It
    is interactive; use SegmentedControl for one-of-many modes and Pill for passive labels.
    Compact admin actions use quiet shell lighting without a drop shadow, with the standard button focus ring and quick feedback.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/ToggleChip',
    component: ToggleChip,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  const tones = ['success', 'danger', 'warning', 'action', 'neutral'] as const;

  let pressedAllow = $state(false);
  let pressedDeny = $state(false);
</script>

<Story name="Tones (released vs pressed)" asChild>
  <div class="flex flex-col gap-3">
    {#each tones as tone (tone)}
      <div class="flex items-center gap-3">
        <span class="w-20 text-sm text-muted">{tone}</span>
        <ToggleChip {tone}>{tone}</ToggleChip>
        <ToggleChip {tone} pressed>{tone}</ToggleChip>
      </div>
    {/each}
  </div>
</Story>

<Story name="Permission editor pattern" asChild>
  <div class="flex items-center gap-2">
    <span class="text-sm">message.post</span>
    <ToggleChip
      tone="success"
      pressed={pressedAllow}
      onclick={() => {
        pressedAllow = !pressedAllow;
        if (pressedAllow) pressedDeny = false;
      }}
    >
      Allow
    </ToggleChip>
    <ToggleChip
      tone="danger"
      pressed={pressedDeny}
      onclick={() => {
        pressedDeny = !pressedDeny;
        if (pressedDeny) pressedAllow = false;
      }}
    >
      Deny
    </ToggleChip>
  </div>
</Story>

<Story name="Admin actions" asChild>
  <div class="flex flex-col gap-4 rounded-lg bg-background p-4">
    <div class="flex gap-2 rounded-lg bg-surface p-3">
      <ToggleChip tone="neutral" square title="Edit room">
        <span class="iconify icon-[uil--pen] text-base" aria-label="Edit room"></span>
      </ToggleChip>
      <ToggleChip tone="neutral" square title="Room permissions">
        <span class="iconify icon-[uil--shield] text-base" aria-label="Room permissions"></span>
      </ToggleChip>
      <ToggleChip tone="warning" square title="Archive room">
        <span class="iconify icon-[uil--archive] text-base" aria-label="Archive room"></span>
      </ToggleChip>
      <ToggleChip tone="danger" square title="Delete group" disabled>
        <span class="iconify icon-[uil--trash-alt] text-base" aria-label="Delete group"></span>
      </ToggleChip>
    </div>
    <div class="flex gap-2">
      <ToggleChip tone="neutral">All rooms</ToggleChip>
      <ToggleChip tone="action" pressed>Archived</ToggleChip>
    </div>
  </div>
</Story>
