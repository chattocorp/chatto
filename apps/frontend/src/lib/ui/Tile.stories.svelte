<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';

  const { Story } = defineMeta({
    title: 'Foundations/Tiles',
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'A tile is a flat, borderless card for one item in a grid. It shares the panel frame surface and corners, without depth lighting. tile-interactive adds the selectable-row hover fill for tiles that open something. A section-label names each group of tiles.'
        }
      }
    }
  });

  const rooms = [
    { name: 'hiking', description: 'Trip reports, routes, and the Saturday hikes', joined: true },
    { name: 'trail-maps', description: 'GPX files, maps, and trail conditions', joined: true },
    { name: 'climbing', description: 'Bouldering, top rope, and gym meetups', joined: false }
  ];
</script>

<Story name="Group of tiles" asChild>
  <section class="flex max-w-3xl flex-col gap-2 bg-background p-4">
    <div class="flex min-h-6 items-center justify-between gap-3 px-1">
      <h3 class="section-label text-muted">Outdoors <span class="font-normal">· 3</span></h3>
      <Button variant="ghost" size="compact">
        <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
        Join all
      </Button>
    </div>
    <ul class="grid grid-cols-[repeat(auto-fill,minmax(min(100%,15rem),1fr))] gap-2">
      {#each rooms as room (room.name)}
        <li
          class={[
            'relative flex min-w-0 flex-col gap-1 tile px-3 py-2.5',
            room.joined && 'tile-interactive'
          ]}
        >
          <div class="flex min-h-6 items-center gap-2">
            <span aria-hidden="true" class="w-4 text-center text-muted/70">#</span>
            <span class="min-w-0 flex-1 truncate font-semibold text-text-top">{room.name}</span>
            {#if room.joined}
              <span class="text-xs text-muted">Joined</span>
            {:else}
              <Button variant="secondary" size="compact">Join</Button>
            {/if}
          </div>
          <p class="line-clamp-2 h-10 ps-6 text-sm text-muted">{room.description}</p>
        </li>
      {/each}
    </ul>
  </section>
</Story>
