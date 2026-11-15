<!-- @component Help overview reuses the settings destination cards. -->
<script lang="ts">
  import { page } from '$app/state';
  import DestinationCard from '$lib/components/settings/DestinationCard.svelte';
  import HelpArtwork from '$lib/help/HelpArtwork.svelte';
  import { helpGroups } from '$lib/help/navigation';
  import { m } from '$lib/i18n/messages';
  import { PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  const groups = $derived(helpGroups(page.url.searchParams.get('server')));
</script>

<PageTitle title={m('ui.help')} />
<div class="pane-page">
  <PaneHeader title={m('help.overview')} subtitle={m('ui.help')} />
  <PaneContent wide>
    <div class="mx-auto flex max-w-5xl flex-col gap-12 py-4 md:py-8">
      <header class="flex max-w-2xl flex-col gap-5">
        <h1 class="text-4xl font-bold tracking-tight text-text-top sm:text-5xl">
          {m('help.design.welcome')}
        </h1>
        <p class="text-xl leading-relaxed text-muted">{m('help.intro')}</p>
      </header>
      {#each groups as group (group.persistKey)}
        <section class="flex flex-col gap-4" aria-label={group.label}>
          <h2 class="text-xl font-bold tracking-tight text-text-top">{group.label}</h2>
          <nav aria-label={group.label} class="grid grid-cols-1 gap-5 sm:grid-cols-2">
            {#each group.items as item (item.href)}
              <DestinationCard {...item}>
                {#snippet artwork()}<HelpArtwork topic={item.topic} />{/snippet}
              </DestinationCard>
            {/each}
          </nav>
        </section>
      {/each}
    </div>
  </PaneContent>
</div>
