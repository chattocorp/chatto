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
      title="Answer mentions"
      description="Replies when someone @mentions it, right there in the thread."
    />
    <SelectableTile
      bind:checked={directMessages}
      icon="icon-[uil--envelope]"
      title="Chat in DMs"
      description="Talks one-on-one when someone sends it a direct message."
    />
    <SelectableTile
      disabled
      icon="icon-[uil--eye]"
      title="Read everything"
      description="Sees every message in its rooms, not just the ones meant for it."
      disabledReason="You don't have this permission yourself, so you can't give it to your bot."
    />
  </fieldset>
</Story>
