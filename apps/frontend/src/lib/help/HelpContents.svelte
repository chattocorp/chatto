<!-- @component Build-generated article links. The route shows this only beside wide reading columns. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { getTextDirection } from '$lib/i18n/runtime';
  import { m } from '$lib/i18n/messages';
  import type { HelpHeading } from './articles';
  import type { HelpTopic } from './navigation';
  import { withHelpServer } from './links';
  import type { Locale } from '$lib/i18n/locales';
  let {
    headings,
    topic,
    locale,
    serverSegment
  }: { headings: HelpHeading[]; topic: HelpTopic; locale: Locale; serverSegment?: string | null } =
    $props();
</script>

<p class="mb-4 text-xs font-semibold tracking-wider text-text-top uppercase">
  {m('help.contents')}
</p>
<nav aria-label={m('help.contents')} lang={locale} dir={getTextDirection(locale)}>
  <ol class="flex flex-col gap-1 border-s border-border">
    {#each headings as heading (heading.id)}
      <li>
        <a
          href={resolve(withHelpServer(`/help/${topic}#${heading.id}`, serverSegment) as '/')}
          class={[
            'block rounded-e-md py-2 pe-3 text-sm leading-snug text-muted transition-colors hover:bg-surface hover:text-text-top focus-visible:outline-2 focus-visible:outline-action',
            heading.depth === 3 ? 'ps-7' : 'ps-3 font-medium'
          ]}>{heading.title}</a
        >
      </li>
    {/each}
  </ol>
</nav>
