<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PaneContent from './PaneContent.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaneContent',
    component: PaneContent,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared body frame for pages beneath a PaneHeader. It supplies the standard readable width, page padding, and scrolling; pages supply their own content stack.'
        }
      }
    }
  });
</script>

<Story name="Panel stack" asChild>
  <div class="h-80 w-full bg-background">
    <PaneContent fillHeight>
      <div class="flex min-h-0 flex-1 flex-col">
        <div class="flex min-h-0 flex-1 flex-col panel-shell p-6">Primary panel</div>
      </div>
    </PaneContent>
  </div>
</Story>
