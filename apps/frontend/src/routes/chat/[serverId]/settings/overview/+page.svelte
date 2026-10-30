<script lang="ts">
  import { resolve } from '$app/paths';
  import { getAdminNavItems } from '$lib/components/chat/adminNav';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';

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
        <Hint icon="icon-[uil--shield]">{m('settings.overview.shield_notice')}</Hint>
      {/if}
      {#each groups as group (group.persistKey)}
        {#if group.items.length > 0}
          <section class="flex flex-col gap-4" aria-label={group.label}>
            <h2 class="text-lg font-bold text-text-top">{group.label}</h2>
            <nav
              aria-label={group.label}
              class={[
                'grid grid-cols-1 gap-4 sm:grid-cols-2',
                group.items.length > 4 && 'xl:grid-cols-3'
              ]}
            >
              {#each group.items as item (item.href)}
                <a
                  href={resolve(item.href as '/')}
                  aria-label={item.label}
                  aria-describedby={`${item.href}-description`}
                  class="group flex min-w-0 cursor-pointer flex-col items-start gap-5 surface-box p-6 hover:bg-surface-emphasized focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
                >
                  <span
                    class="flex size-14 items-center justify-center rounded-xl bg-action/10 text-action group-hover:bg-action/15"
                  >
                    <span aria-hidden="true" class={['text-3xl', item.icon]}></span>
                  </span>
                  <div class="flex flex-col gap-2">
                    <span class="text-lg font-bold text-text-top">{item.label}</span>
                    <p id={`${item.href}-description`} class="text-base leading-relaxed text-muted">
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
