<!--
@component

One public recommendation of a directory server. The testimonial stays
visually separate from the recommended server profile, while the device-local
source name and icon identify who supplied it.
-->
<script lang="ts">
  import ServerLogo from '$lib/components/ServerLogo.svelte';
  import TestimonialText from '$lib/components/TestimonialText.svelte';
  import { m } from '$lib/i18n/messages';

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
  <blockquote class="text-sm">
    <TestimonialText {testimonial} />
  </blockquote>

  <figcaption class="mt-4 flex min-w-0 items-center gap-3 border-t border-border pt-3 text-sm">
    <div
      class="h-9 w-9 shrink-0 overflow-hidden rounded-lg bg-surface-emphasized"
      data-testid="server-testimonial-source-icon"
      aria-hidden="true"
    >
      <ServerLogo server={source} fill />
    </div>
    <div class="min-w-0 truncate font-medium text-muted">
      <bdi dir="auto">
        {m('add_server.directory.recommended_by', { servers: sourceName })}
      </bdi>
    </div>
  </figcaption>
</figure>
