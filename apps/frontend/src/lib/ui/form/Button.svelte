<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    type = 'button',
    variant = 'action',
    size = 'md',
    loading = false,
    disabled = false,
    fullWidth = false,
    defaultAction = false,
    loadingText,
    href,
    opensInNewTab = false,
    form,
    onclick,
    label,
    title,
    children
  }: {
    type?: 'button' | 'submit' | 'reset';
    variant?:
      'action' | 'neutral' | 'secondary' | 'ghost' | 'warning' | 'danger' | 'danger-secondary';
    size?: 'sm' | 'md' | 'lg';
    loading?: boolean;
    disabled?: boolean;
    fullWidth?: boolean;
    /** Marks this as its containing dialog's action for the Enter key. */
    defaultAction?: boolean;
    loadingText?: string;
    /** When provided, renders as an <a> link instead of a <button> */
    href?: string;
    /** Opens an href in a separate browsing context without opener access. */
    opensInNewTab?: boolean;
    /** ID of the form this button submits when rendered outside that form. */
    form?: string;
    onclick?: (e: MouseEvent) => void;
    /** Accessible name for icon-only buttons. */
    label?: string;
    /** Optional native hover hint. */
    title?: string;
    children: Snippet;
  } = $props();

  const variantClasses = {
    action: 'btn-action',
    neutral: 'btn-neutral',
    secondary: 'btn-secondary',
    ghost: 'btn-ghost',
    warning: 'btn-warning',
    danger: 'btn-danger',
    'danger-secondary': 'btn-danger-secondary'
  };

  const sizeClasses = {
    sm: 'btn-sm',
    md: '',
    lg: 'btn-lg'
  };

  function handleClick(e: MouseEvent) {
    if (disabled || loading) {
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    onclick?.(e);
  }
</script>

{#snippet content()}
  {#if loading}
    {#if loadingText}
      {loadingText}
    {:else}
      <span class="inline-flex items-center gap-2">
        <span class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
        ></span>
        {@render children()}
      </span>
    {/if}
  {:else}
    {@render children()}
  {/if}
{/snippet}

{#if href}
  <!-- eslint-disable svelte/no-navigation-without-resolve -- href is a prop; callers pass resolved app paths or external URLs -->
  <a
    {href}
    target={opensInNewTab ? '_blank' : undefined}
    rel={opensInNewTab ? 'noopener noreferrer' : undefined}
    onclick={handleClick}
    aria-busy={loading || undefined}
    aria-disabled={disabled || loading || undefined}
    aria-label={label}
    {title}
    data-dialog-default={defaultAction || undefined}
    tabindex={disabled || loading ? -1 : undefined}
    class={[
      variantClasses[variant],
      sizeClasses[size],
      fullWidth ? 'w-full' : '',
      disabled || loading ? 'pointer-events-none opacity-60' : ''
    ]}
  >
    {@render content()}
  </a>
  <!-- eslint-enable svelte/no-navigation-without-resolve -->
{:else}
  <button
    {type}
    {form}
    onclick={handleClick}
    disabled={disabled || loading}
    aria-busy={loading || undefined}
    aria-label={label}
    {title}
    data-dialog-default={defaultAction || undefined}
    class={[variantClasses[variant], sizeClasses[size], fullWidth ? 'w-full' : '']}
  >
    {@render content()}
  </button>
{/if}
