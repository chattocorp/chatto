<!--
@component

The highlights of each Chatto release, latest first. The highlights come from
the `highlight` notes in the root `CHANGELOG.md` and stay in English. Each
released version links to its full release notes on GitHub.
Development builds also show the pending highlights, without a version.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import releases from 'virtual:chatto-release-highlights';
  import { m } from '$lib/i18n/messages';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import {
    Hint,
    LoadingDots,
    MarkdownHtml,
    PageTitle,
    PaneContent,
    PaneHeader,
    Panel
  } from '$lib/ui';

  const CHANGELOG_URL = 'https://docs.chatto.run/changelog/';

  const rendered = import('$lib/markdown').then(({ renderInlineMarkdown }) =>
    releases.map(({ version, highlights }) => ({
      version,
      highlights: highlights.map(renderInlineMarkdown)
    }))
  );
</script>

{#snippet externalIcon()}
  <span class="iconify icon-[mdi--open-in-new] text-sm" aria-hidden="true"></span>
{/snippet}

<PageTitle title={m('help.whats_new.title')} />
<div class="pane-page">
  <PaneHeader
    title={m('help.whats_new.title')}
    backHref={sidebarNav.isMobile ? resolve('/help') : undefined}
    backLabel={m('help.overview')}
  />
  <PaneContent>
    <div class="mx-auto flex max-w-3xl flex-col gap-6">
      <p class="text-muted">
        {m('help.whats_new.description')}
        <a
          href={CHANGELOG_URL}
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center gap-1.5 link"
        >
          {m('help.whats_new.full_changelog')}{@render externalIcon()}
        </a>
      </p>
      {#await rendered}
        <LoadingDots class="h-40" />
      {:then releases}
        {#each releases as release (release.version ?? '')}
          <Panel title={release.version ?? m('help.whats_new.unreleased')}>
            {#if release.highlights.length}
              <ul class="flex flex-col gap-3" lang="en" dir="ltr">
                {#each release.highlights as html, index (index)}
                  <li class="flex gap-3 leading-relaxed">
                    <span
                      class="iconify mt-1 icon-[uil--star] shrink-0 text-action"
                      aria-hidden="true"
                    ></span>
                    <span class="min-w-0"><MarkdownHtml {html} /></span>
                  </li>
                {/each}
              </ul>
            {/if}
            {#if release.version}
              <p class={['text-sm', release.highlights.length > 0 && 'mt-4']}>
                <a
                  href={`https://github.com/chattocorp/chatto/releases/tag/v${release.version}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex items-center gap-1.5 link"
                >
                  {m('help.whats_new.release_notes', {
                    version: release.version
                  })}{@render externalIcon()}
                </a>
              </p>
            {/if}
          </Panel>
        {:else}
          <Hint>{m('help.whats_new.empty')}</Hint>
        {/each}
      {:catch}
        <Hint tone="danger">{m('common.error.network')}</Hint>
      {/await}
    </div>
  </PaneContent>
</div>
