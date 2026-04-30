<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Hint from './Hint.svelte';

  const { Story } = defineMeta({
    title: 'UI/Hint',
    component: Hint,
    tags: ['autodocs']
  });
</script>

<Story name="Variants" asChild>
  <div class="flex max-w-prose flex-col gap-3">
    <Hint variant="info">Heads up: this is informational context.</Hint>
    <Hint variant="success" icon="uil--check-circle">
      Settings saved. New members will pick this up on next login.
    </Hint>
    <Hint variant="warning" icon="uil--exclamation-triangle">
      Changes here apply immediately to all members.
    </Hint>
    <Hint variant="danger" icon="uil--exclamation-octagon">
      Deleting this space cannot be undone.
    </Hint>
  </div>
</Story>
