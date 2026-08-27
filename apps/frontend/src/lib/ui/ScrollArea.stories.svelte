<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ScrollArea from './ScrollArea.svelte';

  const { Story } = defineMeta({
    title: 'UI/ScrollArea',
    component: ScrollArea,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'A reusable native scroll viewport. It owns the positioning and overflow container; add ScrollFader when the viewport also needs edge fades.'
        }
      }
    }
  });
</script>

<Story name="Vertical content" asChild>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollArea>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollArea>
  </div>
</Story>

<Story name="Horizontal table" asChild>
  <div class="flex h-80 w-96 flex-col border border-border bg-background">
    <ScrollArea scrollX>
      <table class="w-[40rem] text-sm">
        <thead class="bg-surface">
          <tr><th class="px-3 py-2 text-left">Permission</th><th>Owner</th><th>Admin</th></tr>
        </thead>
        <tbody>
          {#each Array(24) as _, i (i)}
            <tr class="border-t border-border">
              <td class="px-3 py-2">message.permission-{i + 1}</td><td>✓</td><td>✓</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </ScrollArea>
  </div>
</Story>
