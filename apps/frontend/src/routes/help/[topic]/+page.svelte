<!-- @component A readable guide with a local working example and related destinations. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { withHelpServer } from '$lib/help/links';
  import { m } from '$lib/i18n/messages';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { PageTitle, PaneContent, PaneHeader, Panel, LoadingFog, Hint } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import { loadHelpArticle } from '$lib/help/articles';
  import HelpDocument from '$lib/help/HelpDocument.svelte';
  import HelpContents from '$lib/help/HelpContents.svelte';
  import { helpGroups } from '$lib/help/navigation';
  import type { PageData } from './$types';
  let { data }: { data: PageData } = $props();
  const selectedServer = $derived(page.url.searchParams.get('server'));
  const title = $derived(m(`help.${data.topic}.title`));
  const article = $derived(loadHelpArticle(getReactiveLocale(), data.topic));
  const related = $derived(
    helpGroups(selectedServer)
      .flatMap((group) => group.items)
      .filter((item) => item.topic !== data.topic)
  );
</script>

<PageTitle {title} />
<div class="pane-page">
  <PaneHeader
    {title}
    backHref={sidebarNav.isMobile
      ? resolve(withHelpServer('/help', selectedServer) as '/')
      : undefined}
    backLabel={m('help.overview')}
  />
  <PaneContent wide>
    <!-- Clip the panel rim without creating a scroll container for its sticky contents. -->
    <div
      class="@container/help-page mx-auto flex max-w-6xl flex-col gap-6 [&>[data-panel]]:overflow-clip"
    >
      {#await article}
        <LoadingFog class="h-80" />
      {:then { default: Article, locale, metadata }}
        <Panel {title}>
          <div
            class={[
              'grid items-start gap-6 @min-[48rem]/help-page:gap-8',
              metadata.toc.length > 1 && '@min-[48rem]/help-page:grid-cols-[minmax(0,1fr)_14rem]'
            ]}
          >
            {#if metadata.toc.length > 1}
              <aside
                class="hidden min-w-0 @min-[48rem]/help-page:sticky @min-[48rem]/help-page:top-6 @min-[48rem]/help-page:col-start-2 @min-[48rem]/help-page:row-start-1 @min-[48rem]/help-page:block @min-[48rem]/help-page:max-h-[calc(100dvh-10rem)] @min-[48rem]/help-page:overflow-y-auto"
              >
                <HelpContents
                  headings={metadata.toc}
                  topic={data.topic}
                  {locale}
                  serverSegment={selectedServer}
                />
              </aside>
            {/if}
            <article
              class="w-full min-w-0 @min-[48rem]/help-page:col-start-1 @min-[48rem]/help-page:row-start-1 @min-[48rem]/help-page:max-w-prose"
            >
              <HelpDocument {locale}><Article /></HelpDocument>
            </article>
          </div>
        </Panel>
        <Panel title={m('help.design.explore')}>
          <nav aria-label={m('help.design.explore')} class="@container/help-links">
            <div
              class="grid grid-cols-1 gap-3 @min-[36rem]/help-links:grid-cols-2 [&_.button-content]:w-full"
            >
              {#each related as item (item.href)}
                <Button href={resolve(item.href as '/')} variant="neutral" fullWidth>
                  <span class={[item.icon, 'shrink-0 text-xl']} aria-hidden="true"></span><span
                    class="min-w-0 flex-1 text-start break-words whitespace-normal"
                    >{item.label}</span
                  ><span
                    class="iconify icon-[uil--arrow-right] shrink-0 rtl:-scale-x-100"
                    aria-hidden="true"
                  ></span>
                </Button>
              {/each}
            </div>
          </nav>
        </Panel>
      {:catch}
        <Hint tone="danger">{m('common.error.network')}</Hint>
      {/await}
    </div>
  </PaneContent>
</div>
