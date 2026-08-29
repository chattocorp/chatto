<!--
@component

One public recommendation of a directory server. The testimonial stays
visually separate from the recommended server profile, while the device-local
source name and icon identify who supplied it.
-->
<script lang="ts">
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import TestimonialText from '$lib/components/TestimonialText.svelte';

  let {
    testimonial,
    sourceName,
    sourceIconUrl = null
  }: {
    testimonial: string;
    sourceName: string;
    sourceIconUrl?: string | null;
  } = $props();

  const source = $derived({ name: sourceName, logoUrl: sourceIconUrl });
</script>

<figure class="rounded-xl border border-border bg-surface p-4" data-testid="server-testimonial">
  <figcaption class="flex min-w-0 items-center gap-3 text-sm">
    <div
      class="h-9 w-9 shrink-0 overflow-hidden rounded-lg bg-surface-emphasized"
      data-testid="server-testimonial-source-icon"
      aria-hidden="true"
    >
      <ServerLogo server={source} fill />
    </div>
    <div class="min-w-0 truncate font-semibold text-text-top">
      <bdi dir="auto">{sourceName}</bdi>
    </div>
  </figcaption>

  <blockquote
    class="relative mt-3 rounded-xl bg-surface-emphasized p-3 text-sm before:absolute before:-top-1 before:start-4 before:h-3 before:w-3 before:rotate-45 before:bg-surface-emphasized"
  >
    <div class="relative">
      <TestimonialText {testimonial} />
    </div>
  </blockquote>
</figure>
