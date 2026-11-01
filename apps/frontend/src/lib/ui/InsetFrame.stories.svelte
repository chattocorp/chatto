<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent, waitFor } from 'storybook/test';
  import ContextMenu from './ContextMenu.svelte';
  import MenuSection from './MenuSection.svelte';
  import MenuItem from './MenuItem.svelte';
  import PaletteFrame from './PaletteFrame.svelte';
  import PaletteSearch from './PaletteSearch.svelte';
  import ComposerLinkPreviewCard from '$lib/components/composer/ComposerLinkPreviewCard.svelte';

  const { Story } = defineMeta({
    title: 'UI/Inset Frame',
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'One Inset Frame, used by real context menus, palettes, and previews. Change the theme to compare light and dark; each story checks the shared geometry and colors after opening the menu.'
        }
      }
    }
  });

  async function compareFrames({ canvasElement }: { canvasElement: HTMLElement }) {
    await userEvent.click(canvasElement.querySelector<HTMLButtonElement>('[data-open-frame]')!);
    await waitFor(() => expect(canvasElement.querySelector('[role="menu"]')).not.toBeNull());
    const menu = canvasElement.querySelector<HTMLElement>('[role="menu"]')!;
    const preview = canvasElement.querySelector<HTMLElement>(
      '[data-testid="composer-link-preview"]'
    )!;
    const frames = [preview];
    if (window.innerWidth >= 768)
      frames.push(canvasElement.querySelector<HTMLElement>('.command-palette')!);
    for (const frame of frames) {
      for (const property of [
        'borderTopWidth',
        'borderTopColor',
        'borderTopLeftRadius',
        'backgroundColor',
        'paddingTop'
      ] as const) {
        // Flat previews hide their decorative rim; menus keep their boundary.
        if (
          frame === preview &&
          Number(getComputedStyle(frame).getPropertyValue('--depth-strength')) === 0 &&
          (property === 'borderTopColor' || property === 'backgroundColor')
        )
          continue;
        await expect(getComputedStyle(frame)[property]).toBe(getComputedStyle(menu)[property]);
      }
      const content = frame.querySelector<HTMLElement>('.preview-content, .menu-section')!;
      const menuContent = menu.querySelector<HTMLElement>('.menu-section')!;
      for (const property of ['backgroundColor', 'borderTopLeftRadius', 'boxShadow'] as const) {
        await expect(getComputedStyle(content)[property]).toBe(
          getComputedStyle(menuContent)[property]
        );
      }
    }
  }
</script>

<script lang="ts">
  let anchor = $state<DOMRect | null>(null);
</script>

{#snippet comparison(strength: number, width: number)}
  <div
    class="flex flex-wrap items-start gap-6 p-6"
    style:--depth-strength={strength}
    style:--depth-width={width}
  >
    <section class="min-h-60 w-64">
      <p class="mb-3 text-sm font-medium">Context menu</p>
      <button
        data-open-frame
        type="button"
        class="btn-secondary"
        onclick={(event) => (anchor = event.currentTarget.getBoundingClientRect())}
        >Open menu</button
      >
      {#if anchor}
        <ContextMenu
          {anchor}
          presentation="floating"
          ariaLabel="Frame example"
          onclose={() => (anchor = null)}
        >
          <MenuSection
            ><MenuItem icon="icon-[uil--comment-alt]" onclick={() => {}}>Reply</MenuItem><MenuItem
              icon="icon-[uil--copy]"
              onclick={() => {}}>Copy message</MenuItem
            ></MenuSection
          >
          <MenuSection
            ><MenuItem icon="icon-[uil--trash-alt]" tone="danger" onclick={() => {}}
              >Delete</MenuItem
            ></MenuSection
          >
        </ContextMenu>
      {/if}
    </section>
    <section class="w-72 max-w-full">
      <p class="mb-3 text-sm font-medium">Preview</p>
      <ComposerLinkPreviewCard
        preview={{
          url: 'https://example.test/guide',
          title: 'A guide to building together',
          siteName: 'Chatto community'
        }}
        onDismiss={() => {}}
      />
    </section>
    <section class="max-w-full">
      <p class="mb-3 text-sm font-medium">Palette</p>
      <PaletteFrame title="Quick switcher" onBack={() => {}}>
        <PaletteSearch label="Search conversations" />
        <MenuSection
          ><button type="button" class="command-palette-result"># development</button><button
            type="button"
            class="command-palette-result"># general</button
          ></MenuSection
        >
      </PaletteFrame>
    </section>
  </div>
{/snippet}

<Story name="Shared treatment" asChild play={compareFrames}>{@render comparison(0.75, 1)}</Story>
<Story name="Flat" asChild play={compareFrames}>{@render comparison(0, 1)}</Story>
<Story name="Strong depth" asChild play={compareFrames}>{@render comparison(1.75, 1.5)}</Story>
