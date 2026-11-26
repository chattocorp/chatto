<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PulseIcon from './PulseIcon.svelte';

  const { Story } = defineMeta({
    title: 'UI/Pulse Icon',
    component: PulseIcon,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'An icon with a ping pulse for live state, such as an active call in the sidebar, the room header, and the server Overview. Set pulse to false to keep the glyph without motion.'
        }
      }
    }
  });
</script>

<Story name="Active call" asChild>
  <span class="sidebar-icon text-action"><PulseIcon icon="icon-[uil--phone]" /></span>
</Story>

<Story name="Without pulse" asChild>
  <span class="sidebar-icon text-action"><PulseIcon icon="icon-[uil--phone]" pulse={false} /></span>
</Story>
