<script lang="ts">
  import { resolve } from '$app/paths';
  import { getAdminNavItems } from '$lib/components/chat/adminNav';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, PageTitle, PaneContent, PaneHeader, Panel } from '$lib/ui';

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
    <div class="flex flex-col gap-6">
      {#if privilegedMode?.available && !privilegedMode.active}
        <Hint icon="icon-[uil--shield]">{m('settings.overview.shield_notice')}</Hint>
      {/if}
      {#each groups as group (group.persistKey)}
        {#if group.items.length > 0}
          <Panel title={group.label}>
            <nav
              aria-label={group.label}
              class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3"
            >
              {#each group.items as item (item.href)}
                <a
                  href={resolve(item.href as '/')}
                  class="flex min-w-0 cursor-pointer items-center gap-3 surface-box p-4 hover:bg-surface-emphasized focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
                >
                  <span aria-hidden="true" class={['text-xl text-muted', item.icon]}></span>
                  <span>{item.label}</span>
                </a>
              {/each}
            </nav>
          </Panel>
        {/if}
      {/each}
    </div>
  </PaneContent>
</div>
