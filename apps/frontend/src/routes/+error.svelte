<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';
</script>

<PageTitle title={m('error_page.title')} />

<div class="flex min-h-full flex-1 items-center justify-center px-6 py-12 text-center">
  <section class="flex max-w-md flex-col items-center gap-5" aria-labelledby="error-page-title">
    <span class="iconify icon-[uil--exclamation-triangle] text-5xl text-muted" aria-hidden="true"
    ></span>

    <div class="flex flex-col gap-2">
      <h1 id="error-page-title" class="text-xl font-semibold text-balance text-text-top">
        {m('error_page.title')}
      </h1>
      <p class="leading-7 text-pretty text-muted">
        {m('error_page.description')}
      </p>
    </div>

    <Button href="/" variant="secondary">{m('error_page.home_link')}</Button>
  </section>
</div>
