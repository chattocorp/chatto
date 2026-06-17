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

  const toneStyles: Record<Tone, { shell: string; icon: string; primary: string }> = {
    info: {
      shell: 'border-border bg-surface text-text shadow-lg shadow-black/15',
      icon: 'bg-primary/10 text-primary',
      primary: 'bg-primary text-white hover:bg-primary/90'
    },
    success: {
      shell: 'border-success/25 bg-surface text-text shadow-lg shadow-success/10',
      icon: 'bg-success/10 text-success',
      primary: 'bg-success text-white hover:bg-success/90'
    },
    warning: {
      shell: 'border-warning/30 bg-surface text-text shadow-lg shadow-warning/10',
      icon: 'bg-warning/10 text-warning',
      primary: 'bg-warning text-white hover:bg-warning/90'
    },
    danger: {
      shell: 'border-danger/30 bg-surface text-text shadow-lg shadow-danger/10',
      icon: 'bg-danger/10 text-danger',
      primary: 'bg-danger text-white hover:bg-danger/90'
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

<div class="pointer-events-none fixed top-3 right-3 left-3 z-[60] flex justify-center sm:top-4">
  <section
    class={[
      'pointer-events-auto flex w-full max-w-2xl items-start gap-3 rounded-lg border px-3 py-3 sm:px-4',
      toneStyles[tone].shell
    ]}
    role="status"
    aria-live="polite"
  >
    <span
      class={[
        'iconify mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg text-lg',
        resolvedIcon,
        toneStyles[tone].icon
      ]}
    ></span>

    <div class="min-w-0 flex-1">
      <p class="text-sm font-semibold">{title}</p>
      <p class="mt-0.5 text-sm text-muted">{message}</p>
      {#if children}
        <div class="mt-2 text-sm text-muted">
          {@render children()}
        </div>
      {/if}
    </div>

    <div class="flex shrink-0 flex-col gap-2 sm:flex-row">
      {#if secondaryAction}
        <button
          type="button"
          class="cursor-pointer rounded-lg px-3 py-1.5 text-sm font-medium text-muted transition-colors hover:bg-surface-200 hover:text-text disabled:cursor-not-allowed disabled:opacity-60"
          onclick={secondaryAction.onclick}
          disabled={loading}
        >
          {secondaryAction.label}
        </button>
      {/if}
      {#if primaryAction}
        <button
          type="button"
          class={[
            'inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60',
            toneStyles[tone].primary
          ]}
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
  </section>
</div>
