<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import * as m from '$lib/i18n/messages';
  import FloatingPopover from '$lib/ui/FloatingPopover.svelte';
  import FormField from './FormField.svelte';

  let {
    id,
    label,
    value = $bindable(''),
    text = $bindable(''),
    items,
    getValue,
    getLabel,
    placeholder,
    description,
    error,
    disabled = false,
    loading = false,
    allowFreeform = true,
    emptyMessage = 'No matches',
    clearLabel = 'Clear',
    class: className,
    item,
    ontextchange,
    onselect,
    onclear
  }: {
    id: string;
    label: string;
    value?: string;
    text?: string;
    items: T[];
    getValue: (item: T) => string;
    getLabel: (item: T) => string;
    placeholder?: string;
    description?: string;
    error?: string;
    disabled?: boolean;
    loading?: boolean;
    allowFreeform?: boolean;
    emptyMessage?: string;
    clearLabel?: string;
    class?: ClassValue;
    item?: Snippet<[{ item: T; selected: boolean }]>;
    ontextchange?: (text: string) => void;
    onselect?: (item: T) => void;
    onclear?: () => void;
  } = $props();

  if (!text && value) {
    text = value;
  }

  let inputEl = $state<HTMLInputElement>();
  let open = $state(false);
  let selectedIndex = $state(0);
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  const showPopover = $derived(open && !disabled && (loading || items.length > 0 || text !== ''));

  function updateAnchor() {
    const rect = inputEl?.getBoundingClientRect();
    anchor = rect ? { top: rect.top, bottom: rect.bottom, left: rect.left } : null;
  }

  function openMenu() {
    if (disabled) return;
    selectedIndex = 0;
    open = true;
    updateAnchor();
  }

  function handleInput(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    text = input.value;
    value = allowFreeform ? text : '';
    selectedIndex = 0;
    open = true;
    updateAnchor();
    ontextchange?.(text);
  }

  function selectOption(option: T) {
    value = getValue(option);
    text = getLabel(option);
    open = false;
    onselect?.(option);
  }

  function clear() {
    value = '';
    text = '';
    selectedIndex = 0;
    open = false;
    ontextchange?.('');
    onclear?.();
    inputEl?.focus();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (disabled) return;

    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (!open) {
        openMenu();
        return;
      }
      if (items.length > 0) {
        selectedIndex = (selectedIndex + 1) % items.length;
      }
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      if (items.length > 0) {
        selectedIndex = (selectedIndex - 1 + items.length) % items.length;
      }
    } else if (event.key === 'Enter') {
      if (open && items[selectedIndex]) {
        event.preventDefault();
        selectOption(items[selectedIndex]);
      }
    } else if (event.key === 'Escape') {
      if (open) {
        event.preventDefault();
        open = false;
      }
    }
  }
</script>

<FormField {id} {label} {error} {description}>
  <div class={['relative', className]}>
    <input
      bind:this={inputEl}
      {id}
      type="text"
      bind:value={text}
      {placeholder}
      {disabled}
      autocomplete="off"
      role="combobox"
      aria-expanded={showPopover}
      aria-autocomplete="list"
      aria-controls={`${id}-listbox`}
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
      class={['input pr-16', loading && 'pr-20']}
      onfocus={openMenu}
      oninput={handleInput}
      onkeydown={handleKeydown}
    />
    <div class="absolute top-1/2 right-2 flex -translate-y-1/2 items-center gap-1">
      {#if loading}
        <span class="iconify animate-spin text-base text-muted uil--spinner" aria-hidden="true"
        ></span>
      {/if}
      {#if text}
        <button
          type="button"
          class="pane-header-icon-button h-7 w-7"
          aria-label={clearLabel}
          title={clearLabel}
          {disabled}
          onclick={clear}
        >
          <span class="pane-header-icon-glyph iconify uil--times" aria-hidden="true"></span>
        </button>
      {/if}
    </div>
  </div>
</FormField>

<FloatingPopover
  open={showPopover}
  {anchor}
  role="listbox"
  id={`${id}-listbox`}
  class="max-h-72 w-80 overflow-y-auto menu"
  onclose={() => (open = false)}
>
  <div class="menu-section">
    {#if items.length > 0}
      {#each items as option, index (getValue(option))}
        <button
          type="button"
          role="option"
          aria-selected={index === selectedIndex}
          class={['menu-item w-full text-left', index === selectedIndex && 'menu-item-active']}
          onpointerenter={() => (selectedIndex = index)}
          onclick={() => selectOption(option)}
        >
          {#if item}
            {@render item({ item: option, selected: index === selectedIndex })}
          {:else}
            <span class="min-w-0 truncate">{getLabel(option)}</span>
          {/if}
        </button>
      {/each}
    {:else if loading}
      <div class="px-3 py-2 text-sm text-muted">{m['ui.combobox.loading']()}</div>
    {:else}
      <div class="px-3 py-2 text-sm text-muted">{emptyMessage}</div>
    {/if}
  </div>
</FloatingPopover>
