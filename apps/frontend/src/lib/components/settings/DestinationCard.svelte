<!-- @component Shared overview link with decorative artwork and a description. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import SettingsCardArtwork from './SettingsCardArtwork.svelte';
  import type { Snippet } from 'svelte';

  let {
    href,
    label,
    icon,
    description,
    artwork
  }: {
    href: string;
    label: string;
    icon: string;
    description?: string;
    /** Optional destination-specific illustration; kept decorative by the card. */
    artwork?: Snippet;
  } = $props();
  const titleId = $props.id();
</script>

<div class="group hover-lift-group relative min-w-0 rounded-lg">
  <!-- Keep the hit area fixed and the description separate from the link's label. -->
  <a
    href={resolve(href as '/')}
    aria-labelledby={titleId}
    aria-describedby={description ? `${titleId}-description` : undefined}
    class="absolute inset-0 z-10 cursor-pointer rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-action"
    ><span class="sr-only">{label}</span></a
  >
  <div class="flex h-full hover-lift flex-row inset-frame sm:flex-col">
    <div class="w-24 shrink-0 overflow-hidden inset-content sm:w-auto">
      {#if artwork}<div aria-hidden="true">{@render artwork()}</div>{:else}<SettingsCardArtwork
          {href}
          {icon}
        />{/if}
    </div>
    <div class="flex min-w-0 flex-1 flex-col gap-2 p-3">
      <span id={titleId} class="text-base font-bold tracking-tight text-text-top">{label}</span>
      {#if description}<p id={`${titleId}-description`} class="text-sm text-muted sm:min-h-[3lh]">
          {description}
        </p>{/if}
    </div>
  </div>
</div>
