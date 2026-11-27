<!-- @component Shared overview link with decorative artwork and a description. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { resolve } from '$app/paths';
  import SettingsCardArtwork from './SettingsCardArtwork.svelte';
  import type { Snippet } from 'svelte';

  let {
    href,
    label,
    icon,
    description,
    artwork,
    requestAccess,
    shieldActive,
    onclick
  }: {
    href: string;
    label: string;
    icon: string;
    description?: string;
    /** Optional destination-specific illustration; kept decorative by the card. */
    artwork?: Snippet;
    /** Open Shield Mode to request access to this page. */
    requestAccess?: () => void;
    /** This page uses the active Shield Mode roles. */
    shieldActive?: boolean;
    /** Runs when the link is followed, for example to close a containing dialog. */
    onclick?: () => void;
  } = $props();
  const hitArea =
    'absolute inset-0 z-10 cursor-pointer rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-action';
  const titleId = $props.id();
</script>

<div class="group hover-lift-group relative min-w-0 rounded-lg">
  <!-- Keep the hit area fixed and the description separate from the link's label. -->
  {#if requestAccess}
    <button
      type="button"
      class={hitArea}
      aria-haspopup="dialog"
      onclick={requestAccess}
      aria-labelledby={titleId}
      title={m('chat.privileged_mode.enable')}
      aria-describedby={description
        ? `${titleId}-shield ${titleId}-description`
        : `${titleId}-shield`}
      ><span id={`${titleId}-shield`} class="sr-only">{m('chat.privileged_mode.enable')}</span
      ></button
    >
  {:else}
    {#if shieldActive}
      <span id={`${titleId}-active`} hidden>{m('settings.overview.shield_active')}</span>
    {/if}
    <a
      href={resolve(href as '/')}
      {onclick}
      title={shieldActive ? m('settings.overview.shield_active') : undefined}
      aria-labelledby={titleId}
      aria-describedby={[
        shieldActive && `${titleId}-active`,
        description && `${titleId}-description`
      ]
        .filter(Boolean)
        .join(' ') || undefined}
      class={hitArea}><span class="sr-only">{label}</span></a
    >
  {/if}
  <div
    class={[
      'flex h-full hover-lift flex-row inset-frame sm:flex-col',
      requestAccess && 'opacity-65'
    ]}
  >
    <div class="w-24 shrink-0 overflow-hidden inset-content sm:w-auto">
      {#if artwork}<div aria-hidden="true">{@render artwork()}</div>{:else}<SettingsCardArtwork
          {href}
          {icon}
        />{/if}
    </div>
    <div class="flex min-w-0 flex-1 flex-col gap-2 p-3">
      <span id={titleId} class="text-base font-bold tracking-tight text-text-top"
        >{label}
        {#if requestAccess}<span class="iconify icon-[uil--shield] align-middle" aria-hidden="true"
          ></span>{:else if shieldActive}<span
            class="iconify icon-[uil--shield-check] align-middle text-warning"
            aria-hidden="true"
          ></span>{/if}</span
      >
      {#if description}<p id={`${titleId}-description`} class="text-sm text-muted sm:min-h-[3lh]">
          {description}
        </p>{/if}
    </div>
  </div>
</div>
