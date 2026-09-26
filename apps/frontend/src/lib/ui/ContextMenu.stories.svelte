<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent } from 'storybook/test';
  import ContextMenu from './ContextMenu.svelte';
  import MenuItem from './MenuItem.svelte';
  import MenuSection from './MenuSection.svelte';

  const { Story } = defineMeta({
    title: 'UI/ContextMenu',
    component: ContextMenu,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Responsive menu primitive: a top-layer floating menu on hover-capable devices and a bottom sheet on touch devices. Floating menus use a subtly lit outer frame around solid content sections with soft inset shadows.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let trigger: HTMLButtonElement;
  let open = $state(false);
  let sheetOpen = $state(false);
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  function openMenu() {
    const rect = trigger.getBoundingClientRect();
    anchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    open = true;
  }
</script>

<Story
  name="Floating menu"
  asChild
  play={async ({ canvas }) => {
    await userEvent.click(canvas.getByRole('button', { name: 'Open menu' }));
    const menu = canvas.getByRole('menu', { name: 'Example actions' });
    const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    await expect(getComputedStyle(menu).animationName).toBe(
      reducedMotion ? 'none' : 'overlay-enter'
    );
    if (!reducedMotion) await expect(getComputedStyle(menu).animationDuration).toBe('0.1s');
    await expect(
      getComputedStyle(canvas.getByRole('menuitem', { name: 'Edit' })).transitionDuration
    ).toBe('0s');
    await userEvent.click(canvas.getByRole('menuitem', { name: 'Copy' }));
    await expect(canvas.queryByRole('menu', { name: 'Example actions' })).not.toBeInTheDocument();
  }}
>
  <button bind:this={trigger} type="button" class="btn-action" onclick={openMenu}>Open menu</button>

  {#if open}
    <ContextMenu
      {anchor}
      presentation="floating"
      ariaLabel="Example actions"
      onclose={() => (open = false)}
    >
      <MenuSection>
        <MenuItem icon="icon-[uil--edit]" onclick={() => (open = false)}>Edit</MenuItem>
        <MenuItem icon="icon-[uil--copy]" onclick={() => (open = false)}>Copy</MenuItem>
        <MenuItem icon="icon-[uil--trash]" tone="danger" onclick={() => (open = false)}>
          Delete
        </MenuItem>
      </MenuSection>
    </ContextMenu>
  {/if}
</Story>

<Story name="Touch sheet" asChild>
  <button type="button" class="btn-action" onclick={() => (sheetOpen = true)}>Open sheet</button>

  {#if sheetOpen}
    <ContextMenu
      presentation="sheet"
      ariaLabel="Example touch actions"
      onclose={() => (sheetOpen = false)}
    >
      <MenuSection>
        <MenuItem icon="icon-[uil--edit]" onclick={() => (sheetOpen = false)}>Edit</MenuItem>
        <MenuItem icon="icon-[uil--copy]" onclick={() => (sheetOpen = false)}>Copy</MenuItem>
      </MenuSection>
      <MenuSection>
        <MenuItem icon="icon-[uil--trash]" tone="danger" onclick={() => (sheetOpen = false)}>
          Delete
        </MenuItem>
      </MenuSection>
    </ContextMenu>
  {/if}
</Story>
