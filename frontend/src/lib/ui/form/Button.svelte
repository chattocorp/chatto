<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- href is a prop; callers pass already-resolved paths */
  import type { Snippet } from 'svelte';

  let {
    type = 'button',
    variant = 'primary',
    size = 'md',
    loading = false,
    disabled = false,
    fullWidth = false,
    loadingText,
    href,
    onclick,
    children
  }: {
    type?: 'button' | 'submit' | 'reset';
    variant?: 'primary' | 'accent' | 'secondary' | 'ghost' | 'warning' | 'danger';
    size?: 'sm' | 'md' | 'lg';
    loading?: boolean;
    disabled?: boolean;
    fullWidth?: boolean;
    loadingText?: string;
    /** When provided, renders as an <a> link instead of a <button> */
    href?: string;
    onclick?: (e: MouseEvent) => void;
    children: Snippet;
  } = $props();

  const variantClasses = {
    primary: 'btn-primary',
    accent: 'btn-accent',
    secondary: 'btn-secondary',
    ghost: 'btn-ghost',
    warning: 'btn-warning',
    danger: 'btn-danger'
  };

  const sizeClasses = {
    sm: 'btn-sm',
    md: '',
    lg: 'btn-lg'
  };
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
  <a
    {href}
    aria-busy={loading || undefined}
    class="{variantClasses[variant]} {sizeClasses[size]} {fullWidth ? 'w-full' : ''}"
  >
    {@render content()}
  </a>
{:else}
  <button
    {type}
    {onclick}
    disabled={disabled || loading}
    aria-busy={loading || undefined}
    class="{variantClasses[variant]} {sizeClasses[size]} {fullWidth ? 'w-full' : ''}"
  >
    {@render content()}
  </button>
{/if}
