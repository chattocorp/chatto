<script lang="ts">
  import * as m from '$lib/i18n/messages';
  import type { ToastAction, ToastTone } from './toastState.svelte';

  let {
    tone,
    message,
    action,
    onDismiss
  }: {
    tone: ToastTone;
    message: string;
    action?: ToastAction;
    onDismiss: () => void;
  } = $props();

  const icons: Record<ToastTone, string> = {
    error: 'uil--times-circle',
    success: 'uil--check-circle',
    info: 'uil--info-circle',
    warning: 'uil--exclamation-triangle'
  };

  const iconColors: Record<ToastTone, string> = {
    error: 'text-error',
    success: 'text-success',
    info: 'text-action',
    warning: 'text-warning'
  };

  function handleActionClick() {
    action?.onClick();
    onDismiss(); // Close toast after action is clicked
  }
</script>

<div class="w-full max-w-96 min-w-0 menu text-left sm:w-auto">
  <div class="flex min-h-10 items-center gap-3 menu-section px-3 py-2">
    <span class={['iconify size-5 shrink-0', icons[tone], iconColors[tone]]} aria-hidden="true"
    ></span>
    <span class="min-w-0 flex-1 leading-snug break-words">{message}</span>
    {#if action}
      <button type="button" class="btn-secondary btn-xs shrink-0" onclick={handleActionClick}>
        {action.label}
      </button>
    {/if}
    <button
      type="button"
      class="btn-ghost btn-xs shrink-0"
      onclick={onDismiss}
      aria-label={m['ui.toast.dismiss']()}
    >
      <span class="iconify size-4 uil--times" aria-hidden="true"></span>
    </button>
  </div>
</div>
