<script lang="ts">
  import * as m from '$lib/i18n/messages';
  import { Button } from '$lib/ui/form';
</script>

<div class="flex min-h-full flex-1 items-center justify-center px-6 py-12 text-center">
  <section class="flex max-w-md flex-col items-center gap-5" aria-labelledby="error-page-title">
    <div
      class="flex h-14 w-14 items-center justify-center rounded-2xl bg-surface-100 text-muted ring-1 ring-text/5"
      aria-hidden="true"
    >
      <span class="iconify text-3xl uil--exclamation-triangle"></span>
    </div>

    <div class="flex flex-col gap-2">
      <h1 id="error-page-title" class="text-xl font-semibold text-balance text-text-top">
        {m['error_page.title']()}
      </h1>
      <p class="leading-7 text-pretty text-muted">
        {m['error_page.description']()}
      </p>
    </div>

    <Button href="/" variant="secondary">{m['error_page.home_link']()}</Button>
  </section>
</div>
