<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SelectableTile from './SelectableTile.svelte';

  const { Story } = defineMeta({
    title: 'UI/SelectableTile',
    component: SelectableTile,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Large multi-select checkbox tile with an icon, a title, and a description. Group tiles in a fieldset with a legend.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let mentions = $state(true);
  let directMessages = $state(false);
</script>

<Story name="Group" asChild>
  <fieldset class="grid max-w-2xl gap-3 sm:grid-cols-2">
    <legend class="mb-2 font-medium">Starting permissions</legend>
    <SelectableTile
      bind:checked={mentions}
      icon="icon-[uil--at]"
      title="Answer mentions and threads"
      description="Read and reply in threads where someone mentions the bot."
    />
    <SelectableTile
      bind:checked={directMessages}
      icon="icon-[uil--envelope]"
      title="Chat in direct messages"
      description="Read and post in DMs that people start with the bot."
    />
    <SelectableTile
      disabled
      icon="icon-[uil--eye]"
      title="Read all messages"
      description="Read every message in rooms the bot is in."
      disabledReason="You don't have this permission."
    />
  </fieldset>
</Story>
