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
  const descriptionId = $props.id();
</script>

<a
  href={resolve(href as '/')}
  aria-label={label}
  aria-describedby={description ? descriptionId : undefined}
  class="group hover-lift-group min-w-0 cursor-pointer rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-action"
>
  <!-- The link stays fixed while its visual surface lifts. -->
  <div class="flex h-full hover-lift flex-row inset-frame sm:flex-col">
    <div class="w-24 shrink-0 overflow-hidden inset-content sm:w-auto">
      {#if artwork}<div aria-hidden="true">{@render artwork()}</div>{:else}<SettingsCardArtwork
          {href}
          {icon}
        />{/if}
    </div>
    <div class="flex min-w-0 flex-1 flex-col gap-2 p-3">
      <span class="text-base font-bold tracking-tight text-text-top">{label}</span>
      {#if description}<p id={descriptionId} class="text-sm text-muted sm:min-h-[3lh]">
          {description}
        </p>{/if}
    </div>
  </div>
</a>
