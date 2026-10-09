<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PaneContent from './PaneContent.svelte';
  import Panel from './Panel.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaneContent',
    component: PaneContent,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared body frame for pages beneath a PaneHeader. Below 768px, panels become flat sections and ordinary content keeps a 16px gutter. Wider pages use framed panels and 24px page padding.'
        }
      }
    }
  });
</script>

<Story name="Panel stack" asChild>
  <div class="h-80 w-full bg-background">
    <PaneContent fillHeight>
      <div class="flex min-h-0 flex-1 flex-col">
        <Panel title="Primary panel" fillHeight>Panel content</Panel>
      </div>
    </PaneContent>
  </div>
</Story>

<Story name="Edge to edge on narrow screens" asChild>
  <div class="h-80 w-full bg-background">
    <PaneContent fillHeight>
      <Panel title="Recent activity" noPadding fillHeight>
        <div class="px-4 py-3">Alex replied in a thread you follow.</div>
        <div class="px-4 py-3">Sam reacted to your message.</div>
      </Panel>
    </PaneContent>
  </div>
</Story>

<Story name="Wide browsing grid" asChild>
  <div class="h-80 w-full bg-background">
    <PaneContent wide>
      <div class="grid grid-cols-3 gap-4">
        <div class="panel-shell p-6">Group</div>
        <div class="panel-shell p-6">Group</div>
        <div class="panel-shell p-6">Group</div>
      </div>
    </PaneContent>
  </div>
</Story>
