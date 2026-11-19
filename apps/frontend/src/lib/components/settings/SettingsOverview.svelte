<!-- @component Settings destination cards shared by server settings and signed-out app preferences. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type { SettingsNavGroup } from '$lib/navigation/settingsNav';
  import { Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import DestinationCard from './DestinationCard.svelte';

  let {
    groups,
    showShieldNotice = false
  }: {
    groups: SettingsNavGroup[];
    showShieldNotice?: boolean;
  } = $props();
</script>

<PageTitle title={m('settings.overview.title')} />

<div class="pane-page">
  <PaneHeader title={m('settings.overview.title')} subtitle={m('settings.nav.title')} />
  <PaneContent wide>
    <div class="flex flex-col gap-8">
      {#each groups as group (group.persistKey)}
        {#if group.items.length > 0}
          <section class="flex flex-col gap-4" aria-label={group.label}>
            <h2 class="text-xl font-bold tracking-tight text-text-top">{group.label}</h2>
            {#if group.id === 'server-configuration' && showShieldNotice}
              <Hint tone="warning" icon="icon-[uil--shield]"
                >{m('settings.overview.shield_notice')}</Hint
              >
            {/if}
            <nav
              aria-label={group.label}
              class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
            >
              {#each group.items as item (item.href)}
                <DestinationCard {...item} />
              {/each}
            </nav>
          </section>
        {/if}
      {/each}
    </div>
  </PaneContent>
</div>
