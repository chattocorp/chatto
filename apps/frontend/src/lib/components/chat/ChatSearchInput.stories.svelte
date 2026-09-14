<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ChatSearchInput from './ChatSearchInput.svelte';

  const { Story } = defineMeta({
    title: 'Chat/Search input',
    component: ChatSearchInput,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Search input for fixed chat action rails. It shares the message composer surface and height.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let emptyValue = $state('');
  let populatedValue = $state('roadmap');
  let narrowValue = $state('');
  let disabledValue = $state('Search is unavailable');
</script>

<Story name="Empty" asChild>
  <div class="w-96 max-w-full bg-background p-2">
    <ChatSearchInput
      label="Search messages"
      placeholder="Words, phrases, or filters..."
      bind:value={emptyValue}
    />
  </div>
</Story>

<Story name="Populated and clearable" asChild>
  <div class="w-96 max-w-full bg-background p-2">
    <ChatSearchInput
      label="Search members"
      placeholder="Search members"
      clearLabel="Clear member search"
      bind:value={populatedValue}
      onclear={() => (populatedValue = '')}
    />
  </div>
</Story>

<Story name="Narrow" asChild>
  <div class="w-52 bg-background p-2">
    <ChatSearchInput
      label="Search messages"
      placeholder="Words, phrases, or filters..."
      bind:value={narrowValue}
    />
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="w-96 max-w-full bg-background p-2">
    <ChatSearchInput label="Search messages" bind:value={disabledValue} disabled />
  </div>
</Story>
