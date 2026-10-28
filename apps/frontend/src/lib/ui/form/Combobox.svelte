<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import { m } from '$lib/i18n/messages';
  import FloatingPopover from '$lib/ui/FloatingPopover.svelte';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
  import FormField from './FormField.svelte';

  let {
    id,
    testid,
    label,
    value = $bindable(''),
    multiple = false,
    selectedItems = $bindable<T[]>([]),
    text = $bindable(''),
    items,
    getValue,
    getLabel,
    getOptionDisabled = () => false,
    placeholder,
    description,
    error,
    labelHidden = false,
    disabled = false,
    loading = false,
    allowFreeform = true,
    emptyMessage = m('ui.combobox.empty'),
    clearLabel = m('ui.combobox.clear'),
    class: className,
    item,
    selectionAdornment,
    selectedItemLeading,
    selectionDescription,
    ontextchange,
    onselect,
    onclear
  }: {
    id: string;
    testid?: string;
    label: string;
    value?: string;
    /** Render removable selections inside the input; selection clears the search text. */
    multiple?: boolean;
    /** Selected objects remain available when asynchronous search results change. */
    selectedItems?: T[];
    text?: string;
    items: T[];
    getValue: (item: T) => string;
    getLabel: (item: T) => string;
    /** Keep unavailable results visible while excluding them from selection and navigation. */
    getOptionDisabled?: (item: T) => boolean;
    placeholder?: string;
    description?: string;
    error?: string;
    labelHidden?: boolean;
    disabled?: boolean;
    loading?: boolean;
    allowFreeform?: boolean;
    emptyMessage?: string;
    clearLabel?: string;
    class?: ClassValue;
    item?: Snippet<[{ item: T; selected: boolean }]>;
    /** Passive content beside a selected value, such as an account badge. */
    selectionAdornment?: Snippet;
    /** Compact passive content before each selected label in multiple mode. */
    selectedItemLeading?: Snippet<[{ item: T }]>;
    /** Text announced with the selected value when the adornment is visual. */
    selectionDescription?: string;
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

  const selectedValues = $derived(new Set(selectedItems.map(getValue)));
  const options = $derived(
    multiple ? items.filter((option) => !selectedValues.has(getValue(option))) : items
  );
  const activeIndex = $derived(
    options[selectedIndex] && !getOptionDisabled(options[selectedIndex])
      ? selectedIndex
      : options.findIndex((option) => !getOptionDisabled(option))
  );
  const showPopover = $derived(open && !disabled && (loading || options.length > 0 || text !== ''));

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
    if (!multiple) value = allowFreeform ? text : '';
    selectedIndex = 0;
    open = true;
    updateAnchor();
    ontextchange?.(text);
  }

  function selectOption(option: T) {
    if (disabled || getOptionDisabled(option)) return;
    if (multiple) {
      if (selectedValues.has(getValue(option))) return;
      selectedItems = [...selectedItems, option];
      text = '';
      selectedIndex = 0;
      open = false;
      ontextchange?.('');
      inputEl?.focus();
    } else {
      value = getValue(option);
      text = getLabel(option);
      open = false;
    }
    onselect?.(option);
  }

  /** Keep keyboard focus in the search after a pill is removed. */
  function removeSelection(option: T) {
    if (disabled) return;
    selectedItems = selectedItems.filter((selected) => getValue(selected) !== getValue(option));
    inputEl?.focus();
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

  /** Traverse at most one full result page, including pages with no selectable results. */
  function moveSelection(direction: number): void {
    if (activeIndex < 0) return;
    for (let step = 1; step <= options.length; step++) {
      const index = (activeIndex + direction * step + options.length) % options.length;
      if (!getOptionDisabled(options[index])) {
        selectedIndex = index;
        return;
      }
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (disabled) return;

    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (!open) {
        openMenu();
        return;
      }
      if (options.length > 0) {
        moveSelection(1);
      }
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      if (options.length > 0) {
        moveSelection(-1);
      }
    } else if (event.key === 'Enter') {
      if (open && options.length > 0) {
        event.preventDefault();
        if (options[activeIndex]) selectOption(options[activeIndex]);
      }
    } else if (event.key === 'Backspace' && multiple && !text && selectedItems.length > 0) {
      event.preventDefault();
      removeSelection(selectedItems[selectedItems.length - 1]);
    } else if (event.key === 'Escape') {
      if (showPopover) {
        event.preventDefault();
        open = false;
      }
    }
  }
</script>

<FormField {id} {label} {error} {description} {labelHidden}>
  <div
    class={[
      'relative',
      multiple &&
        'flex input flex-wrap items-center gap-1.5 p-1.5 pe-16 text-base leading-6 focus-within:border-action',
      multiple && disabled && 'opacity-60',
      className
    ]}
  >
    {#if multiple}
      {#each selectedItems as selected (getValue(selected))}
        <span
          class="inline-flex h-7 max-w-full min-w-0 items-center gap-1 rounded-md border border-input-border bg-surface selection-indicator ps-1.5 pe-0.5"
          data-testid="combobox-selection"
        >
          {#if selectedItemLeading}
            <span class="flex shrink-0 items-center" aria-hidden="true">
              {@render selectedItemLeading({ item: selected })}
            </span>
          {/if}
          <bdi class="min-w-0 truncate">{getLabel(selected)}</bdi>
          <button
            type="button"
            class="mini-icon-action size-6 items-center justify-center hover:bg-surface-emphasized"
            aria-label={m('ui.combobox.remove', { label: getLabel(selected) })}
            {disabled}
            onclick={() => removeSelection(selected)}
            onkeydown={(event) => {
              if (event.key === 'Backspace' || event.key === 'Delete') {
                event.preventDefault();
                removeSelection(selected);
              }
            }}
          >
            <span class="iconify icon-[uil--times] text-sm" aria-hidden="true"></span>
          </button>
        </span>
      {/each}
    {/if}
    <input
      bind:this={inputEl}
      {id}
      data-testid={testid}
      type="text"
      bind:value={text}
      {placeholder}
      {disabled}
      autocomplete="off"
      role="combobox"
      aria-expanded={showPopover}
      aria-autocomplete="list"
      aria-controls={`${id}-listbox`}
      aria-activedescendant={showPopover && options[activeIndex]
        ? `${id}-option-${activeIndex}`
        : undefined}
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={[
        error ? `${id}-error` : description ? `${id}-description` : undefined,
        value && selectionDescription ? `${id}-selection-description` : undefined
      ]
        .filter(Boolean)
        .join(' ') || undefined}
      class={multiple
        ? 'h-7 min-w-20 flex-1 bg-transparent px-1.5 py-0 text-base leading-6 text-text outline-none placeholder:text-muted/70'
        : ['input pe-16', loading && 'pe-20', selectionAdornment && value && 'pe-28']}
      onfocus={openMenu}
      oninput={handleInput}
      onkeydown={handleKeydown}
    />
    <div class="absolute end-2 top-1/2 flex -translate-y-1/2 items-center gap-1">
      {#if selectionAdornment && value}
        <span class="pointer-events-none">{@render selectionAdornment()}</span>
      {/if}
      {#if loading}
        <span
          class="iconify icon-[uil--spinner] animate-spin text-base text-muted"
          aria-hidden="true"
        ></span>
      {/if}
      {#if text}
        <button
          type="button"
          class="icon-action"
          aria-label={clearLabel}
          title={clearLabel}
          {disabled}
          onclick={clear}
        >
          <span class="iconify icon-[uil--times] pane-header-icon-glyph" aria-hidden="true"></span>
        </button>
      {/if}
    </div>
    {#if value && selectionDescription}
      <span id={`${id}-selection-description`} class="sr-only">{selectionDescription}</span>
    {/if}
  </div>
</FormField>

<FloatingPopover
  open={showPopover}
  {anchor}
  role="listbox"
  id={`${id}-listbox`}
  class="w-80 menu"
  onclose={() => (open = false)}
>
  <!-- Scroll inside the section so the frame's inset and border stay visible. -->
  <div class="max-h-72 overflow-y-auto menu-section">
    {#if options.length > 0}
      {#each options as option, index (getValue(option))}
        <button
          type="button"
          role="option"
          id={`${id}-option-${index}`}
          aria-selected={index === activeIndex}
          aria-disabled={getOptionDisabled(option)}
          disabled={getOptionDisabled(option)}
          class={[
            'menu-item w-full text-start disabled:cursor-default disabled:opacity-50',
            index === activeIndex && 'menu-item-active'
          ]}
          onpointerenter={() => {
            if (!getOptionDisabled(option)) selectedIndex = index;
          }}
          onclick={() => selectOption(option)}
        >
          {#if item}
            {@render item({ item: option, selected: index === activeIndex })}
          {:else}
            <span class="min-w-0 truncate">{getLabel(option)}</span>
          {/if}
        </button>
      {/each}
    {:else if loading}
      <LoadingFog class="m-2 h-10" label={m('ui.combobox.loading')} />
    {:else}
      <div class="px-3 py-2 text-sm text-muted">{emptyMessage}</div>
    {/if}
  </div>
</FloatingPopover>
