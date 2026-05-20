<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ScrollFader from './ScrollFader.svelte';

  const { Story } = defineMeta({
    title: 'UI/ScrollFader',
    component: ScrollFader,
    tags: ['autodocs']
  });
</script>

<Story name="Top and bottom" asChild>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story name="Bottom only" asChild>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story name="Top only" asChild>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story name="Short content (no fades)" asChild>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(3) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story name="Sandwiched between header and footer" asChild>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <div class="border-b border-border bg-surface px-3 py-2 text-sm font-semibold">Header</div>
    <ScrollFader top bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
    <div class="border-t border-border bg-surface px-3 py-2 text-sm font-semibold">Footer</div>
  </div>
</Story>

<Story name="Taller fade overlay" asChild>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top bottom fadeHeight="h-16">
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>
