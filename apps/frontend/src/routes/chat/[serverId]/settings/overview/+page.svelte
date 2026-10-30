<script lang="ts">
  import { resolve } from '$app/paths';
  import { getAdminNavItems } from '$lib/components/chat/adminNav';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import SettingsCardArtwork from './SettingsCardArtwork.svelte';

  const scope = useServerScope();
  const serverSegment = serverIdToSegment(scope.serverId);
  const groups = $derived(
    getSettingsNavGroups(
      scope.serverId,
      serverSegment,
      getAdminNavItems({ serverSegment, permissions: scope.store.permissions })
    )
  );
  const privilegedMode = $derived(scope.store.projection.viewer?.privilegedMode);
</script>

<PageTitle title={m('settings.overview.title')} />

<div class="pane-page">
  <PaneHeader title={m('settings.overview.title')} subtitle={m('settings.nav.title')} />
  <PaneContent wide>
    <div class="flex flex-col gap-8">
      {#if privilegedMode?.available && !privilegedMode.active}
        <Hint tone="warning" icon="icon-[uil--shield]">{m('settings.overview.shield_notice')}</Hint>
      {/if}
      {#each groups as group (group.persistKey)}
        {#if group.items.length > 0}
          <section class="flex flex-col gap-4" aria-label={group.label}>
            <h2 class="text-xl font-bold tracking-tight text-text-top">{group.label}</h2>
            <nav
              aria-label={group.label}
              class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4"
            >
              {#each group.items as item (item.href)}
                <a
                  href={resolve(item.href as '/')}
                  aria-label={item.label}
                  aria-describedby={`${item.href}-description`}
                  class="group flex min-w-0 cursor-pointer flex-col overflow-hidden rounded-2xl border border-border bg-surface transition-colors duration-200 hover:border-action/40 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-action motion-reduce:transition-none"
                >
                  <SettingsCardArtwork href={item.href} icon={item.icon} />
                  <div class="flex flex-1 flex-col gap-2 p-4">
                    <span class="text-xl font-bold tracking-tight text-text-top">{item.label}</span>
                    <p id={`${item.href}-description`} class="text-base text-muted">
                      {item.description}
                    </p>
                  </div>
                </a>
              {/each}
            </nav>
          </section>
        {/if}
      {/each}
    </div>
  </PaneContent>
</div>
