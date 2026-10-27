<!-- @component
Shared search section for command palettes. The caller owns filtering and
keyboard navigation; optional trailing content holds palette actions or hints.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import MenuSection from './MenuSection.svelte';

  let {
    value = $bindable(''),
    label,
    placeholder = label,
    loading = false,
    focusOnMount = false,
    class: className,
    onvaluechange,
    onkeydown,
    children
  }: {
    value?: string;
    label: string;
    placeholder?: string;
    loading?: boolean;
    /** Use when the palette itself does not already focus its first field. */
    focusOnMount?: boolean;
    class?: ClassValue;
    onvaluechange?: (value: string) => void;
    onkeydown?: (event: KeyboardEvent) => void;
    children?: Snippet;
  } = $props();

  function focusInput(node: HTMLInputElement) {
    queueMicrotask(() => {
      if (node.isConnected) node.focus();
    });
  }
</script>

<MenuSection class={className}>
  <div class="flex min-h-10 items-center gap-2 px-3 py-1.5">
    <span class="iconify sidebar-icon icon-[uil--search] text-muted" aria-hidden="true"></span>
    <!-- Keep input text at 16px to prevent iOS zoom on focus. -->
    <input
      {@attach focusOnMount ? focusInput : undefined}
      bind:value
      oninput={(event) => onvaluechange?.(event.currentTarget.value)}
      {onkeydown}
      type="text"
      {placeholder}
      aria-label={label}
      class="min-w-0 flex-1 bg-transparent text-[16px] text-text outline-none placeholder:text-muted"
    />
    {#if loading}
      <span
        class="iconify sidebar-icon icon-[uil--spinner-alt] animate-spin text-muted"
        aria-hidden="true"
      ></span>
    {/if}
    {@render children?.()}
  </div>
</MenuSection>
