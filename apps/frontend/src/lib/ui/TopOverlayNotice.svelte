<!--
@component

Reusable toast-style notice for persistent, user-actionable prompts that should
float above the app chrome. Unlike transient toasts, callers control when this
appears and disappears.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  type Tone = 'info' | 'success' | 'warning' | 'danger';

  export type TopOverlayNoticeAction = {
    label: string;
    onclick: () => void;
    icon?: string;
  };

  let {
    title,
    message,
    tone = 'info',
    icon,
    primaryAction,
    secondaryAction,
    loading = false,
    children
  }: {
    title: string;
    message: string;
    tone?: Tone;
    icon?: string;
    primaryAction?: TopOverlayNoticeAction;
    secondaryAction?: TopOverlayNoticeAction;
    loading?: boolean;
    children?: Snippet;
  } = $props();

  const toneStyles: Record<Tone, { icon: string; primary: string }> = {
    info: {
      icon: 'text-muted',
      primary: 'btn-accent'
    },
    success: {
      icon: 'text-success',
      primary: 'btn-accent'
    },
    warning: {
      icon: 'text-warning',
      primary: 'btn-warning'
    },
    danger: {
      icon: 'text-danger',
      primary: 'btn-danger'
    }
  };

  const defaultIcons: Record<Tone, string> = {
    info: 'uil--info-circle',
    success: 'uil--check-circle',
    warning: 'uil--exclamation-triangle',
    danger: 'uil--times-circle'
  };

  const resolvedIcon = $derived(icon ?? defaultIcons[tone]);
</script>

<div
  class="pointer-events-none fixed top-[calc(env(safe-area-inset-top,0px)+0.75rem)] right-3 left-3 z-[60] flex justify-center sm:top-[calc(env(safe-area-inset-top,0px)+1rem)]"
>
  <section
    class="pointer-events-auto w-full max-w-4xl menu"
    role="status"
    aria-live="polite"
  >
    <div class="flex flex-col gap-3 menu-section px-3 py-2 md:flex-row md:items-center">
      <span
        class={['iconify mt-0.5 shrink-0 text-lg md:mt-0', resolvedIcon, toneStyles[tone].icon]}
        aria-hidden="true"
      ></span>

      <div class="min-w-0 flex-1">
        <p class="text-sm font-medium text-text">{title}</p>
        <p class="mt-0.5 text-sm text-muted">{message}</p>
        {#if children}
          <div class="mt-2 text-sm text-muted">
            {@render children()}
          </div>
        {/if}
      </div>

      <div class="flex shrink-0 items-center gap-1.5">
        {#if secondaryAction}
          <button
            type="button"
            class="btn-secondary btn-sm"
            onclick={secondaryAction.onclick}
            disabled={loading}
          >
            {secondaryAction.label}
          </button>
        {/if}
        {#if primaryAction}
          <button
            type="button"
            class={[toneStyles[tone].primary, 'btn-sm']}
            onclick={primaryAction.onclick}
            disabled={loading}
          >
            {#if loading}
              <span class="size-4 animate-spin rounded-full border-2 border-current border-t-transparent"
              ></span>
            {:else if primaryAction.icon}
              <span class={['iconify text-base', primaryAction.icon]}></span>
            {/if}
            <span>{primaryAction.label}</span>
          </button>
        {/if}
      </div>
    </div>
  </section>
</div>
