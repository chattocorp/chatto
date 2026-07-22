<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ShortcutTextInput from './ShortcutTextInput.svelte';

  const { Story } = defineMeta({
    title: 'Form/ShortcutTextInput',
    component: ShortcutTextInput,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'A text field that can be focused by Command/Control plus a supplied key. Its placeholder uses the current platform convention and the shortcut selects existing text for immediate replacement.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let permissions = $state('message.post');
  let rooms = $state('');
</script>

<Story name="Filter shortcut" asChild>
  <div class="max-w-md">
    <ShortcutTextInput
      id="permission-filter"
      label="Filter permissions"
      labelHidden
      shortcutKey="/"
      bind:value={permissions}
      placeholder="Filter permissions…"
      leadingIcon="uil--search"
      autocomplete="off"
    />
  </div>
</Story>

<Story name="Visible label" asChild>
  <div class="max-w-md">
    <ShortcutTextInput
      id="room-filter"
      label="Find a room"
      shortcutKey="/"
      bind:value={rooms}
      placeholder="Search rooms…"
      leadingIcon="uil--search"
    />
  </div>
</Story>
