<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  import MenuItem from './MenuItem.svelte';
  import MenuSection from './MenuSection.svelte';

  const { Story } = defineMeta({
    title: 'UI/MenuItem',
    component: MenuItem,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Standard command entry for context menus and action sheets. Supports buttons, links, optional or custom leading content, trailing content, destructive tone, disabled state, wrapped labels, and RTL icon mirroring.'
        }
      }
    }
  });
</script>

<script lang="ts">
  import { provideMenuContext } from './menuContext.svelte';

  provideMenuContext({
    presentation: () => 'floating',
    containerRole: () => 'menu'
  });
</script>

<Story name="Common states" asChild>
  <div class="w-64 menu" role="menu" aria-label="Common menu entry states">
    <MenuSection>
      <MenuItem icon="icon-[uil--edit]">Edit</MenuItem>
      <MenuItem icon="icon-[uil--copy]">Copy</MenuItem>
      <MenuItem>Text-only action</MenuItem>
      <MenuItem icon="icon-[uil--trash]" tone="danger">Delete</MenuItem>
      <MenuItem icon="icon-[uil--lock]" disabled>Unavailable</MenuItem>
    </MenuSection>
  </div>
</Story>

<Story name="Wrapped and selected" asChild>
  <div class="w-64 menu" role="menu" aria-label="Wrapped and selected menu entries">
    <MenuSection ariaLabel="Account actions">
      <MenuItem icon="icon-[uil--servers]">In der Serveradministration anzeigen</MenuItem>
      <MenuItem role="menuitemradio" checked selected>
        {#snippet leading()}<span>🙂</span>{/snippet}
        {#snippet trailing()}
          <span class="iconify icon-[uil--check]"></span>
        {/snippet}
        Custom leading and trailing content
      </MenuItem>
    </MenuSection>
  </div>
</Story>

<Story name="RTL" asChild>
  <div class="w-64 menu" role="menu" aria-label="RTL menu entries" dir="rtl">
    <MenuSection>
      <MenuItem icon="icon-[uil--corner-up-left]" mirrorIconInRtl>ردّ</MenuItem>
      <MenuItem icon="icon-[uil--copy]">نسخ الرابط</MenuItem>
    </MenuSection>
  </div>
</Story>
