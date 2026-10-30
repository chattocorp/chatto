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
              class="grid grid-cols-2 gap-4 lg:grid-cols-3 xl:grid-cols-4"
            >
              {#each group.items as item (item.href)}
                <a
                  href={resolve(item.href as '/')}
                  aria-label={item.label}
                  aria-describedby={`${item.href}-description`}
                  class="group hover-lift-group min-w-0 cursor-pointer rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-action"
                >
                  <!-- Keep the link's hover area fixed while its visual surface lifts. -->
                  <div class="flex h-full hover-lift flex-col inset-frame">
                    <div class="overflow-hidden inset-content">
                      <SettingsCardArtwork href={item.href} icon={item.icon} />
                    </div>
                    <div class="flex flex-1 flex-col gap-2 p-3">
                      <span class="text-base font-bold tracking-tight text-text-top"
                        >{item.label}</span
                      >
                      <p id={`${item.href}-description`} class="min-h-[3lh] text-sm text-muted">
                        {item.description}
                      </p>
                    </div>
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
