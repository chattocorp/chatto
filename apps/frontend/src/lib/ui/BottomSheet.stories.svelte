<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import BottomSheet from './BottomSheet.svelte';

  const { Story } = defineMeta({
    title: 'UI/BottomSheet',
    component: BottomSheet,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Touch-oriented bottom sheet with backdrop dismissal, Escape handling, and drag-to-close behavior.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let visible = $state(false);
</script>

<Story name="Interactive" asChild>
  <Button onclick={() => (visible = true)}>Open bottom sheet</Button>

  <BottomSheet bind:visible>
    <div class="flex flex-col gap-4">
      <div>
        <h2 class="text-lg font-semibold">Message actions</h2>
        <p class="text-sm text-muted">Tap the backdrop, press Escape, or drag down to dismiss.</p>
      </div>
      <div class="menu-section">
        <button type="button" class="menu-item">Reply</button>
        <button type="button" class="menu-item">Copy link</button>
        <button type="button" class="menu-item text-danger">Delete message</button>
      </div>
    </div>
  </BottomSheet>
</Story>
