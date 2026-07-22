<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ToggleChip from './ToggleChip.svelte';

  const componentDescription = `
    Use ToggleChip for compact binary or independently selectable toggles inside dense editors. It
    is interactive; use SegmentedControl for one-of-many modes and Pill for passive labels.
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
