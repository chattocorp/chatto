<!--
@component

Standard command entry for `ContextMenu`. It renders a button by default or an
anchor when `href` is set. Render it below a `ContextMenu`; review fixtures can
provide the same internal menu context directly.

Use `icon` for an Iconify glyph, `leading` for custom leading content such as
an emoji or selection marker, and `trailing` for checks or shortcuts. Omit all
three for a text-only entry.
-->
<script lang="ts">
  /* eslint-disable svelte/no-navigation-without-resolve -- callers must pass an already-resolved URL */
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';

  import { useMenuContext } from './menuContext.svelte';

  type MenuItemRole = 'menuitem' | 'menuitemcheckbox' | 'menuitemradio';
  type MenuItemTone = 'default' | 'danger';

  let {
    href,
    icon,
    mirrorIconInRtl = false,
    tone = 'default',
    selected = false,
    disabled = false,
    busy = false,
    role,
    checked,
    ariaLabel,
    title,
    dataTestid,
    onclick,
    class: className,
    leading,
    trailing,
    children
  }: {
    /** An already-resolved URL. When set, the entry renders as an anchor. */
    href?: string;
    /** Iconify utility class, for example `icon-[uil--copy]`. */
    icon?: string;
    mirrorIconInRtl?: boolean;
    tone?: MenuItemTone;
    selected?: boolean;
    disabled?: boolean;
    busy?: boolean;
    role?: MenuItemRole;
    /** Checked state for checkbox and radio menu items. */
    checked?: boolean;
    ariaLabel?: string;
    title?: string;
    dataTestid?: string;
    onclick?: (event: MouseEvent) => unknown;
    /** Layout-only classes for exceptional placement. */
    class?: ClassValue;
    leading?: Snippet;
    trailing?: Snippet;
    children: Snippet;
  } = $props();

  const menuContext = useMenuContext();
  const isSheet = $derived(menuContext.presentation() === 'sheet');
  const effectiveRole = $derived<MenuItemRole | undefined>(
    role ?? (menuContext.containerRole() === 'menu' ? 'menuitem' : undefined)
  );
  const unavailable = $derived(disabled || busy);
  const itemClasses = $derived([
    'menu-entry',
    isSheet && 'menu-entry-sheet',
    selected && 'menu-entry-selected',
    tone === 'danger' && 'text-danger hover:text-danger',
    className
  ]);
  const leadingClasses = $derived([
    'menu-entry-leading',
    'self-start',
    isSheet ? 'menu-entry-leading-sheet' : 'menu-entry-leading-floating'
  ]);

  function handleClick(event: MouseEvent): void {
    if (unavailable) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    void onclick?.(event);
  }
</script>

{#snippet content()}
  {#if icon}
    <span
      class={['iconify', leadingClasses, icon, mirrorIconInRtl && 'rtl:-scale-x-100']}
      aria-hidden="true"
    ></span>
  {:else if leading}
    <span class={leadingClasses} aria-hidden="true">{@render leading()}</span>
  {/if}
  <span class="min-w-0 flex-1">{@render children()}</span>
  {#if trailing}
    <span class="ms-auto shrink-0" aria-hidden="true">{@render trailing()}</span>
  {/if}
{/snippet}

{#if href}
  <a
    {href}
    class={itemClasses}
    role={effectiveRole}
    aria-checked={checked}
    aria-disabled={unavailable || undefined}
    aria-busy={busy || undefined}
    aria-label={ariaLabel}
    {title}
    tabindex={unavailable ? -1 : undefined}
    data-testid={dataTestid}
    onclick={handleClick}
  >
    {@render content()}
  </a>
{:else}
  <button
    type="button"
    class={itemClasses}
    role={effectiveRole}
    aria-checked={checked}
    aria-busy={busy || undefined}
    aria-label={ariaLabel}
    {title}
    disabled={unavailable}
    data-testid={dataTestid}
    onclick={handleClick}
  >
    {@render content()}
  </button>
{/if}
