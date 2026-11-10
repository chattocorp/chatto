<!-- @component Build-generated article links. The route shows this only beside wide reading columns. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { m } from '$lib/i18n/messages';
  import type { HelpHeading } from './articles';
  import type { HelpTopic } from './navigation';
  import type { Locale } from '$lib/i18n/locales';
  let { headings, topic, locale }: { headings: HelpHeading[]; topic: HelpTopic; locale: Locale } =
    $props();
</script>

<p class="mb-4 text-xs font-semibold tracking-wider text-text-top uppercase">
  {m('help.contents')}
</p>
<nav
  aria-label={m('help.contents')}
  lang={locale}
  dir={locale === 'ar' || locale === 'he-IL' ? 'rtl' : 'ltr'}
>
  <ol class="flex flex-col gap-1 border-s border-border">
    {#each headings as heading (heading.id)}
      <li>
        <a
          href={resolve(`/help/[topic]#${heading.id}`, { topic })}
          class={[
            'block rounded-e-md py-2 pe-3 text-sm leading-snug text-muted transition-colors hover:bg-surface hover:text-text-top focus-visible:outline-2 focus-visible:outline-action',
            heading.depth === 3 ? 'ps-7' : 'ps-3 font-medium'
          ]}>{heading.title}</a
        >
      </li>
    {/each}
  </ol>
</nav>
